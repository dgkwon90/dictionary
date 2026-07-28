//! 트레이 아이콘과 메뉴(PRD §10.1).
//!
//! Quick Search는 프레임리스 팝업(`popup`)을 띄운다. 나머지 항목은 메인 윈도우를 띄우고
//! `navigate` 이벤트로 해당 화면으로 이동시킨다. Quit은 앱을 종료한다(사이드카도 정리).

use crate::navigate;
use crate::popup;
use tauri::menu::{Menu, MenuItem, PredefinedMenuItem};
use tauri::tray::TrayIconBuilder;
use tauri::{AppHandle, Runtime};

/// Quick Search는 팝업이라 별도 처리하고, 아래는 메인 윈도우 화면.
/// (id, route, 트레이 표시 라벨) — route는 프론트엔드 `labels.ts`의 `ROUTES`와 문자 그대로
/// 일치해야 하는 wire contract라 절대 한글화하지 않는다(navigate 이벤트 payload로 그대로
/// 나간다). 화면에 보이는 텍스트만 한글 라벨(`routeLabel()`과 동일한 매핑, Rust는 TS 모듈을
/// import 못 해 수동으로 맞춰준다)로 바꾼다.
const ITEMS: &[(&str, &str, &str)] = &[
    ("inbox", "Inbox", "검색함"),
    ("review", "Today Review", "오늘 복습"),
    ("dashboard", "Dashboard", "내 기록"),
    ("settings", "Settings", "설정"),
];
const QUICK_SEARCH_ID: &str = "quick_search";

/// 앱 setup 단계에서 트레이 아이콘을 구성한다.
pub fn build<R: Runtime>(app: &AppHandle<R>) -> tauri::Result<()> {
    let quick_search = MenuItem::with_id(app, QUICK_SEARCH_ID, "빠른 검색", true, None::<&str>)?;
    let mut menu_items: Vec<MenuItem<R>> = Vec::with_capacity(ITEMS.len());
    for (id, _route, label) in ITEMS {
        menu_items.push(MenuItem::with_id(app, *id, *label, true, None::<&str>)?);
    }
    let separator = PredefinedMenuItem::separator(app)?;
    let quit = MenuItem::with_id(app, "quit", "끝내기", true, None::<&str>)?;

    // Menu::with_items는 &dyn IsMenuItem 슬라이스를 받는다.
    let mut refs: Vec<&dyn tauri::menu::IsMenuItem<R>> = vec![&quick_search];
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
    match id {
        "quit" => app.exit(0),
        QUICK_SEARCH_ID => popup::show(app),
        _ => {
            // 프론트엔드 라우트는 이 문자열로 식별하므로 표시 라벨이 아니라 route를 보낸다.
            if let Some((_, route, _label)) = ITEMS.iter().find(|(item_id, _, _)| *item_id == id) {
                navigate::show_main(app, route, None);
            }
        }
    }
}
