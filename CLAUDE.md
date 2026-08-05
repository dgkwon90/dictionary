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
- 로컬 DB: **SQLite** (WAL 모드, FTS5)
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
- **패키징**: `.app`은 자기완결형(사이드카 `externalBin` 번들 + 애드혹 서명, #31). 태그 push → GitHub Actions가 mac arm64/x86_64 + Windows 빌드(#32). 번들 실행 시 cwd=`/`라 사용자 config `.env`를 읽는다(#25).
- **부모 사망 watchdog**: 셸이 비정상 종료해도 `NEULSANG_PARENT_PID` 재입양 감지로 desktopd가 스스로 종료(macOS).
- **오프라인 발음 추론**: CMUdict + 큐레이션 dev 용어 사전(#21 Phase3, #30). 캐시 → AI → 로컬 폴백 순서.
- **백업/동기화**: `GET /v1/export`·`POST /v1/import`(멱등·비파괴)·`POST /v1/backup`(#19), `sync_outbox` push는 `NEULSANG_SYNC_URL` 설정 시에만(#20).

### 검증 게이트 (커밋 전 전부)
`go build ./... && go vet ./... && gofmt -l . && go test -race ./...`, `golangci-lint run ./...`(PATH에 없음 — `$(go env GOPATH)/bin`), `deadcode -test ./...`
`npm test && npx tsc --noEmit && npm run build`(반드시 `apps/desktop-ui`에서)
`cargo fmt --check && cargo clippy --all-targets -- -D warnings`(`apps/desktop-ui/src-tauri`)
GUI 수용 기준은 `npm run tauri dev` 또는 서명 `.app`에서 **사람이** 확인해야 한다 — 자동 게이트가 다 통과해도 화면이 안 도는 경우가 실제로 여러 번 있었다.

### 남은 것
- 문서 드리프트: **PRD는 §6·§9.4·§10.4·§11·§12·§14.3·§15가 v1 서술**(스키마·API는 코드가 사실). 제품 의도 서술은 여전히 유효.
- 미확인: 서명 `.app`에서 OS 알림 클릭 → 검색 기록 이동(dev 비번들은 배너 미배달)
- 백로그 #33(AI 타임아웃 실측 기반 재검토)
