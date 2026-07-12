# ADR-0008: 사이드카 → UI 이벤트 전달·알림 방식

- 날짜: 2026-07-12 / 상태: 승인 (codex·agy 교차조사 반영)

## 맥락 (무엇을 결정해야 했나)
PRD §7.1(알림 표시=Tauri)과 §8.1(Notification=Go sidecar)이 알림 주체를 서로 다르게 그려 두었고(백로그 "PRD 갭 3"), §15의 로컬 API는 **UI→sidecar 단방향 요청**뿐이다. 그래서 desktopd(Go 사이드카)가 만든 **비동기 이벤트**를 UI가 알 방법이 정의돼 있지 않다. 백로그 #18(알림·리마인더) 착수를 위해 이 전달 방식을 먼저 결정해야 한다.

전달해야 할 사이드카발 이벤트는 두 종류:
1. **result_ready**: AI 해석(explain goroutine)이 완료됨(검색 후 초 단위. Quick Search 팝업은 이미 직접 폴링 중이나 팝업을 닫았을 수 있음).
2. **review_due**: 사용자가 설정한 아침/저녁 시각(#17 `app_settings`)에 due 복습 카드가 있을 때.

전달 표면(사용자에게 보이는 방식)은 **OS 네이티브 알림 + 시스템 트레이 "New" 표시** 둘 다로 확정됐다.

제약:
- desktopd ↔ desktop-ui는 로컬 HTTP로만 통신(PRD §15, ADR-0002). 직접 import 금지.
- desktopd는 Tauri 셸의 **자식 프로세스**라 셸과 함께 죽는다(watchdog, #13). 앱을 완전 종료하면 사이드카도 없으므로 **앱이 살아있을 때(트레이 상주 포함)만** 알림이 가능하다.
- 앱은 **창 닫기=hide-to-tray**(9c70819). 메인 창을 닫아도 앱은 트레이에 상주.
- 단일 사용자·로컬. 이벤트 빈도가 매우 낮음(검색은 초 단위, 리마인더는 하루 2회).

## 결정

### 전달 방식: 폴링 + SQLite 알림 원장 + **Rust 셸 소유 루프** (안 A′)

**1. 루프는 React webview가 아니라 Rust 셸이 소유한다 (비협상).**
- 두 트리거는 **정확히 메인 창이 닫혀 있을 때** 발생하기 쉽다. 폴/알림 루프를 webview(`App.tsx`의 `setInterval`)에 두면 창을 트레이로 숨기는 순간 JS 런타임이 중단/파괴돼 알림이 **조용히 멈춘다**.
- 따라서 `tauri::async_runtime::spawn`으로 셸에 백그라운드 태스크를 띄워 `reqwest`로 폴링하고, `tauri-plugin-notification`의 **Rust API**와 트레이 핸들을 직접 호출한다. 창 존재와 무관하게 동작.

**2. 전송은 고정 간격 폴링(약 5~10초). SSE/WebSocket은 MVP 제외.**
- 이벤트 빈도가 사실상 0이라 push의 저지연 이점이 붕괴한다(리마인더는 하루 2회, result_ready는 팝업이 이미 직접 폴링하는 fast-path 존재). 폴링은 연결 생명주기·재연결·keep-alive가 없어 desktopd를 순수 요청-응답으로 유지한다.
- **long-poll(Syncthing `GET /rest/events` 방식)** 을 "지연이 실제 문제가 되면 도입할 값싼 업그레이드"로 기록. SSE는 라이브 진행률 UI 같은 고빈도 이벤트가 생기면 재검토.

**3. 서버측 알림 원장 = SQLite `notifications` 테이블.**
- 어떤 전송을 쓰든 **dedup·ack·coalesce·배지 카운트 소스**가 서버측에 필요하다(비영속 링버퍼도 결국 같은 상태를 가짐). 코드베이스가 이미 전부 SQLite+마이그레이션 grain이고, result_ready를 explanation write와 원자적으로 묶을 수 있어 테이블로 둔다.
- 마이그레이션 `0005_notifications.sql` 1개 추가. 배지 카운트 = `SELECT count(*) WHERE acked_at IS NULL`.
- API: `GET /v1/notifications`(미확인 목록) + `POST /v1/notifications/{id}/ack`.
- **coalesce**: 한 문장에서 단어가 여러 개 추출돼도 capture 하나당 result_ready **1건**. review_due는 슬롯당 1건.

**4. "지난 알림 재발" 방지(영속의 유일한 부작용 무력화).**
- **result_ready**: 짧은 TTL(예: 미확인 N분 후 만료) 또는 세션 스코프 — 재시작 후 어제 검색의 "준비됨"이 뜨지 않게. insert는 explain `SaveSuccess` 트랜잭션에 묶어 at-least-once.
- **review_due**: `(date, slot)` 멱등 키 — 재시작해도 중복 발화 안 함. 여전히 due면 재알림이 오히려 옳음.

**5. 트레이 "New" 표시: 숫자 배지 대신 아이콘/타이틀.**
- macOS 메뉴바 트레이는 **숫자 배지 오버레이를 지원하지 않는다.** `TrayIcon.set_title(Some("•"))`(메뉴바 텍스트) 또는 dot이 그려진 에셋으로 `set_icon` 교체로 표시. Dock/taskbar 배지(`set_badge_count`/Windows `set_overlay_icon`)는 트레이와 별개 표면이라 2차로 검토.

**6. 리마인더 스케줄러는 Go(desktopd)에 유지.** 시간 로직을 순수함수로 테스트 가능하게(FSRS-lite와 동일 원칙), webview와 독립 발화.

### 기각안
- **C안(사이드카가 OS 알림 직접 발송, beeep 등)**: 트레이 배지·클릭→화면 라우팅 불가, 알림 코드 이원화, 미서명 Go 바이너리의 macOS 알림 동작이 앱 번들보다 나쁨. 요구(트레이 배지 + 클릭 시 Review로 이동)를 충족 못 함.
- **B안(SSE/WebSocket push)**: 저지연이 이 워크로드에 불필요하고, 자식 프로세스에 장수명 스트리밍 연결과 Rust SSE 클라이언트 의존성을 추가한다. 후속 여지로만 문서화.
- **in-memory 링버퍼**: 마이그레이션은 아끼나 result_ready를 explanation write와 원자적으로 묶지 못하고 배지 카운트 쿼리 이점이 없음. SQLite 테이블 채택.

## 근거 (교차조사)
- codex·agy 교차조사 모두 **표준 관행 = 지속 채널 push**(LSP JSON-RPC/stdio, Docker `/events` 스트리밍, Syncthing `/rest/events` long-poll)이고 고정 폴링은 실용적 최소선임을 확인. 동시에 이 워크로드(이벤트 빈도 ≈ 0)에선 폴링으로 충분하다는 데 수렴.
- **양쪽이 독립적으로 지목한 최중요 결함**: 원안이 "기존 폴 루프 확장"이라 했으나 그 루프가 webview에 있어, hide-to-tray 시 알림이 죽는다 → **루프를 Rust 셸로 이동**이 1순위.
- 원장 영속 여부에서 codex(테이블 유지·목적 재정의)와 agy(in-memory)가 갈렸으나, 둘 다 서버측 dedup/ack/coalesce 상태가 필요하다는 데 동의. result_ready↔write 원자성과 코드베이스 grain을 이유로 SQLite 테이블 채택하고, 영속의 부작용은 TTL/멱등 키로 상쇄.

## 결과·트레이드오프
- **범위·한계(ADR에 명시)**: 이 방식은 **앱이 살아있을 때(트레이 상주 포함)만** 알림을 전달한다. 앱을 완전 종료하면 사이드카도 없어 알림이 없다 — "앱 종료 상태 리마인더"는 OS 레벨 스케줄링(launchd/agent 등)이 필요하며 MVP 범위 밖.
- **장점**: desktopd 순수 요청-응답 유지, 새 Rust 의존성 최소(reqwest+notification 플러그인), 마이그레이션 1개, 배지 카운트·coalesce·ack가 SQL로 단순. 나중에 long-poll/SSE로 바꿔도 알림 원장·렌더 계층은 그대로(되돌리기 쉬움).
- **트레이드오프(모니터링)**: 폴 간격만큼 result_ready 지연(팝업이 열려 있으면 fast-path라 무관). 5~10초가 체감상 느리면 long-poll 도입.
- **착수 전 실측 확인**(구현 이슈 #18에서): (a) 핀된 Tauri 2 버전에서 트레이 `set_title`/`set_icon` 및 Dock/taskbar `set_badge_count`/`set_overlay_icon` 심볼 시그니처(2.x에서 이동함), (b) `tauri-plugin-notification` 권한 요청·미서명 dev 빌드 배달 동작, (c) hide-to-tray가 webview JS 타이머를 실제로 중단시키는지(루프를 Rust로 옮기는 근거의 부하 지점). (b)·서명 이슈로 dev(`tauri dev`)에선 알림이 불안정할 수 있어 서명 `.app`로 검증.
