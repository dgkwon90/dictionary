//! 메인 윈도우를 띄우고 어느 화면으로 갈지 알리는 한 곳.
//!
//! 트레이 메뉴, OS 알림 클릭(#29), Quick Search 팝업의 "모르는 단어 고르기"가 모두
//! 이 경로를 쓴다. 셋이 각자 창을 띄우고 각자 이벤트를 쏘면 payload 모양이 조용히
//! 갈라지므로(프론트는 그 중 하나만 맞춰도 컴파일된다) 한 함수로 모은다.

use serde::Serialize;
use tauri::{AppHandle, Emitter, Manager, Runtime};

/// `navigate` 이벤트 payload. route는 프론트엔드 `labels.ts`의 `ROUTES`와 문자 그대로
/// 일치해야 하는 wire contract다(한글 라벨이 아니라 영문 route를 보낸다).
///
/// capture_id는 "이 화면을 열되 이 검색을 펴 둬라"는 뜻으로, 문장에서 모르는 단어를
/// 고르는 흐름에만 쓴다 — 팝업은 포커스를 잃으면 숨으므로(hide-on-blur) 여러 단어를
/// 고르는 중에 다른 창을 클릭하면 진행이 통째로 사라진다. 그래서 그 작업만 메인 창으로
/// 넘긴다.
#[derive(Clone, Serialize)]
pub struct Navigate {
    pub route: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub capture_id: Option<String>,
}

/// 메인 윈도우를 보이고 포커스한 뒤 이동할 화면을 알린다.
pub fn show_main<R: Runtime>(app: &AppHandle<R>, route: &str, capture_id: Option<String>) {
    let Some(window) = app.get_webview_window("main") else {
        log::error!("main window not found for navigate({route})");
        return;
    };
    let _ = window.show();
    let _ = window.set_focus();
    // 갈 화면을 모르면 창만 띄운다. 알림을 눌렀는데 아무 일도 안 일어나는 것보다는
    // 앱이 열리는 편이 낫고, 빈 route를 보내면 프론트가 어차피 무시한다.
    if route.is_empty() {
        return;
    }
    let payload = Navigate {
        route: route.to_string(),
        capture_id,
    };
    if let Err(err) = window.emit("navigate", payload) {
        log::error!("failed to emit navigate({route}): {err}");
    }
}

/// Quick Search 팝업이 문장의 단어 선택을 메인 창으로 넘길 때 부른다.
#[tauri::command]
pub fn open_main_screen(app: AppHandle, route: String, capture_id: Option<String>) {
    show_main(&app, &route, capture_id);
}
