# desktop-ui

Neulsang(늘상) 데스크톱 UI. **Tauri 2 + React + TypeScript + Vite** (ADR-0005).
트레이·기본 윈도우·화면(Quick Search/Inbox/Review/Dashboard/Settings)을 담당하고,
`apps/desktopd`와는 로컬 HTTP API로만 통신한다(직접 import 금지, PRD §15).

## 셸 구성 (#13)

- **트레이 메뉴**(PRD §10.1): Quick Search / Inbox / Today Review / Dashboard / Settings / Quit.
  항목 클릭 시 메인 윈도우를 띄우고 `navigate` 이벤트로 해당 화면으로 전환. Quit은 앱 종료.
- **desktopd 사이드카 생명주기**: 앱 기동 시 자식 프로세스로 실행, 종료 시 함께 정리
  (`src-tauri/src/sidecar.rs`). 바이너리 탐색 순서 = 환경변수 `NEULSANG_DESKTOPD_BIN`
  → 실행 파일 옆 → 개발용 `../../desktopd/desktopd`. 못 찾으면 UI만 단독 기동.
- **API 클라이언트 골격**: `src/api/client.ts` (`DesktopdClient`, 기본 주소 `127.0.0.1:48989`).

## Quick Search 팝업 (#14)

- **글로벌 단축키** `Cmd/Ctrl+Shift+E`(및 트레이 Quick Search)로 프레임리스 팝업 윈도우
  (`quicksearch`)를 띄운다. `src-tauri/src/popup.rs` + `lib.rs`(global-shortcut 등록).
- 팝업은 열릴 때 **클립보드 자동 삽입**, 입력 후 `POST /v1/captures` → 해석 폴링 →
  결과 표시(`src/quicksearch/QuickSearch.tsx`). `main.tsx`가 윈도우 라벨로 렌더를 분기한다.

## Inbox (#15)

메인 윈도우 Inbox 탭(`src/inbox/Inbox.tsx`): New/Saved/Review Added/Archived/Failed
5탭, 행을 펼치면 그 캡처의 추출 단어를 단어별 **모름**(복습 카드 생성)·**알아요**로
표시한다. 백엔드 `GET /v1/captures/{id}/knowledge`로 단어의 knowledge_item id를 받는다.

## Today Review (#16)

메인 윈도우 Today Review 탭(`src/review/Review.tsx`): `GET /v1/reviews/due`로 지금 복습할
카드를 받아 질문만 보여주고, **답 보기**(Space)로 답/설명을 공개한 뒤
**Again/Hard/Good/Easy**(키 1~4)로 채점하면 `POST /v1/reviews/{id}/grade` 후 다음 카드로
넘어간다. due 응답은 자가 채점을 위해 `answer`/`explanation`을 포함한다(로컬 단일 사용자).

## Dashboard (#17)

메인 윈도우 Dashboard 탭(`src/dashboard/Dashboard.tsx`): `GET /v1/dashboard/summary`를
읽어 오늘/이번 주 검색 수, 오늘 복습 완료·due 카드 수, 가장 많이 검색/자주 틀린 단어,
카테고리별 약점(막대)을 읽기전용으로 표시한다. (PRD §10.6의 "최근 7일 추세"는 요약
API에 시계열이 없어 후속.)

## Settings (#17)

메인 윈도우 Settings 탭(`src/settings/Settings.tsx`): `GET/PUT /v1/settings`. 설정을 두
계층으로 나눈다(ADR-0004 부록). **preferences**(알림 허용·아침/저녁 복습 시간)는
편집·저장(`app_settings`), **effective**(AI provider·모델·DB 경로·주소·API key 유무)는
`.env`로만 설정하는 읽기전용. API key는 값이 아니라 설정 여부만 표시한다. 복습 시간은
저장되지만 실제 알림 구동은 #18 소관.

## 개발

```sh
# 사전: Rust(stable), Node 20+, Go(desktopd 자동 빌드용)
npm install
npm run tauri dev      # 셸 + 프론트엔드 동시 기동
```

`npm run tauri dev`는 시작 시 `beforeDevCommand`로 `npm run build:sidecar`를 먼저 돌려
**desktopd Go 바이너리를 자동 재빌드**한다(백엔드 변경분을 매번 반영 — 안 하면 구버전
사이드카가 떠서 새 API 필드가 안 내려온다). Go가 없으면 경고만 하고 기존 바이너리로
UI만 기동한다. 수동 재빌드는 `npm run build:sidecar`.

`npm run build`는 프론트엔드(tsc+vite)만 빌드한다(사이드카 제외). 셸 검증은 `src-tauri`에서
`cargo check` / `cargo clippy` / `cargo fmt`.

## 직접 테스트 (수동)

GUI 수용 기준은 헤드리스로 못 돌리므로 아래 순서로 확인한다.

**0. 준비**
```sh
source "$HOME/.cargo/env"                        # 새 터미널마다
# desktopd는 tauri dev가 자동 빌드(build:sidecar). Go만 설치돼 있으면 됨.
```

**1. 실행** — `npm run tauri dev` (최초는 Rust 컴파일로 수 분). 창(`Neulsang 늘상`)과
메뉴 막대 트레이 아이콘이 뜬다.

**2. 트레이 네비게이션(수용 기준)** — 트레이 아이콘 클릭 → 각 항목:

| 클릭 | 기대 |
|---|---|
| Quick Search / Inbox / Today Review / Dashboard / Settings | 메인 창이 앞으로 뜨고 상단 탭·제목이 해당 화면으로 전환 |
| Quit | 앱 종료(+ desktopd 함께 종료) |

하단 상태줄: desktopd 빌드 시 초록 "desktopd 연결됨", 아니면 빨간 "미연결".

**2-1. Quick Search 팝업(#14)** — AI 해석까지 보려면 루트 `.env`에 `NEULSANG_GEMINI_API_KEY` 필요.
- **글로벌 단축키** `Cmd+Shift+E`(mac)/`Ctrl+Shift+E` — 다른 앱이 앞에 있어도 팝업이 뜬다.
- 팝업은 **클립보드 내용을 자동 삽입**(직접 수정·입력도 가능). Enter → "해석 중…" →
  브리프/발음/상세/예문/하위 단어 표시. `Esc`로 닫기. 트레이 "Quick Search"로도 열린다.

**3. 사이드카 정상 종료** — 앱 실행 중 `pgrep -x desktopd`로 PID 확인 → 트레이 Quit
→ 다시 `pgrep -x desktopd`가 비면 정상(자식 정리됨).

**4. Watchdog(비정상 종료 시 고아 방지)** — 앱 실행 중 별도 터미널:
```sh
pgrep -x desktopd                    # 사이드카 살아있음
pkill -9 -f target/debug/neulsang    # Tauri 셸(desktopd의 부모)을 SIGKILL
sleep 3
pgrep -x desktopd && echo "❌ 고아 남음" || echo "✅ watchdog 동작"
```
셸을 강제 종료해도 desktopd가 재입양(부모 PID 변화)을 2초 내 감지해 스스로 종료한다.
(프로세스명이 안 잡히면 `pgrep -fl neulsang`으로 확인.)

**5. 로직 단위 검증(GUI 없이)** — `(cd ../desktopd && go test ./internal/watchdog/ -v)`.

## 권장 IDE

- [VS Code](https://code.visualstudio.com/) + [Tauri](https://marketplace.visualstudio.com/items?itemName=tauri-apps.tauri-vscode) + [rust-analyzer](https://marketplace.visualstudio.com/items?itemName=rust-lang.rust-analyzer)
