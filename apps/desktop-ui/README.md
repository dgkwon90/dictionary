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

## 권장 IDE

- [VS Code](https://code.visualstudio.com/) + [Tauri](https://marketplace.visualstudio.com/items?itemName=tauri-apps.tauri-vscode) + [rust-analyzer](https://marketplace.visualstudio.com/items?itemName=rust-lang.rust-analyzer)
