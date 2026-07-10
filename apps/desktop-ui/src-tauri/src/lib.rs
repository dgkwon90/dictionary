//! Neulsang(늘상) 데스크톱 셸.
//!
//! 트레이·기본 윈도우를 띄우고, desktopd 사이드카를 자식 프로세스로 관리한다.
//! 실제 화면(Quick Search/Inbox/Review/Dashboard/Settings)은 프론트엔드에서 라우팅한다.

mod sidecar;
mod tray;

use sidecar::Desktopd;
use tauri::{Manager, RunEvent};

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_log::Builder::default().build())
        .plugin(tauri_plugin_http::init())
        .plugin(tauri_plugin_opener::init())
        .manage(Desktopd::default())
        .setup(|app| {
            app.state::<Desktopd>().spawn();
            tray::build(app.handle())?;
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(|app, event| {
            if let RunEvent::Exit = event {
                app.state::<Desktopd>().shutdown();
            }
        });
}
