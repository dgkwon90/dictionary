# 개발 · 빌드 · 실행 가이드

로컬에서 Neulsang을 **직접 실행해 테스트**하기 위한 실무 가이드. 제품 정의/스펙은 [`prd.md`](prd.md), 작업 대장은 [`planning/backlog.md`](planning/backlog.md)를 따른다. 여기서는 "어떻게 빌드하고 띄워서 확인하는가"만 다룬다.

> 대상 OS: **macOS 우선**(Apple Silicon 검증). Windows/Linux는 일부 동작(부모-사망 watchdog, 트레이 배지)이 다르다 — 각 절에 표시.

---

## 1. 사전 준비

| 도구 | 버전 | 용도 |
|---|---|---|
| Go | **1.26+** (`go.work`/`apps/desktopd/go.mod` 기준) | 백엔드 사이드카 `desktopd` |
| Node.js | 18+ (LTS 권장) | 프론트엔드 빌드 (Vite) |
| Rust toolchain | stable | Tauri 셸(`src-tauri`) |
| Tauri 2 사전요건 | — | macOS: Xcode Command Line Tools |
| golangci-lint | 최신 | Go 린트 게이트 |

```bash
# 확인
go version            # go1.26+
node -v && npm -v
rustc --version && cargo --version
golangci-lint version
```

프론트 의존성 설치(최초 1회):

```bash
npm --prefix apps/desktop-ui install
```

---

## 2. 저장소 구조 (실행 관점)

```
neulsang-dictionary/
├─ go.work                     # Go 워크스페이스 (apps/desktopd 포함)
├─ .env                        # 로컬 개발용 secret/설정 (gitignore) — 아래 3절
├─ apps/
│  ├─ desktopd/                # Go HTTP 사이드카 (백엔드)
│  │  └─ cmd/desktopd/main.go  #   → 실행 진입점
│  └─ desktop-ui/              # Tauri 2 + React (프론트 + 셸)
│     ├─ package.json          #   npm scripts (dev/build/build:sidecar)
│     └─ src-tauri/            #   Rust 셸 (트레이·사이드카 spawn·알림)
└─ docs/
```

핵심 원칙: **`desktopd` ↔ `desktop-ui`는 직접 import 금지, 로컬 HTTP API로만 통신**(기본 `127.0.0.1:48989`). 그래서 백엔드는 UI 없이도 curl로 단독 검증할 수 있다.

---

## 3. 설정 (config 계층)

설정은 두 계층으로 나뉜다(ADR-0004 부록, 백로그 #17):

1. **부트스트랩/인프라 (`.env` · 환경변수)** — DB 경로·주소·AI provider·모델·**API key**. 시작 시점에만 읽고, Settings 화면에선 **읽기전용**으로만 표시.
2. **동작 정책 (`app_settings` 테이블, `GET/PUT /v1/settings`)** — 알림 허용, 아침/저녁 복습 시각. 앱에서 편집·저장.

### 3.1 환경변수 전체 목록

| 변수 | 기본값 | 설명 |
|---|---|---|
| `NEULSANG_ADDR` | `127.0.0.1:48989` | 사이드카 HTTP 주소 |
| `NEULSANG_DB_PATH` | `<UserConfigDir>/neulsang/neulsang.db` | SQLite 경로. macOS=`~/Library/Application Support/neulsang/neulsang.db` |
| `NEULSANG_LOG_LEVEL` | `info` | `debug`/`info`/`warn`/`error` |
| `NEULSANG_AI_PROVIDER` | (자동) | `gemini`/`mock`. 미지정 시 API key 유무로 자동 선택 |
| `NEULSANG_GEMINI_API_KEY` | — | **secret. DB·파일·로그에 저장 안 함. env/.env 우선, 없으면 OS 키체인(#26, §6.1)** |
| `NEULSANG_GEMINI_MODEL` | `gemini-flash-lite-latest` | Gemini 모델 override |
| `NEULSANG_SYNC_URL` | (빈값=off) | 설정 시 sync_outbox를 이 URL로 전송(#20) |
| `NEULSANG_DESKTOPD_BIN` | (자동 탐색) | 셸이 spawn할 사이드카 바이너리 경로 override(Rust) |
| `NEULSANG_PARENT_PID` | (셸이 설정) | 부모-사망 watchdog 게이트. 수동 설정 불필요 |

### 3.2 `.env` 파일 위치와 로드 순서

`desktopd`는 시작 시 아래 **두 곳**에서 `.env`를 찾아 로드한다. 우선순위: **실 환경변수 > repo `.env`(dev) > 사용자 config `.env`(설치판)**.

1. **repo `.env`** — cwd에서 상위로 올라가며 탐색. 개발 중(`tauri dev`, `go run`)엔 저장소 루트 `.env`가 여기 걸린다.
2. **사용자 config `.env`** — `<UserConfigDir>/neulsang/.env` (macOS=`~/Library/Application Support/neulsang/.env`). **번들 `.app`을 Finder/`open`으로 실행하면 cwd=`/`라 repo `.env`에 못 닿는데, 이 경로가 그걸 해결한다(백로그 #25).**

> 저장소 루트 `.env` 예시(gitignore됨):
> ```dotenv
> NEULSANG_GEMINI_API_KEY=AIza...          # 없으면 mock provider로 동작
> NEULSANG_DB_PATH=/tmp/neulsang-dev.db    # 개발용 격리 DB(선택)
> # NEULSANG_AI_PROVIDER=mock              # 키 있어도 강제로 mock 쓰고 싶을 때
> ```

secret은 항상 gitignore된 `.env`(개발) 또는 사용자 홈의 config `.env`(설치판)에만 둔다. 커밋·DB·로그 금지.

---

## 4. 백엔드만 실행 + curl로 검증 (가장 빠른 루프)

UI 없이 API 단위로 확인하는 방법. 디버깅이 쉬워 백엔드 변경 검증에 권장.

```bash
# 저장소 루트에서 (repo .env가 여기서 walk-up으로 잡힌다)
cd apps/desktopd
go run ./cmd/desktopd
# → 127.0.0.1:48989 리슨, DB 자동 마이그레이션
```

다른 터미널에서:

```bash
# 헬스체크
curl -s 127.0.0.1:48989/healthz

# 캡처 생성 → 비동기 explain → 폴링 조회
CID=$(curl -s -XPOST 127.0.0.1:48989/v1/captures \
  -H 'content-type: application/json' \
  -d '{"input_text":"idempotent","input_mode":"manual"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')
sleep 3
curl -s "127.0.0.1:48989/v1/captures/$CID/explanation" | python3 -m json.tool

# 현재 설정(effective) 확인 — provider가 gemini인지 mock인지 여기서 확인
curl -s 127.0.0.1:48989/v1/settings | python3 -m json.tool
```

### 4.1 주요 엔드포인트 (curl 테스트용)

| 메서드 · 경로 | 용도 |
|---|---|
| `GET /healthz` | 헬스체크 |
| `POST /v1/captures` | 캡처 생성(검색 시작) |
| `GET /v1/captures/{id}/explanation` | 해석 결과 조회(준비 전엔 202/pending) |
| `GET /v1/captures/{id}/knowledge` | 캡처에서 추출된 단어 목록 |
| `GET /v1/inbox?status=new&limit=50` | Inbox (new/saved/review_added/archived/failed) |
| `POST /v1/inbox/{id}/save` · `/archive` | Inbox 상태 변경 |
| `POST /v1/knowledge/{id}/mark-unknown` · `/mark-known` | 모름/알아요 (모름 시 복습 카드 생성) |
| `GET /v1/reviews/due?limit=N` | due 복습 카드 |
| `POST /v1/reviews/session/start` | 세션 시작(=due 목록) |
| `POST /v1/reviews/{id}/grade` | 채점(Again/Hard/Good/Easy) |
| `GET /v1/dashboard/summary` | 대시보드 지표 |
| `GET /v1/suggest?q=스테일` · `POST /v1/suggest/confirm` | 한글 발음→영어 후보 |
| `GET /PUT /v1/settings` | 설정(preferences 편집 + effective 읽기전용) |
| `GET /v1/notifications` · `POST /v1/notifications/{id}/ack` | 알림 원장 |
| `GET /v1/export` · `POST /v1/import` · `POST /v1/backup` | 백업/내보내기/가져오기 |
| `GET /v1/sync/status` | outbox 동기화 상태 |

---

## 5. 프론트 + 백엔드 통합 실행 (`tauri dev`)

트레이·단축키·팝업·알림까지 실제로 확인하는 개발 실행.

```bash
npm --prefix apps/desktop-ui run tauri dev
```

- `beforeDevCommand`가 **먼저 Go `desktopd`를 재빌드**(`build:sidecar`)한 뒤 Vite dev + Tauri를 띄운다 → 백엔드 변경분이 매번 반영된다(Go 미설치면 경고만 하고 UI 단독 기동).
- 셸이 `desktopd`를 자식 프로세스로 spawn. 이때 cwd가 저장소 안이라 **repo `.env`가 로드된다**(→ API key 있으면 gemini).
- 글로벌 단축키 **`Cmd+Shift+E`**(mac) / `Ctrl+Shift+E`로 Quick Search 팝업.
- 창을 닫아도 트레이로 hide(완전 종료는 트레이 **Quit**).

> ⚠️ **알림(OS 배너)은 `tauri dev`(미서명 비번들)에서는 배달이 불안정**하다. 알림·트레이 배지까지 확인하려면 6절의 번들 실행을 쓴다(ADR-0008).

---

## 6. 번들 `.app` 빌드 & 실행 (알림·배포형 확인)

```bash
# 디버그 번들 (빠름) — 릴리스는 --debug 빼기
npm --prefix apps/desktop-ui run tauri build -- --debug --bundles app
# 산출물: apps/desktop-ui/src-tauri/target/debug/bundle/macos/Neulsang.app
```

**번들은 자기완결형이다(#30 externalBin).** `beforeBuildCommand`가 `build:sidecar`를 돌려
`desktopd`를 `src-tauri/binaries/desktopd-<target-triple>`(예: `-aarch64-apple-darwin`)로 만들고,
Tauri `bundle.externalBin`이 이를 `.app/Contents/MacOS/desktopd`로 복사·서명한다. Rust 셸의
바이너리 탐색 "실행 파일 옆" 분기가 이걸 찾아 spawn한다 → **다른 기기에서도 백엔드 동작**.
서명은 **애드혹**(`bundle.macOS.signingIdentity: "-"`) — 이 Mac에선 바로 실행, 다른 Mac에
전달 시 Gatekeeper가 막아 **우클릭 → 열기**가 필요(정식 배포는 Developer ID + 공증, 아래 6.3).
`binaries/`는 gitignore(빌드 산출물). 확인:

```bash
codesign --verify --deep --verbose=2 apps/desktop-ui/src-tauri/target/debug/bundle/macos/Neulsang.app
# → "valid on disk" / "satisfies its Designated Requirement"
```

### 6.1 설치판처럼 config 주입하기 (#25)

번들 `.app`을 **정상적으로**(Finder 더블클릭 / `open`) 실행하면 cwd=`/`라 repo `.env`에 못 닿는다. 실사용 config는 **사용자 config `.env`**에 둔다:

```bash
# 한 번만 만들어 두면 이후 번들 실행이 자동으로 읽는다
mkdir -p ~/Library/Application\ Support/neulsang
cat > ~/Library/Application\ Support/neulsang/.env <<'EOF'
NEULSANG_GEMINI_API_KEY=AIza...
EOF
chmod 600 ~/Library/Application\ Support/neulsang/.env

open apps/desktop-ui/src-tauri/target/debug/bundle/macos/Neulsang.app
```

- 최초 실행 시 macOS **알림 권한 허용** 필요.
- provider가 gemini로 잡혔는지는 앱 **Settings 화면의 effective** 또는 `curl 127.0.0.1:48989/v1/settings`로 확인.
- **secret 주의**: 이 파일은 평문이다 — 사용자 홈 소유(0600), 커밋/DB/export 대상 아님.
- **(대안) OS 키체인에 API key 보관(#26)**: 평문 `.env` 대신 macOS Keychain에 두면 `.env`에서 `NEULSANG_GEMINI_API_KEY`를 빼도 desktopd가 키체인에서 읽는다(우선순위: env > `.env` > 키체인). 등록/삭제:
  ```bash
  security add-generic-password -s neulsang -a gemini_api_key -w '<KEY>'      # 등록
  security add-generic-password -s neulsang -a gemini_api_key -w '<KEY>' -U   # 덮어쓰기
  security delete-generic-password -s neulsang -a gemini_api_key              # 삭제
  ```

### 6.2 (대안) 개발 `.env`로 번들 실행

번들 안 바이너리를 직접 띄우면서 저장소 `.env`를 주입해 빠르게 확인:

```bash
cd /Users/<you>/.../neulsang-dictionary   # repo 루트 (cwd 중요)
set -a; . ./.env; set +a
apps/desktop-ui/src-tauri/target/debug/bundle/macos/Neulsang.app/Contents/MacOS/neulsang &
```

> 이 방식은 번들 안 `neulsang`을 직접 실행하므로, 옆에 함께 번들된 `Contents/MacOS/desktopd`를
> spawn한다(별도 desktopd 실행 불필요). `curl 127.0.0.1:48989/healthz` → `{"status":"ok"}`면 사이드카 정상.

### 6.3 정식 배포 서명 (Developer ID + 공증) — 후속

현재는 애드혹 서명(6절)이라 다른 Mac에서 Gatekeeper 경고가 뜬다. 경고 없는 배포는 **유료 Apple
Developer 계정($99/년)**이 필요하다. 준비되면:

1. `security find-identity -v -p codesigning`로 "Developer ID Application: …" 인증서 확인(없으면 발급).
2. `bundle.macOS.signingIdentity`를 그 인증서 이름으로 교체.
3. 공증: 환경변수 `APPLE_ID`/`APPLE_PASSWORD`(앱 암호)/`APPLE_TEAM_ID`(또는 `APPLE_API_KEY`/`APPLE_API_ISSUER`/`APPLE_API_KEY_PATH`)를 주면 `tauri build`가 공증까지 자동. 없으면 `Warn skipping app notarization`만 나온다(지금 상태).

---

## 7. 검증 게이트 (커밋 전)

### 백엔드 (`apps/desktopd`)
```bash
cd apps/desktopd
go build ./...
go test -race ./...
go vet ./...
golangci-lint run ./...      # "0 issues" 목표
```

### 프론트 (`apps/desktop-ui`)
```bash
npm --prefix apps/desktop-ui run build       # tsc + vite build
```

### Rust 셸 (`src-tauri`)
```bash
cd apps/desktop-ui/src-tauri
cargo check
cargo clippy --all-targets -- -D warnings
cargo fmt --check
```

---

## 8. 자주 겪는 함정

| 증상 | 원인 · 해결 |
|---|---|
| **번들 앱이 "목업 해석"만 냄** | `open`으로 띄우면 cwd=`/`라 repo `.env` 미로드 → 사용자 config `.env`(6.1)에 API key를 둔다 |
| **백엔드 변경이 dev에 반영 안 됨** | `tauri dev`의 `build:sidecar`가 재빌드하지만, 이미 떠 있던 사이드카를 쓰면 구버전 — dev를 재시작하거나 `desktopd`를 수동 재빌드 |
| **DB를 봤는데 데이터가 없음** | 앱이 쓰는 DB 경로 착각. `NEULSANG_DB_PATH`(또는 기본 `~/Library/Application Support/neulsang/neulsang.db`) 확인 — `curl .../v1/settings`의 `effective.db_path`가 정답 |
| **날짜/시간 비교가 어긋남(알림·복습)** | modernc SQLite가 `time.Time`을 타임존 포함 문자열로 저장 → 새 쿼리는 경계에서 `.UTC()` 정규화 필수 |
| **알림 배너가 dev에서 안 뜸** | 미서명 비번들 한계. 6절 번들 `.app` + 권한 허용으로 확인 |
| **트레이 ● 안 지워짐(mac)** | macOS는 `set_title(None)`로 안 지워짐 → 빈 문자열로 클리어(구현됨). 메인 창 포커스 시 ack로 클리어 |
| **`desktopd` 고아 프로세스(mac)** | 셸 비정상 종료 시 watchdog(`NEULSANG_PARENT_PID`)가 재입양 감지로 종료. Windows는 미동작(후속) |

---

## 9. 참고 문서

- [`prd.md`](prd.md) — 제품/스키마/API 원본 (충돌 시 우선)
- [`planning/backlog.md`](planning/backlog.md) — 이슈·마일스톤·의존 그래프
- [`adr/`](adr/README.md) — 결정 기록 (특히 ADR-0004 provider, ADR-0005 React, ADR-0008 알림)
- [`glossary.md`](glossary.md) — 용어집
