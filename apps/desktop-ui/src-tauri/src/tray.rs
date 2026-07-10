//! 트레이 아이콘과 메뉴(PRD §10.1).
//!
//! 메뉴 항목을 클릭하면 메인 윈도우를 띄우고 프론트엔드로 `navigate` 이벤트를 보내
//! 해당 화면으로 이동시킨다. Quit은 앱을 종료한다(종료 훅에서 사이드카도 정리).

use tauri::menu::{Menu, MenuItem, PredefinedMenuItem};
use tauri::tray::TrayIconBuilder;
use tauri::{AppHandle, Emitter, Manager, Runtime};

/// 트레이 메뉴 항목 id ↔ 프론트엔드 라우트.
const ITEMS: &[(&str, &str)] = &[
    ("quick_search", "Quick Search"),
    ("inbox", "Inbox"),
    ("review", "Today Review"),
    ("dashboard", "Dashboard"),
    ("settings", "Settings"),
];

/// 앱 setup 단계에서 트레이 아이콘을 구성한다.
pub fn build<R: Runtime>(app: &AppHandle<R>) -> tauri::Result<()> {
    let mut menu_items: Vec<MenuItem<R>> = Vec::with_capacity(ITEMS.len());
    for (id, label) in ITEMS {
        menu_items.push(MenuItem::with_id(app, *id, *label, true, None::<&str>)?);
    }
    let separator = PredefinedMenuItem::separator(app)?;
    let quit = MenuItem::with_id(app, "quit", "Quit", true, None::<&str>)?;

    // Menu::with_items는 &dyn IsMenuItem 슬라이스를 받는다.
    let mut refs: Vec<&dyn tauri::menu::IsMenuItem<R>> = Vec::new();
    for item in &menu_items {
        refs.push(item);
    }
    refs.push(&separator);
    refs.push(&quit);
    let menu = Menu::with_items(app, &refs)?;

    let mut builder = TrayIconBuilder::with_id("neulsang-tray")
        .tooltip("Neulsang 늘상")
        .menu(&menu)
        .show_menu_on_left_click(true)
        .on_menu_event(|app, event| handle_menu_event(app, event.id.as_ref()));
    if let Some(icon) = app.default_window_icon() {
        builder = builder.icon(icon.clone());
    } else {
        log::warn!("no default window icon; tray shows without an icon");
    }
    builder.build(app)?;

    Ok(())
}

fn handle_menu_event<R: Runtime>(app: &AppHandle<R>, id: &str) {
    if id == "quit" {
        app.exit(0);
        return;
    }
    // 프론트엔드 라우트는 라벨로 식별하므로 id가 아니라 대응 라벨을 실어 보낸다.
    let Some((_, route)) = ITEMS.iter().find(|(item_id, _)| *item_id == id) else {
        return;
    };
    show_and_navigate(app, route);
}

/// 메인 윈도우를 보이고 포커스한 뒤, 이동할 라우트를 프론트엔드에 알린다.
fn show_and_navigate<R: Runtime>(app: &AppHandle<R>, route: &str) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.show();
        let _ = window.set_focus();
        if let Err(err) = window.emit("navigate", route) {
            log::error!("failed to emit navigate({route}): {err}");
        }
    } else {
        log::error!("main window not found for navigate({route})");
    }
}
