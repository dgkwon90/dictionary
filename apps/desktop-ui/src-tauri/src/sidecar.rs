//! desktopd 사이드카 프로세스 생명주기 관리.
//!
//! UI는 `apps/desktopd`(Go HTTP 사이드카)를 자식 프로세스로 실행하고, 앱 종료 시 함께
//! 종료시킨다. 바이너리 경로는 (1) 환경변수 `NEULSANG_DESKTOPD_BIN`, (2) 실행 파일 옆,
//! (3) 개발용 상대 경로 순으로 탐색한다. 어디에도 없으면 UI만 단독 기동한다.
//!
//! 배포 번들에서는 Tauri `bundle.externalBin`이 `desktopd-<target-triple>`를 앱 안
//! `Contents/MacOS/desktopd`로 복사·서명한다 → (2) "실행 파일 옆" 분기가 이를 찾는다.
//!
//! **왜 `std::process`로 직접 spawn하나(tauri-plugin-shell `app.shell().sidecar()` 대신)**:
//! `externalBin`은 번들·서명만 담당하고 실행 방식은 강제하지 않는다. 셸 플러그인 sidecar API를
//! 쓰면 `shell:allow-execute` capability + 추가 의존이 필요하지만, 여기선 (a) watchdog용
//! `NEULSANG_PARENT_PID` env 주입, (b) 자체 `Mutex<Option<Child>>` 생명주기(kill/wait)로 충분해
//! 셸 플러그인 없이 직접 spawn한다(공격 표면·의존 최소화). 경로만 externalBin이 놓은 위치로 해결.
//!
//! **세션 API 토큰(review R-01)**: desktopd의 모든 `/v1/*`는 이제 bearer 토큰을 요구한다.
//! 셸이 spawn마다 난수 토큰을 생성해 `NEULSANG_API_TOKEN` env로 주입하고, webview(토큰이
//! 필요한 `get_api_token` Tauri command 경유)와 알림 폴 루프(같은 프로세스라 managed state로
//! 직접 조회)가 그 값을 공유한다. 토큰은 프로세스 메모리에만 있고 저장/로그 없음.
//!
//! **Graceful shutdown(review R-03)**: desktopd의 Go 쪽 `Run()`은 SIGTERM/컨텍스트 취소를
//! 받으면 이미 잘 정리한다 — HTTP 신규 접수 중단(`srv.Shutdown`) → 진행 중 explain 컨텍스트
//! 취소(Gemini 호출은 곧바로 에러 반환) → 그 실패를 `lookup_jobs`에 기록(취소돼도 안 죽는
//! 별도 컨텍스트로) → `explainWG.Wait()` → DB close. 문제는 예전엔 셸이 이 경로를 전혀 타지
//! 않고 `child.kill()`(SIGKILL)로 바로 죽였다는 것 — 그러면 진행 중이던 job이 상태 기록 없이
//! `running`으로 영구 고착된다(재시작 후 #복구 로직이 그걸 훑어 failed로 정리하지만, 애초에
//! 정상 종료 시엔 이 경로를 안 타는 게 맞다). 이제 Unix에서는 SIGTERM을 먼저 보내고
//! `SHUTDOWN_GRACE` 동안 기다린 뒤에만 SIGKILL로 넘어간다. Windows는 표준 라이브러리에
//! SIGTERM 상당 기능이 없어(콘솔 제어 이벤트는 별도 Win32 FFI 필요) 즉시 kill 그대로 —
//! 알려진 갭(macOS 우선, docs/planning/remaining-work.md RW-03/RW-11).

use std::path::PathBuf;
use std::process::{Child, Command};
use std::sync::Mutex;
use std::time::Duration;

/// HTTP 서버의 `shutdownTimeout`(Go, bootstrap.go)과 맞춘 유예 시간 — 그 안에 자연스럽게
/// 끝나면 SIGKILL이 필요 없다.
const SHUTDOWN_GRACE: Duration = Duration::from_secs(5);

use rand::RngCore;

/// 관리 중인 desktopd 자식 프로세스와 이번 세션의 API 토큰. Tauri managed state로 보관한다.
#[derive(Default)]
pub struct Desktopd {
    child: Mutex<Option<Child>>,
    token: Mutex<Option<String>>,
}

impl Desktopd {
    /// desktopd 바이너리를 찾아 실행한다. 찾지 못하면 경고만 남기고 계속 진행한다.
    /// 바이너리 발견 여부와 무관하게 세션 토큰은 항상 생성해 저장한다(호출자가
    /// `token()`으로 일관되게 가져갈 수 있도록).
    pub fn spawn(&self) {
        let token = generate_token();
        *self.token.lock().expect("desktopd lock poisoned") = Some(token.clone());

        match resolve_binary() {
            // NEULSANG_PARENT_PID로 사이드카의 부모 감시(watchdog)를 켠다 — 셸이
            // 비정상 종료돼도 desktopd가 스스로 종료해 고아로 남지 않도록.
            Some(path) => match Command::new(&path)
                .env("NEULSANG_PARENT_PID", std::process::id().to_string())
                .env("NEULSANG_API_TOKEN", &token)
                .spawn()
            {
                Ok(child) => {
                    log::info!("desktopd started: {} (pid {})", path.display(), child.id());
                    *self.child.lock().expect("desktopd lock poisoned") = Some(child);
                }
                Err(err) => log::error!("failed to spawn desktopd at {}: {err}", path.display()),
            },
            None => log::warn!(
                "desktopd binary not found (set NEULSANG_DESKTOPD_BIN); running UI without sidecar"
            ),
        }
    }

    /// 이번 세션의 API 토큰. `spawn()` 호출 후에는 항상 `Some`.
    pub fn token(&self) -> Option<String> {
        self.token.lock().expect("desktopd lock poisoned").clone()
    }

    /// 실행 중인 desktopd를 종료하고 회수한다. 앱 종료 시 호출한다. 가능하면 graceful하게
    /// (SIGTERM+대기), 유예 시간 안에 안 끝나면 강제 종료로 넘어간다.
    pub fn shutdown(&self) {
        if let Some(mut child) = self.child.lock().expect("desktopd lock poisoned").take() {
            graceful_stop(&mut child, SHUTDOWN_GRACE);
            log::info!("desktopd stopped");
        }
    }
}

/// 32바이트 CSPRNG 값을 hex로 인코딩한 세션 토큰(64자)을 생성한다.
fn generate_token() -> String {
    let mut bytes = [0u8; 32];
    rand::rng().fill_bytes(&mut bytes);
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

/// `child`에게 정상 종료를 요청하고(Unix: SIGTERM), `grace` 동안 스스로 끝나길 기다린다.
/// 그래도 살아있으면 강제 종료로 넘어간다. Windows는 SIGTERM 상당 기능이 표준 라이브러리에
/// 없어 곧바로 강제 종료한다(알려진 갭, 모듈 상단 문서 참고).
#[cfg(unix)]
fn graceful_stop(child: &mut Child, grace: Duration) {
    let pid = child.id() as libc::pid_t;
    // SAFETY: pid는 우리가 소유한 살아있는 자식 프로세스이고, SIGTERM은 표준적인
    // 비파괴적 종료 요청이다.
    if unsafe { libc::kill(pid, libc::SIGTERM) } != 0 {
        log::warn!("failed to send SIGTERM to desktopd (pid {pid}); falling back to force kill");
        force_kill(child);
        return;
    }

    let deadline = std::time::Instant::now() + grace;
    loop {
        match child.try_wait() {
            Ok(Some(_)) => return, // 스스로 종료 완료(이미 reap됨)
            Ok(None) => {
                if std::time::Instant::now() >= deadline {
                    log::warn!(
                        "desktopd did not exit within {grace:?} of SIGTERM; sending SIGKILL"
                    );
                    force_kill(child);
                    return;
                }
                std::thread::sleep(Duration::from_millis(50));
            }
            Err(err) => {
                log::error!("failed to wait for desktopd: {err}");
                return;
            }
        }
    }
}

#[cfg(not(unix))]
fn graceful_stop(child: &mut Child, _grace: Duration) {
    force_kill(child);
}

fn force_kill(child: &mut Child) {
    if let Err(err) = child.kill() {
        log::error!("failed to kill desktopd: {err}");
    }
    let _ = child.wait();
}

/// desktopd 실행 파일 경로를 탐색 순서대로 해석한다.
fn resolve_binary() -> Option<PathBuf> {
    if let Ok(explicit) = std::env::var("NEULSANG_DESKTOPD_BIN") {
        let path = PathBuf::from(explicit);
        if path.is_file() {
            return Some(path);
        }
        log::warn!(
            "NEULSANG_DESKTOPD_BIN set but not a file: {}",
            path.display()
        );
    }

    let bin_name = if cfg!(windows) {
        "desktopd.exe"
    } else {
        "desktopd"
    };

    // 배포: 실행 파일과 같은 디렉터리(사이드카 번들).
    if let Ok(exe) = std::env::current_exe() {
        if let Some(dir) = exe.parent() {
            let beside = dir.join(bin_name);
            if beside.is_file() {
                return Some(beside);
            }
        }
    }

    // 개발: apps/desktop-ui/src-tauri 기준 상대 경로의 빌드 산출물.
    let dev = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../desktopd")
        .join(bin_name);
    if dev.is_file() {
        return Some(dev);
    }

    None
}

#[cfg(all(test, unix))]
mod tests {
    use super::*;
    use std::process::Command;

    /// SIGTERM에 반응해 스스로 종료하는 프로세스(기본 동작)는 SIGKILL 없이,
    /// grace 시간보다 훨씬 빨리 종료돼야 한다.
    #[test]
    fn graceful_stop_lets_a_well_behaved_child_exit_on_sigterm() {
        let mut child = Command::new("sleep")
            .arg("30")
            .spawn()
            .expect("spawn sleep");

        let started = std::time::Instant::now();
        graceful_stop(&mut child, Duration::from_secs(5));
        let elapsed = started.elapsed();

        assert!(
            elapsed < Duration::from_secs(2),
            "graceful_stop took {elapsed:?}, want well under the 5s grace period \
             (sleep should exit on SIGTERM almost immediately)"
        );
        assert!(
            matches!(child.try_wait(), Ok(Some(_))),
            "child should already be reaped after graceful_stop returns"
        );
    }

    /// SIGTERM을 무시하도록 trap을 건 프로세스는 grace 시간이 지나면 SIGKILL로
    /// 강제 종료돼야 한다 — graceful_stop이 무한정 기다리며 앱 종료를 막지 않는다.
    #[test]
    fn graceful_stop_escalates_to_sigkill_after_grace() {
        // The trailing `; true` matters: without it, a shell running a single
        // final simple command commonly exec()s straight into it (tail-call),
        // replacing the process image and losing the TERM trap along with it —
        // then `sleep` would die on SIGTERM via its own default disposition,
        // making this test flaky. Giving the shell more work after `sleep`
        // keeps it as the running (and TERM-ignoring) process throughout.
        //
        // codex review: a fixed sleep before signaling is a race, not a
        // guarantee — a loaded/throttled CI runner could still not have reached
        // `trap` in time, deriving spurious flakes. Instead, the script echoes
        // a marker line *after* `trap` runs, and the test blocks on reading that
        // line from the child's stdout — a real synchronization point, not a
        // timing guess.
        let mut child = Command::new("sh")
            .args(["-c", "trap '' TERM; echo ready; sleep 30; true"])
            .stdout(std::process::Stdio::piped())
            .spawn()
            .expect("spawn sh");
        let mut stdout = std::io::BufReader::new(child.stdout.take().expect("piped stdout"));
        let mut ready_line = String::new();
        std::io::BufRead::read_line(&mut stdout, &mut ready_line).expect("read ready marker");
        assert_eq!(ready_line.trim(), "ready", "child did not signal readiness");
        drop(stdout); // done with the pipe; the child writes nothing further

        let grace = Duration::from_millis(300);
        let started = std::time::Instant::now();
        graceful_stop(&mut child, grace);
        let elapsed = started.elapsed();

        assert!(
            elapsed >= grace,
            "graceful_stop returned before the grace period elapsed: {elapsed:?} < {grace:?}"
        );
        assert!(
            elapsed < grace + Duration::from_secs(2),
            "graceful_stop took {elapsed:?} after grace {grace:?}, want it to escalate to \
             SIGKILL promptly rather than hang"
        );
        assert!(
            matches!(child.try_wait(), Ok(Some(_))),
            "child should be dead (via SIGKILL) after graceful_stop returns"
        );
    }
}
