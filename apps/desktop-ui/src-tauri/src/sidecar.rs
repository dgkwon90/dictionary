//! desktopd 사이드카 프로세스 생명주기 관리.
//!
//! UI는 `apps/desktopd`(Go HTTP 사이드카)를 자식 프로세스로 실행하고, 앱 종료 시 함께
//! 종료시킨다. 바이너리 경로는 (1) 환경변수 `NEULSANG_DESKTOPD_BIN`, (2) 실행 파일 옆,
//! (3) 개발용 상대 경로 순으로 탐색한다. 어디에도 없으면 UI만 단독 기동한다(스켈레톤 단계).

use std::path::PathBuf;
use std::process::{Child, Command};
use std::sync::Mutex;

/// 관리 중인 desktopd 자식 프로세스. Tauri managed state로 보관한다.
#[derive(Default)]
pub struct Desktopd {
    child: Mutex<Option<Child>>,
}

impl Desktopd {
    /// desktopd 바이너리를 찾아 실행한다. 찾지 못하면 경고만 남기고 계속 진행한다.
    pub fn spawn(&self) {
        match resolve_binary() {
            // NEULSANG_PARENT_PID로 사이드카의 부모 감시(watchdog)를 켠다 — 셸이
            // 비정상 종료돼도 desktopd가 스스로 종료해 고아로 남지 않도록.
            Some(path) => match Command::new(&path)
                .env("NEULSANG_PARENT_PID", std::process::id().to_string())
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
