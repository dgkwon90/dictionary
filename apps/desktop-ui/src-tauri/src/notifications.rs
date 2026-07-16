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
use tauri::{AppHandle, Runtime};
use tauri_plugin_http::reqwest;
use tauri_plugin_notification::NotificationExt;

const BASE_URL: &str = "http://127.0.0.1:48989";
const POLL_INTERVAL: Duration = Duration::from_secs(7);
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
}

/// 폴 루프를 백그라운드 태스크로 시작한다(앱 setup에서 호출).
pub fn spawn<R: Runtime>(app: &AppHandle<R>) {
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        let client = reqwest::Client::new();
        // 세션 내 이미 띄운 알림 id — 재발화 방지(서버는 ack 전까지 계속 반환한다).
        let mut fired: HashSet<String> = HashSet::new();
        loop {
            if let Err(err) = poll_once(&app, &client, &mut fired).await {
                log::debug!("notification poll failed: {err}");
            }
            tokio::time::sleep(POLL_INTERVAL).await;
        }
    });
}

type BoxError = Box<dyn std::error::Error + Send + Sync>;

async fn fetch(client: &reqwest::Client) -> Result<NotificationsResponse, BoxError> {
    let body = client
        .get(format!("{BASE_URL}/v1/notifications"))
        .send()
        .await?
        .text()
        .await?;
    Ok(serde_json::from_str(&body)?)
}

async fn poll_once<R: Runtime>(
    app: &AppHandle<R>,
    client: &reqwest::Client,
    fired: &mut HashSet<String>,
) -> Result<(), BoxError> {
    let resp = fetch(client).await?;

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

/// 미확인 알림을 모두 ack한다(사용자가 앱을 열어 확인했을 때 호출 → 트레이 배지 클리어).
pub fn ack_all<R: Runtime>(app: &AppHandle<R>) {
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        let client = reqwest::Client::new();
        if let Ok(resp) = fetch(&client).await {
            for item in &resp.notifications {
                let _ = client
                    .post(format!("{BASE_URL}/v1/notifications/{}/ack", item.id))
                    .send()
                    .await;
            }
        }
        set_tray_badge(&app, false);
    });
}
