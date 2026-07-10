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

화면 알맹이는 프론트엔드 트랙(#14~#17)에서 채운다.

## 개발

```sh
# 사전: Rust(stable), Node 20+, desktopd 빌드(선택, 연결 표시용)
#   (cd ../desktopd && go build -o desktopd ./cmd/desktopd)
npm install
npm run tauri dev      # 셸 + 프론트엔드 동시 기동
```

`npm run build`는 프론트엔드(tsc+vite)만 빌드한다. 셸 검증은 `src-tauri`에서
`cargo check` / `cargo clippy` / `cargo fmt`.

## 직접 테스트 (수동)

GUI 수용 기준은 헤드리스로 못 돌리므로 아래 순서로 확인한다.

**0. 준비**
```sh
source "$HOME/.cargo/env"                        # 새 터미널마다
(cd ../desktopd && go build -o desktopd ./cmd/desktopd)   # 연결 표시용(선택)
```

**1. 실행** — `npm run tauri dev` (최초는 Rust 컴파일로 수 분). 창(`Neulsang 늘상`)과
메뉴 막대 트레이 아이콘이 뜬다.

**2. 트레이 네비게이션(수용 기준)** — 트레이 아이콘 클릭 → 각 항목:

| 클릭 | 기대 |
|---|---|
| Quick Search / Inbox / Today Review / Dashboard / Settings | 메인 창이 앞으로 뜨고 상단 탭·제목이 해당 화면으로 전환 |
| Quit | 앱 종료(+ desktopd 함께 종료) |

하단 상태줄: desktopd 빌드 시 초록 "desktopd 연결됨", 아니면 빨간 "미연결".

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
