//! Quick Search 팝업 윈도우 제어(PRD §10.2).
//!
//! 글로벌 단축키와 트레이 "Quick Search"가 공유한다. 팝업은 tauri.conf.json에
//! `quicksearch` 라벨로 선언(프레임리스·항상 위·시작 시 숨김)돼 있고, 여기서는 보이기·
//! 포커스·중앙 정렬 후 프론트엔드에 활성화 이벤트를 보낸다(클립보드 재삽입·입력 초기화용).

use tauri::{AppHandle, Emitter, Manager, Runtime};

pub const LABEL: &str = "quicksearch";

/// Quick Search 팝업을 띄우고 포커스한다. 이미 떠 있으면 앞으로 가져온다.
pub fn show<R: Runtime>(app: &AppHandle<R>) {
    let Some(window) = app.get_webview_window(LABEL) else {
        log::error!("quicksearch window not found");
        return;
    };
    let _ = window.center();
    let _ = window.show();
    let _ = window.set_focus();
    // 매 호출마다 프론트엔드가 클립보드를 다시 읽고 입력을 초기화하도록 알린다.
    if let Err(err) = window.emit("quicksearch:activate", ()) {
        log::error!("failed to emit quicksearch:activate: {err}");
    }
}
