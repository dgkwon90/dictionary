//! Neulsang(늘상) 데스크톱 셸.
//!
//! 트레이·기본 윈도우를 띄우고, desktopd 사이드카를 자식 프로세스로 관리한다.
//! 실제 화면(Quick Search/Inbox/Review/Dashboard/Settings)은 프론트엔드에서 라우팅한다.

mod notifications;
mod popup;
mod sidecar;
mod tray;

use sidecar::Desktopd;
use tauri::{Manager, RunEvent, WindowEvent};

/// webview(main/quicksearch)가 desktopd API 호출에 붙일 세션 토큰을 가져온다(review R-01).
/// 빌드타임 상수로 굽지 않고 런타임에 managed state에서 읽어 반환 — 토큰은 매 실행마다
/// 새로 생성되므로 이 방법 외엔 프론트가 알 방법이 없다.
#[tauri::command]
fn get_api_token(state: tauri::State<Desktopd>) -> Option<String> {
    state.token()
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_log::Builder::default().build())
        .plugin(tauri_plugin_http::init())
        .plugin(tauri_plugin_clipboard_manager::init())
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_opener::init())
        // Settings 백업·복원 UI(RW-09): 사용자가 고른 경로에 JSON export를 쓰고,
        // 고른 JSON 파일을 읽어 import한다. dialog의 open()/save()가 선택한 경로를
        // 런타임에 fs 스코프에 자동 추가하므로, capabilities에는 정적 경로 없이
        // 커맨드 권한만 선언한다(capabilities/default.json 참고).
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_fs::init())
        .manage(Desktopd::default())
        .invoke_handler(tauri::generate_handler![get_api_token])
        .setup(|app| {
            app.state::<Desktopd>().spawn();
            tray::build(app.handle())?;
            register_global_shortcut(app.handle())?;
            // 사이드카 알림 폴 루프(ADR-0008) — Rust 셸이 소유(창 닫힘 상태에서도 동작).
            notifications::spawn(app.handle());
            Ok(())
        })
        // 트레이 상주 앱: 창을 닫아도 파괴하지 않고 숨긴다. 파괴하면 트레이 메뉴가
        // 다시 띄울 창(main)을 잃는다. 완전 종료는 트레이 Quit(app.exit)로만.
        .on_window_event(|window, event| match event {
            WindowEvent::CloseRequested { api, .. } => {
                api.prevent_close();
                let _ = window.hide();
            }
            // 사용자가 메인 창을 보면 미확인 알림을 ack해 트레이 "New"를 지운다(ADR-0008).
            WindowEvent::Focused(true) if window.label() == "main" => {
                notifications::ack_all(window.app_handle());
            }
            _ => {}
        })
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(|app, event| match event {
            RunEvent::Exit => app.state::<Desktopd>().shutdown(),
            // macOS: 알림 클릭/Dock 클릭 등으로 앱이 재활성화되면 메인 창을 띄우고
            // 최근 미확인 알림의 화면으로 이동한다(#29).
            #[cfg(target_os = "macos")]
            RunEvent::Reopen { .. } => notifications::focus_recent(app),
            _ => {}
        });
}

/// 어디서든 Quick Search 팝업을 부르는 글로벌 단축키(macOS Cmd+Shift+E / 그 외 Ctrl+Shift+E).
#[cfg(desktop)]
fn register_global_shortcut(app: &tauri::AppHandle) -> Result<(), Box<dyn std::error::Error>> {
    use tauri_plugin_global_shortcut::{Builder, GlobalShortcutExt, ShortcutState};

    const SHORTCUT: &str = "CommandOrControl+Shift+E";

    app.plugin(
        Builder::new()
            .with_handler(|app, _shortcut, event| {
                if event.state() == ShortcutState::Pressed {
                    popup::show(app);
                }
            })
            .build(),
    )?;
    // 다른 앱/OS가 이미 이 조합을 점유하면 등록이 실패한다. 앱 기동을 막지 말고
    // 경고만 남긴다(트레이·팝업은 그대로 쓸 수 있음).
    if let Err(err) = app.global_shortcut().register(SHORTCUT) {
        log::warn!("failed to register global shortcut {SHORTCUT}: {err}");
    }
    Ok(())
}

#[cfg(not(desktop))]
fn register_global_shortcut(_app: &tauri::AppHandle) -> Result<(), Box<dyn std::error::Error>> {
    Ok(())
}
