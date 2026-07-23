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

use std::path::PathBuf;
use std::process::{Child, Command};
use std::sync::Mutex;

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

    /// 실행 중인 desktopd를 종료하고 회수한다. 앱 종료 시 호출한다.
    pub fn shutdown(&self) {
        if let Some(mut child) = self.child.lock().expect("desktopd lock poisoned").take() {
            if let Err(err) = child.kill() {
                log::error!("failed to kill desktopd: {err}");
            }
            let _ = child.wait();
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
