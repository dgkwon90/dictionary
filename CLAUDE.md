# Neulsang(늘상) — 로컬 우선 개발자 영어 학습 앱

## 프로젝트 개요
업무 중 마주친 영어 단어·용어·문장을 단축키로 즉시 검색·AI 해석하고, 그 기록을 복습 카드로 전환하는 로컬 우선(local-first) 데스크톱 앱.
**제품명은 Neulsang(한글 "늘상")으로 확정** (`docs/adr/ADR-0006-product-name.md`). 영문은 `Neulsang`, 한글 병기는 "늘상". 컴포넌트·바이너리명(`desktopd` 등)은 제품명과 별개다.

**제품 의도의 기준 문서: `docs/prd.md`** (무엇을 왜 만드는가 — 기능 판단이 갈리면 이 문서가 우선).
단, **스키마·API·화면 구성은 코드가 사실이다**(`internal/db/migrations/`·`router.go`·`src/`). 재설계 v2(2026-08-05)로 PRD의 해당 절들이 v1 서술로 남았기 때문이다 — 재설계 결정은 `docs/adr/ADR-0010-product-redesign-v2.md`.
문서 지도: `docs/README.md` / 용어: `docs/glossary.md` / 결정 기록: `docs/adr/`

## 개발 전략 (핵심 원칙)
"작게 설계 → 구현 → 검증 → 확장" 반복. `docs/planning/backlog.md`의 마일스톤(v0.1 ~ v0.5) 순서를 따른다.
모든 기능 판단 기준(PRD §21): **"이 기능이 사용자의 영어 기억을 더 잘 쌓고, 다시 떠올리게 하는가?"** — 아니면 MVP에서 뺀다.

## 팀
1인 개발(dgkwon90). 사람 리뷰어가 없으므로 codex/agy 교차 검토가 실질적인 2차 검증 수단이다.

## 일하는 방식 (상세는 docs/rules/)
- **사이클**: 이슈(1~2일 크기) → 설계 → TDD → 구현 → lint·취약점 검사 → (중요 결정만) codex/agy 교차검토 → 확정 (`rules/development-cycle.md`)
- **작업 관리**: `docs/planning/backlog.md`가 이슈 대장
- **AI·토큰 효율**: tri-review는 ADR급 결정에만, 범위는 파일 단위로 명시, 재리뷰 금지 (`rules/ai-collaboration.md`)
- **용어**: 새 용어는 `docs/glossary.md`에 등재, 같은 개념에 두 단어 금지

## 기술 스택
- Backend sidecar: **Go** (`apps/desktopd`) — HTTP API + SQLite + AI provider 연동 + 복습 스케줄
- Desktop UI: **Tauri 2 + TypeScript + React** (`apps/desktop-ui`) — `docs/adr/ADR-0005-frontend-framework.md`
- 로컬 DB: **SQLite** (WAL 모드. FTS5는 드라이버가 지원하지만 아직 안 쓴다 — 전문검색 도입 시 마이그레이션 필요)
- AI Provider: `Explainer` 인터페이스로 추상화, 1차 실연동 provider는 **Gemini** (`docs/adr/ADR-0004`)

## Repo 구조
```
apps/{desktopd, desktop-ui, api(추후 중앙서버)}   deploy/   scripts/   docs/
```
1. `apps/desktopd/internal/domain/`은 infra(AI provider, DB 드라이버 등)를 직접 알면 안 된다 — interface로 주입 (PRD §18.1)
2. `apps/desktopd` ↔ `apps/desktop-ui`는 직접 import 금지 — 로컬 HTTP API로만 통신 (PRD §15)
3. SQLite 스키마 변경은 migration 파일로만
4. API key 등 secret은 평문 저장·커밋 금지
5. **원격 push는 사용자가 명시 지시할 때만**

## AI 오케스트레이션
Claude가 중심, `.claude/agents/`의 codex-worker(구현 위임)·agy-worker(대량 분석)를 작업자로. 위임 결과는 diff/파일 직접 검증 후 채택. `--dangerously-*` 플래그는 사용자가 해당 대화에서 명시 허락 시만. 되돌리기 어려운 결정만 `/tri-review`.

## 현재 상태 (2026-08-05, 재설계 v2 완료)

### 제품 모델 (v1과 다름 — ADR-0010)
**검색 기록**과 **학습 목록**은 다른 것이다. 검색은 자유롭게 쌓이고, 학습 목록에는 사용자가 명시적으로 담은 것만 들어간다(`captures.triage_state`: unseen → needs_selection(문장) → learning/discarded). v1의 "검색하면 자동으로 학습 대상" 모델은 폐기됐다 — 그래서 `mark-unknown` API도, 인박스 5탭(새 것/저장/보관/복습할 것/실패)도 없다.
단어는 [학습할래요] 한 번, 문장은 [모르는 단어 고르기 → 완료]로 등록된다(문장 자체 + 고른 단어들이 함께, 단어마다 그 문장을 문맥으로 하는 cloze 카드). 단어/문장 판정은 서버 자동 + 결과 화면에서 사용자가 뒤집기(D1). 정답률은 `correct_count/attempt_count` **계산값**이고 `again`만 오답(D5). `mastery_score`는 삭제됐고 `wrong_count`는 `unknown_count`(=모른다고 선언한 횟수)로 개명됐다.

### 코드 지도
- **도메인**(`internal/domain/`, infra 무의존): capture·explain·search(검색 기록·분류·재해석)·learning(학습 목록·제외·되돌리기)·review(복습+연습+간격)·knowledge(NormalizeKey 등 공용)·notification·settings·stats·suggest·backup·outbox
- **화면**(`apps/desktop-ui/src/`): 검색 기록 / 학습 목록 / 오늘 복습 / 연습 / 알림 / 내 기록 / 설정 + Quick Search 팝업(별도 윈도우)
- **API**: `internal/transport/http/router.go`가 전체 목록. 스키마는 `internal/db/migrations/`가 사실이다(PRD §11은 v1 서술이 남아 있어 신뢰하지 말 것).
- **마이그레이션 3개**: 0001_init(v2 스키마 — 재작성됨, v1 DB는 checksum 불일치로 기동 거부), 0002_rename_inbox_route, 0003_notification_soft_delete

### 인프라·운영 지식 (v1에서 그대로 유효)
- **AI**: Gemini(`internal/infra/llm/gemini`), 기본 모델 `gemini-flash-lite-latest`, 호출당 20s×최대 3회. **API key는 `NEULSANG_GEMINI_API_KEY` env로만**(DB·파일·로그 금지). 우선순위: 실 env > repo `.env` > `<UserConfigDir>/neulsang/.env` > OS 키체인(#26). `NEULSANG_AI_PROVIDER=mock`로 오프라인 개발.
- **실 provider 검증의 가치**: mock은 늘 완벽한 응답이라 flash-lite의 범위 이탈(difficulty/importance)·빈 enum·필수 필드 누락이 안 보인다 → `parseResponse` clamp + responseSchema의 `required`/`enum`/`minItems`로 방어한다. AI 계약을 바꾸면 **반드시 실 키로 확인**할 것.
- **modernc sqlite는 time.Time을 tz 포함 문자열로 저장** → 시각은 항상 `utc()` 헬퍼로 바인딩(안 하면 DATETIME 비교가 조용히 깨진다). 커넥션은 `SetMaxOpenConns(1)`.
- **알림**(ADR-0008): 폴링 + `notifications` 원장 + **Rust 셸 소유** 루프(창이 닫혀도 동작). route 문자열(`"Search History"`/`"Today Review"`)은 Go·Rust·프론트 3언어 계약이고 **DB에 저장된다** — 개명하려면 세 곳 + 저장된 행(마이그레이션)을 함께 바꾼다. 삭제는 소프트(0003): dedup_key가 재발화를 막으므로 행을 지우면 지운 알림이 되살아난다.
- **알림 클릭(macOS만 다르다)**: `tauri-plugin-notification`은 데스크톱에서 클릭 콜백을 주지 않는다. 그래서 **macOS는 플러그인을 우회해 `mac-notification-sys`로 직접 보내고**(`notifications.rs`의 `mac` 모듈) `wait_for_click`으로 클릭을 받아 그 알림의 route로 이동한다. 발송 API는 결국 같다(플러그인 → notify-rust → 같은 크레이트 → NSUserNotification). 주의 둘: ① `set_application`은 프로세스 전역 `Once`라 **우리가 먼저 불러야** 배너가 Neulsang 이름으로 뜬다(안 부르면 Finder), ② `mac-notification-sys` 버전이 갈려 사본이 둘이 되면 클릭 응답이 다른 쪽 static 맵으로 가 조용히 사라진다. Windows/Linux는 플러그인 그대로 = **클릭해도 이동 없음**. 클릭이 도착하는지는 `~/Library/Logs/com.dgkwon90.neulsang/Neulsang.log`에서 `notification activated: type=2`로 확인한다.
- **같은 bundle id 사본이 여럿이면 알림 라우팅이 흔들린다**: `target/{debug,release}/bundle`·마운트된 DMG·휴지통 사본이 전부 LaunchServices에 등록된다(`lsregister -dump | grep Neulsang.app`). 알림을 실측하기 전에 사본 하나만 남길 것.
- **패키징**: `.app`은 자기완결형(사이드카 `externalBin` 번들 + 애드혹 서명, #31). 번들 실행 시 cwd=`/`라 사용자 config `.env`를 읽는다(#25). 사이드카는 `scripts/build-sidecar.mjs`가 `TAURI_ENV_TARGET_TRIPLE` → GOOS/GOARCH로 크로스컴파일한다(cgo-free라 `CGO_ENABLED=0`으로 충분).
- **릴리스는 `v*` 태그 push로만 시작된다**(#32) — main 머지로는 안 된다(머지 때 도는 건 `quality.yml`뿐). `release.yml`은 quality 재실행 → **draft** 릴리스 1개 생성 → mac arm64/x86_64 + Windows 병렬 빌드가 그 draft에 자산 업로드 → 셋 다 성공하면 **자동 공개**. draft를 먼저 만드는 4-job 구조는 매트릭스 leg 셋이 같은 릴리스를 동시에 만들려다 충돌하는 걸 피하려는 것이다. 주의 셋: ① 한 플랫폼만 깨져도 릴리스가 **draft에 갇힌다**(수동 Publish하거나 고쳐서 태그를 다시 보낸다), ② 사람이 받아보고 공개하려면 `publish-release` job을 지우면 draft로 남는다, ③ **태그명과 `tauri.conf.json` 버전이 맞는지 검사하는 단계가 없다** — 어긋나면 릴리스 태그와 자산 파일명이 갈리고 아무도 안 막는다. 버전은 5곳을 함께 올린다(`package.json`·`package-lock.json`·`Cargo.toml`·`Cargo.lock`·`tauri.conf.json`). updater를 안 쓰므로 서명 시크릿은 필요 없다(mac 애드혹, Windows 미서명 → SmartScreen 경고는 MVP 수용).
- **부모 사망 watchdog**: 셸이 비정상 종료해도 `NEULSANG_PARENT_PID` 재입양 감지로 desktopd가 스스로 종료(macOS).
- **기동 확인**(`src-tauri/src/startup.rs`): 두 번째 인스턴스는 포트 48989를 못 잡아 사이드카가 즉시 죽고, 그 창은 **먼저 뜬 인스턴스의** desktopd에 자기 토큰으로 요청해 전 화면 401이 된다. 그래서 spawn 직후 **`/v1/healthz`(인증 필요)**를 자기 토큰으로 찔러 200/401을 구분하고 401이면 안내 후 종료한다 — `/healthz`는 인증 면제라 남의 인스턴스도 200을 주므로 이 판별에 못 쓴다(프론트 `App.tsx`의 헬스체크가 딱 그 이유로 이 상황을 정상으로 오인한다).
- **오프라인 발음 추론**: CMUdict + 큐레이션 dev 용어 사전(#21 Phase3, #30). 캐시 → AI → 로컬 폴백 순서.
- **백업/동기화**: `GET /v1/export`·`POST /v1/import`(멱등·비파괴)·`POST /v1/backup`(#19), `sync_outbox` push는 `NEULSANG_SYNC_URL` 설정 시에만(#20). SYNC_URL은 https만 받는다(루프백 호스트만 http 예외) — outbox 이벤트에 캡처 원문과 출처 앱이 실려 나가기 때문. 인증 헤더는 아직 없다(중앙 서버 도입 시 설계).
- **modernc의 `VACUUM INTO`는 upstream SQLite와 다르다**: 대상 파일 가드가 "존재"가 아니라 **"크기 > 0"** 검사라 0바이트 파일은 덮어쓰고 dangling symlink는 따라간다(시스템 `sqlite3` CLI는 둘 다 거부하므로 **CLI로 확인하면 틀린 결론이 나온다** — 반드시 드라이버로 실측할 것). 그래서 `BackupFile`은 사용자 경로에 직접 쓰지 않고 옆에 임시파일을 만들어 `os.Rename`으로 교체한다. 이 구조가 "같은 파일명 재백업 실패"(VACUUM은 내용 있는 대상을 거부)도 같이 해결한다.

### 검증 게이트 (커밋 전 전부)
`go build ./... && go vet ./... && gofmt -l . && go test -race ./...`, `golangci-lint run ./...`(PATH에 없음 — `$(go env GOPATH)/bin`), `deadcode -test ./...`
`npm test && npx tsc --noEmit && npm run build`(반드시 `apps/desktop-ui`에서)
`cargo fmt --check && cargo clippy --all-targets -- -D warnings`(`apps/desktop-ui/src-tauri`)
GUI 수용 기준은 `npm run tauri dev` 또는 서명 `.app`에서 **사람이** 확인해야 한다 — 자동 게이트가 다 통과해도 화면이 안 도는 경우가 실제로 여러 번 있었다.
CI(`quality.yml`)가 같은 명령을 돌린다 — go·frontend는 ubuntu, **rust job은 `macos-latest`**(Linux 실기기가 없어 검증 범위에서 뺐다, ADR-0009). golangci-lint는 액션 v9 + `v2.12.2` 고정, govulncheck는 `v1.6.0` 고정 — 둘 다 재현성 때문에 latest를 안 쓴다.

### 다음 세션은 여기서 (2026-08-18 기준)

**재설계 v2는 main에 머지됐고(PR #3 → `6f0eea1`), `v0.2.0` 태그도 push했다.** 이제 **main이 곧 최신이다** — 이전 인수인계가 경고하던 "main을 읽으면 재설계 이전을 본다"는 더 이상 사실이 아니다. `redesign/schema-v2-triage` 브랜치는 역할이 끝났으니 지워도 된다.

**지금 가장 먼저 확인할 것: v0.2.0 릴리스가 끝났는지.**
태그는 `git ls-remote --tags origin | grep v0.2.0`으로, 결과는 https://github.com/dgkwon90/dictionary/releases 에서 본다. 확인 순서:
1. 빌드 3개(mac arm64 / mac x86_64 / Windows)가 다 성공했나. 하나라도 실패면 **릴리스가 draft에 갇혀 있다** — Actions 로그를 보고 고쳐서 태그를 다시 보내거나, GitHub에서 수동 Publish한다.
2. 공개됐으면 **사람이 받아서 실행 확인**. 재설계 v2의 첫 배포본이라 자동 게이트만으로는 부족하다(게이트가 다 통과해도 화면이 안 돌았던 전례가 여러 번 있다).
3. 특히 **v1 DB가 있는 상태의 첫 기동**을 실물로 볼 것 — 마이그레이션 0001이 재작성돼 checksum 불일치로 기동을 거부한다. 의도된 동작이지만, 그때 사용자가 보는 화면이 무엇인지는 아직 아무도 확인하지 않았다.

**그 다음** 우선순위 순:

1. **백로그 #33** — AI 타임아웃(호출당 20s×3)을 실측 분포 근거로 재조정. 근거 없는 상수라는 점에서 import 200MiB와 같은 성격이다. 실측하려면 실 Gemini 키로 호출 분포를 모아야 한다.
2. **문서 드리프트** — **PRD 곳곳에 v1 서술이 남아 있다.** 갱신한 절에는 `(v2 갱신)`/`(v2에서 삭제됨)` 표시를 달았으니, **표시가 없는 스키마·API·화면 서술은 신뢰하지 말고 코드를 확인할 것**(마이그레이션·router.go·src/). 제품 의도 서술(1~9·19~23장)은 유효하다. `docs/planning/remaining-work.md`·`docs/rw-11-platform-verification.md`는 v0.1.0 기준이라 완료 조건·GUI 체크리스트에 없어진 화면이 남아 있다.

**끝난 것**(2026-08-18): 미사용 의존성 제거, 보안 4항목(SYNC_URL https 강제 / 백업 임시파일+rename / export 행 상한 + 200MiB 근거 / 토큰 로그 필드 제거), 버전 0.2.0 범프, PR #3 머지, `v0.2.0` 태그 push. 보안 ④는 애초에 프로덕션에서 안 타는 경로였고(Tauri가 항상 토큰을 주입) 로그 유출도 없었다 — 위생 차원의 선제 정리다. 백업 화면은 `tauri dev`에서 사람이 확인했다(같은 파일명 재백업 성공, 백업 파일 쿼리 가능, 임시파일 잔여물 없음).

**로컬 환경 주의**: Go 툴체인이 `go1.26.5`라 `govulncheck`가 표준 라이브러리 5건을 잡는다(전부 go1.26.6에서 수정). CI는 `go-version: stable`이고 현재 stable이 1.26.6이라 **CI는 통과한다 — 코드 문제가 아니다.** 로컬을 1.26.6 이상으로 올리면 둘이 다시 맞는다.

**`gh` CLI가 이 머신에 없다** — PR 생성도, 릴리스·CI 상태 조회도 Claude가 못 한다. 사람이 브라우저로 보거나, 필요하면 `brew install gh`.

### 알려진 한계 (고칠 계획 없음, 의도된 것)
- **Windows/Linux는 알림 배너를 눌러도 화면이 안 열린다** — 플러그인에 클릭 콜백이 없다. macOS만 우회 구현.
- `notifications.rs`의 `fired` HashSet은 세션 동안 단조 증가하고, `ack_all`은 알림마다 개별 POST를 보낸다. 상주 앱이지만 알림 수가 적어 지금은 아프지 않다.
