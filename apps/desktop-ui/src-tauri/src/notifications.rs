//! 사이드카 알림 폴 루프(ADR-0008).
//!
//! desktopd의 `GET /v1/notifications`를 주기 폴링해 새 알림을 OS 알림으로 띄우고,
//! 미확인 알림이 있으면 트레이에 "New"(●)를 표시한다. **이 루프는 Rust 셸이 소유한다** —
//! 메인 창을 트레이로 숨겨도(webview JS 타이머와 달리) 계속 동작해야 "복습 시간"·"결과
//! 준비" 알림이 창 닫힘 상태에서도 전달되기 때문(ADR-0008 핵심).
//!
//! ack는 폴 루프가 아니라 **사용자가 앱을 열 때**(메인 창 포커스) 수행한다. 그래서 사용자가
//! 앱을 보기 전까지 트레이 배지가 유지된다.

use std::collections::HashSet;
use std::time::Duration;

use serde::Deserialize;
use tauri::{AppHandle, Manager, Runtime};
use tauri_plugin_http::reqwest;
use tauri_plugin_notification::NotificationExt;

use crate::sidecar::{Desktopd, BASE_URL};

const POLL_INTERVAL: Duration = Duration::from_secs(7);
/// 타임아웃이 없으면 응답하지 않는 요청 하나가 폴 루프를 영원히 붙잡는다 — 알림은 그
/// 순간부터 조용히 멈추고 로그도 남지 않는다. 상대는 같은 기기의 사이드카라 정상 응답은
/// 밀리초 단위이므로, 폴 간격보다 짧게 잡아 다음 틱이 밀리지 않게 한다.
const REQUEST_TIMEOUT: Duration = Duration::from_secs(5);

/// 타임아웃이 걸린 HTTP 클라이언트. 빌더 실패는 현실적으로 불가능하지만, 그때도 알림을
/// 통째로 잃지 않도록 기본 클라이언트로 떨어진다(타임아웃 없이라도 도는 편이 낫다).
fn http_client() -> reqwest::Client {
    reqwest::Client::builder()
        .timeout(REQUEST_TIMEOUT)
        .build()
        .unwrap_or_default()
}
const TRAY_ID: &str = "neulsang-tray";

#[derive(Deserialize)]
struct NotificationsResponse {
    notifications: Vec<NotificationItem>,
    unacked_count: i64,
}

#[derive(Deserialize)]
struct NotificationItem {
    id: String,
    title: String,
    body: String,
    // 클릭 시 이동할 메인 탭 라벨(#29). 없을 수 있어 Option(serde: 필드 없으면 None).
    #[serde(default)]
    route: Option<String>,
}

/// 폴 루프를 백그라운드 태스크로 시작한다(앱 setup에서 호출).
pub fn spawn<R: Runtime>(app: &AppHandle<R>) {
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        let client = http_client();
        let token = app.state::<Desktopd>().token().unwrap_or_default();
        // 세션 내 이미 띄운 알림 id — 재발화 방지(서버는 ack 전까지 계속 반환한다).
        let mut fired: HashSet<String> = HashSet::new();
        loop {
            if let Err(err) = poll_once(&app, &client, &token, &mut fired).await {
                log::debug!("notification poll failed: {err}");
            }
            tokio::time::sleep(POLL_INTERVAL).await;
        }
    });
}

type BoxError = Box<dyn std::error::Error + Send + Sync>;

// desktopd의 모든 /v1/*가 bearer 토큰을 요구한다(review R-01) — 이 프로세스는 셸 자신이라
// sidecar::Desktopd managed state에서 토큰을 직접 읽어 매 요청에 붙인다(webview처럼
// invoke 왕복이 필요 없음).
async fn fetch(client: &reqwest::Client, token: &str) -> Result<NotificationsResponse, BoxError> {
    let body = client
        .get(format!("{BASE_URL}/v1/notifications"))
        .header("Authorization", format!("Bearer {token}"))
        .send()
        .await?
        .text()
        .await?;
    Ok(serde_json::from_str(&body)?)
}

async fn poll_once<R: Runtime>(
    app: &AppHandle<R>,
    client: &reqwest::Client,
    token: &str,
    fired: &mut HashSet<String>,
) -> Result<(), BoxError> {
    let resp = fetch(client, token).await?;

    for item in &resp.notifications {
        if fired.contains(&item.id) {
            continue;
        }
        if let Err(err) = app
            .notification()
            .builder()
            .title(&item.title)
            .body(&item.body)
            .show()
        {
            log::warn!("failed to show OS notification: {err}");
        }
        fired.insert(item.id.clone());
    }

    set_tray_badge(app, resp.unacked_count > 0);
    Ok(())
}

/// 트레이 "New" 표시. macOS 메뉴바는 숫자 배지를 지원하지 않아 title에 점(●)을 붙인다.
fn set_tray_badge<R: Runtime>(app: &AppHandle<R>, has_new: bool) {
    if let Some(tray) = app.tray_by_id(TRAY_ID) {
        // macOS NSStatusItem 타이틀은 set_title(None)으로 잔류(직전 "●"가 안 지워짐)하는
        // 경우가 있어, 비울 때 빈 문자열을 명시적으로 넣어 확실히 클리어한다.
        let title = if has_new { "●" } else { "" };
        if let Err(err) = tray.set_title(Some(title)) {
            log::debug!("failed to set tray badge: {err}");
        }
    }
}

/// 알림 클릭 등으로 앱이 재활성화될 때(macOS `RunEvent::Reopen`) 호출한다(#29): 메인 창을
/// 보이고 포커스한 뒤, **가장 최근 미확인 알림의 route**가 있으면 그 화면으로 이동시킨다.
/// 배너별 정확 딥라우팅은 tauri-plugin-notification desktop이 클릭 콜백을 주지 않아 불가 →
/// "최신 미확인 알림" 휴리스틱(대기 알림이 하나면 정확, 여럿이면 최신 것). 알림이 없으면
/// 창만 띄운다. `ListUnacked`가 oldest-first라 뒤에서부터 route 있는 첫 항목이 최신이다.
pub fn focus_recent<R: Runtime>(app: &AppHandle<R>) {
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        let token = app.state::<Desktopd>().token().unwrap_or_default();
        let route = match fetch(&http_client(), &token).await {
            Ok(resp) => resp
                .notifications
                .into_iter()
                .rev()
                .find_map(|n| n.route.filter(|r| !r.is_empty())),
            Err(err) => {
                log::debug!("focus_recent fetch failed: {err}");
                None
            }
        };
        // 이동할 route를 못 찾아도 창은 띄운다 — 알림을 눌렀는데 아무 반응이 없는
        // 것보다 앱이 열리는 편이 낫다.
        crate::navigate::show_main(&app, route.as_deref().unwrap_or(""), None);
    });
}

/// 미확인 알림을 모두 ack한다(사용자가 앱을 열어 확인했을 때 호출 → 트레이 배지 클리어).
pub fn ack_all<R: Runtime>(app: &AppHandle<R>) {
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        let client = http_client();
        let token = app.state::<Desktopd>().token().unwrap_or_default();
        if let Ok(resp) = fetch(&client, &token).await {
            for item in &resp.notifications {
                let _ = client
                    .post(format!("{BASE_URL}/v1/notifications/{}/ack", item.id))
                    .header("Authorization", format!("Bearer {token}"))
                    .send()
                    .await;
            }
        }
        set_tray_badge(&app, false);
    });
}
