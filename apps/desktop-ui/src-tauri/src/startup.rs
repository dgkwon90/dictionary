//! 기동 확인: 방금 띄운 사이드카가 정말 **내 것**인지 확인하고, 아니면 사용자에게 이유를
//! 알리고 앱을 닫는다.
//!
//! **왜 필요한가**(실사용 리포트): 앱이 트레이에 상주하는 상태에서 Finder로 한 번 더 실행하면
//! 두 번째 인스턴스의 desktopd가 48989 바인드에 실패해 즉시 죽는다. 그런데 그 창의 webview는
//! 자기 세션 토큰으로 **이미 떠 있던 첫 인스턴스의** desktopd를 호출하므로 모든 `/v1/*`가
//! 401이 되고, 사용자에겐 아무 설명 없이 화면마다 오류만 뜬다.
//!
//! **왜 `/healthz`로는 못 잡나**: 그 경로는 서버에서 인증 면제라(`security.go`) 남의 인스턴스도
//! 200을 준다 — "포트에 뭔가 떠 있다"까지만 알 수 있어, 프론트의 헬스체크(`App.tsx`)도 이
//! 상황을 정상으로 오인한다. 그래서 여기서는 **토큰이 검증되는** `/v1/healthz`를 찌른다:
//! 200이면 내 사이드카(또는 내 토큰을 받아주는 서버), 401/403이면 포트 주인이 남이다.

use std::time::{Duration, Instant};

use tauri::{AppHandle, Manager, Runtime};
use tauri_plugin_dialog::{DialogExt, MessageDialogKind};
use tauri_plugin_http::reqwest;

use crate::sidecar::{Desktopd, BASE_URL};

/// 사이드카가 포트를 열 때까지 기다려 주는 시간. desktopd는 리슨 전에 DB 열기·마이그레이션을
/// 먼저 하고, Windows 첫 실행에서는 백신이 새로 풀린 exe를 스캔하느라 몇 초가 더 걸린다
/// (`App.tsx`의 재시도 주석과 같은 이유). 넉넉히 잡아도 손해가 적다 — "이미 실행 중" 판정은
/// 401로 즉시 나오므로 이 시간은 "정말 안 뜨는" 경우의 대기일 뿐이다.
const STARTUP_TIMEOUT: Duration = Duration::from_secs(20);
const POLL_INTERVAL: Duration = Duration::from_millis(200);
/// 같은 기기의 사이드카라 정상 응답은 밀리초 단위다. 폴 간격을 밀지 않을 만큼만 준다.
const REQUEST_TIMEOUT: Duration = Duration::from_secs(3);

#[derive(Debug, PartialEq, Eq)]
enum Health {
    /// 내 토큰을 받아주는 서버가 응답했다.
    Ready,
    /// 포트를 다른 인스턴스가 쥐고 있다(내 토큰이 거부됐다).
    PortTaken,
    /// 아무도 응답하지 않는다(사이드카가 못 떴다).
    Down,
}

/// 기동 확인을 백그라운드에서 시작한다(앱 setup에서 `Desktopd::spawn()` 직후 호출).
pub fn verify<R: Runtime>(app: &AppHandle<R>) {
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        let token = app.state::<Desktopd>().token().unwrap_or_default();
        let client = reqwest::Client::builder()
            .timeout(REQUEST_TIMEOUT)
            .build()
            .unwrap_or_default();
        let exited = {
            let app = app.clone();
            move || app.state::<Desktopd>().child_exited()
        };

        match probe(&client, BASE_URL, &token, STARTUP_TIMEOUT, exited).await {
            Health::Ready => log::info!("desktopd is up and accepting this session's token"),
            Health::PortTaken => fail(
                &app,
                "Neulsang이 이미 실행 중입니다",
                "이미 열려 있는 Neulsang을 사용해 주세요 — 메뉴 막대(트레이)의 Neulsang 아이콘에서 창을 다시 열 수 있습니다.\n\n\
                 (같은 기기에서 두 번째 Neulsang은 백엔드 포트 48989를 쓸 수 없어 정상 동작하지 않습니다. 이 창은 닫습니다.)",
            ),
            Health::Down => fail(
                &app,
                "Neulsang을 시작하지 못했습니다",
                "백엔드(desktopd)가 응답하지 않아 앱을 닫습니다. 다시 실행해 주세요.\n\n\
                 계속 같은 문제가 나면 로그를 확인해 주세요 — 데이터베이스를 열지 못했을 때도 이 메시지가 나옵니다.",
            ),
        }
    });
}

/// `/v1/healthz`를 응답이 나올 때까지(또는 `deadline`까지) 주기적으로 찌른다.
///
/// `sidecar_exited`는 "내가 띄운 자식이 이미 죽었나"를 알려준다. 죽었고 포트에도 아무도 없다면
/// 더 기다릴 이유가 없어 즉시 `Down`으로 끝낸다(예: v1 DB를 물고 있어 마이그레이션 checksum
/// 불일치로 기동을 거부한 경우 — 20초를 기다려도 달라지지 않는다). 반대로 자식이 죽었어도 남의
/// 인스턴스가 포트를 쥐고 있으면 그쪽에서 401이 오므로 `PortTaken`이 먼저 나온다.
async fn probe<F>(
    client: &reqwest::Client,
    base_url: &str,
    token: &str,
    deadline: Duration,
    mut sidecar_exited: F,
) -> Health
where
    F: FnMut() -> bool,
{
    let started = Instant::now();
    let url = format!("{base_url}/v1/healthz");
    loop {
        match client
            .get(&url)
            .header("Authorization", format!("Bearer {token}"))
            .send()
            .await
        {
            Ok(res) if res.status().is_success() => return Health::Ready,
            // 401은 토큰 불일치, 403은 Host 거부 — 어느 쪽이든 저 서버는 내 요청을 받아주지
            // 않는다. 내 사이드카라면 둘 다 나올 수 없다(토큰을 내가 주입했고 loopback으로
            // 부른다). 즉 포트 주인이 남이라는 뜻이고, 기다린다고 바뀌지 않는다.
            Ok(res)
                if res.status() == reqwest::StatusCode::UNAUTHORIZED
                    || res.status() == reqwest::StatusCode::FORBIDDEN =>
            {
                log::error!(
                    "port {base_url} is held by another instance (status {})",
                    res.status()
                );
                return Health::PortTaken;
            }
            // 그 밖의 응답(5xx 등)은 기동 중일 수 있으니 계속 기다린다.
            Ok(res) => log::debug!("startup probe got unexpected status {}", res.status()),
            Err(err) => log::debug!("startup probe failed (not listening yet?): {err}"),
        }
        if sidecar_exited() {
            return Health::Down;
        }
        if started.elapsed() >= deadline {
            log::error!("desktopd did not become ready within {deadline:?}");
            return Health::Down;
        }
        tokio::time::sleep(POLL_INTERVAL).await;
    }
}

/// 이유를 알리는 다이얼로그를 띄우고, 사용자가 닫으면 앱을 종료한다. 사용자가 확인하기 전에
/// 창을 없애면 무엇이 잘못됐는지 알 방법이 없으므로 종료는 콜백에서 한다.
fn fail<R: Runtime>(app: &AppHandle<R>, title: &str, body: &str) {
    log::error!("startup check failed: {title}");
    let app_handle = app.clone();
    app.dialog()
        .message(body)
        .title(title)
        .kind(MessageDialogKind::Error)
        .show(move |_| app_handle.exit(1));
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::{BufRead, BufReader, Write};
    use std::net::{TcpListener, TcpStream};

    /// 주어진 상태 코드로만 답하는 최소 HTTP 서버를 띄우고 그 base URL을 돌려준다.
    /// probe가 실제 소켓·실제 reqwest를 거쳐 판정하는지 보기 위한 것이라 라우팅은 하지 않는다.
    fn stub_server(status_line: &'static str) -> (String, std::thread::JoinHandle<()>) {
        let listener = TcpListener::bind("127.0.0.1:0").expect("bind stub server");
        let addr = listener.local_addr().expect("stub server addr");
        // 한 요청만 받고 끝낸다 — probe가 판정을 내리는 데 필요한 응답은 하나뿐이다.
        let handle = std::thread::spawn(move || {
            let Ok((mut stream, _)) = listener.accept() else {
                return;
            };
            // 요청 헤더를 끝까지 읽어야(빈 줄) 클라이언트가 응답을 정상 수신한다.
            let mut reader = BufReader::new(stream.try_clone().expect("clone stream"));
            let mut line = String::new();
            while reader.read_line(&mut line).unwrap_or(0) > 0 {
                if line == "\r\n" || line == "\n" {
                    break;
                }
                line.clear();
            }
            let _ = stream.write_all(
                format!("{status_line}\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
                    .as_bytes(),
            );
            let _ = stream.flush();
        });
        (format!("http://{addr}"), handle)
    }

    fn client() -> reqwest::Client {
        reqwest::Client::builder()
            .timeout(REQUEST_TIMEOUT)
            .build()
            .expect("build client")
    }

    #[tokio::test]
    async fn probe_reports_ready_when_the_server_accepts_our_token() {
        let (base, server) = stub_server("HTTP/1.1 200 OK");
        let health = probe(&client(), &base, "tok", Duration::from_secs(5), || false).await;
        assert_eq!(health, Health::Ready);
        server.join().expect("stub server thread");
    }

    /// 남의 인스턴스가 포트를 쥔 상황: 401은 기다린다고 달라지지 않으므로 즉시 판정해야 한다
    /// — deadline을 넉넉히 줘도 곧바로 끝나는 것으로 "재시도하지 않음"을 함께 확인한다.
    #[tokio::test]
    async fn probe_reports_port_taken_on_unauthorized_without_retrying() {
        let (base, server) = stub_server("HTTP/1.1 401 Unauthorized");
        let started = Instant::now();
        let health = probe(&client(), &base, "tok", Duration::from_secs(30), || false).await;
        assert_eq!(health, Health::PortTaken);
        assert!(
            started.elapsed() < Duration::from_secs(2),
            "PortTaken should be decided on the first response, took {:?}",
            started.elapsed()
        );
        server.join().expect("stub server thread");
    }

    /// 사이드카가 죽었고 포트에도 아무도 없으면 deadline을 기다리지 않고 Down으로 끝난다.
    #[tokio::test]
    async fn probe_gives_up_immediately_when_the_sidecar_is_already_dead() {
        // 리스너를 닫아 아무도 듣지 않는 포트를 확보한다(즉시 연결 거부).
        let listener = TcpListener::bind("127.0.0.1:0").expect("bind");
        let addr = listener.local_addr().expect("addr");
        drop(listener);

        let started = Instant::now();
        let health = probe(
            &client(),
            &format!("http://{addr}"),
            "tok",
            Duration::from_secs(30),
            || true,
        )
        .await;

        assert_eq!(health, Health::Down);
        assert!(
            started.elapsed() < Duration::from_secs(2),
            "a dead sidecar should not wait out the deadline, took {:?}",
            started.elapsed()
        );
    }

    /// 살아는 있는데 끝내 응답하지 않으면 deadline까지 기다렸다가 Down.
    #[tokio::test]
    async fn probe_reports_down_after_the_deadline_when_nothing_ever_answers() {
        let listener = TcpListener::bind("127.0.0.1:0").expect("bind");
        let addr = listener.local_addr().expect("addr");
        drop(listener);

        let deadline = Duration::from_millis(400);
        let started = Instant::now();
        let health = probe(
            &client(),
            &format!("http://{addr}"),
            "tok",
            deadline,
            || false,
        )
        .await;

        assert_eq!(health, Health::Down);
        assert!(
            started.elapsed() >= deadline,
            "probe returned before the deadline: {:?}",
            started.elapsed()
        );
    }

    /// 토큰은 매 요청에 붙어야 한다 — 안 붙이면 내 사이드카조차 401을 주고, 그 결과
    /// "이미 실행 중"이라는 엉뚱한 안내가 뜬다.
    #[tokio::test]
    async fn probe_sends_the_session_token_as_a_bearer_header() {
        let listener = TcpListener::bind("127.0.0.1:0").expect("bind");
        let addr = listener.local_addr().expect("addr");
        let (tx, rx) = std::sync::mpsc::channel();
        std::thread::spawn(move || {
            let (stream, _) = listener.accept().expect("accept");
            let mut reader = BufReader::new(stream.try_clone().expect("clone"));
            let mut headers = Vec::new();
            let mut line = String::new();
            while reader.read_line(&mut line).unwrap_or(0) > 0 {
                if line == "\r\n" || line == "\n" {
                    break;
                }
                headers.push(line.trim_end().to_string());
                line.clear();
            }
            let mut stream: TcpStream = stream;
            let _ = stream
                .write_all(b"HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\n\r\n");
            let _ = tx.send(headers);
        });

        let health = probe(
            &client(),
            &format!("http://{addr}"),
            "session-token",
            Duration::from_secs(5),
            || false,
        )
        .await;
        assert_eq!(health, Health::Ready);

        let headers = rx.recv_timeout(Duration::from_secs(5)).expect("headers");
        assert!(
            headers
                .iter()
                .any(|h| h.eq_ignore_ascii_case("Authorization: Bearer session-token")),
            "request headers missing the bearer token: {headers:?}"
        );
        assert!(
            headers.iter().any(|h| h.starts_with("GET /v1/healthz ")),
            "probe must hit the authenticated liveness path: {headers:?}"
        );
    }
}
