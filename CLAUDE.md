# Neulsang(늘상) — 로컬 우선 개발자 영어 학습 앱

## 프로젝트 개요
업무 중 마주친 영어 단어·용어·문장을 단축키로 즉시 검색·AI 해석하고, 그 기록을 복습 카드로 전환하는 로컬 우선(local-first) 데스크톱 앱.
**제품명은 Neulsang(한글 "늘상")으로 확정** (`docs/adr/ADR-0006-product-name.md`). 영문은 `Neulsang`, 한글 병기는 "늘상". 컴포넌트·바이너리명(`desktopd` 등)은 제품명과 별개다.

**기준 문서: `docs/prd.md`** (제품 정의, DB 스키마, API, 화면 설계, MVP 범위 — 이미 상세). 충돌 시 그 문서가 우선.
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

## 현재 상태 (2026-07-08)
설계 문서·ADR·백로그 준비 완료. 원격 저장소 `github.com/dgkwon90/dictionary`(public) 연결·push 완료.
착수 전 확정 필요 항목 모두 결정됨: 제품명=**Neulsang**(ADR-0006), AI provider=**Gemini**(ADR-0004), UI=**React**(ADR-0005).
백로그 #1~#5 완료: `apps/desktopd`(module `neulsang/desktopd`) — `GET /healthz`, `POST /v1/captures`(PRD §15.1), capture 생성 직후 `Explainer` 실행(비동기 goroutine, 90s 타임아웃) → `GET /v1/captures/{id}/explanation`(PRD §15.2), `GET /v1/inbox`(PRD §15.3, status/limit 필터)+`POST /v1/inbox/{id}/save|archive`. Inbox 상태는 captures.inbox_status(new/saved/archived만 저장) + lookup_jobs.status/review_cards 존재 여부로 review_added·failed를 조회 시 파생(ADR-0007). SQLite 자동 마이그레이션(PRD §11, 드라이버 modernc). 기본 주소 `127.0.0.1:48989`(`NEULSANG_ADDR`), DB 경로 `NEULSANG_DB_PATH`. 계층: domain(interface: capture/explain/inbox) ← db/sqlite·infra/llm/gemini(구현), transport/http/handlers.
백로그 #6 완료: 실제 AI provider(**Gemini**) 연동 — `internal/infra/llm/gemini` REST 클라이언트(`x-goog-api-key`, `responseSchema` 구조화 출력, `parseResponse` 검증), 재시도(20s/attempt, 최대 2회, 지수 백오프, 429/5xx/네트워크만), `raw_response_json` 보존. `Explainer.Explain`은 `(ExplainResult, rawResponseJSON, error)` 반환. **범용성**: `NEULSANG_AI_PROVIDER`(gemini/mock)로 선택, 미지정 시 API key 유무로 자동. **API key는 `NEULSANG_GEMINI_API_KEY` 환경변수로만 읽고 DB·파일·로그에 저장 안 함**(암호화 DB/OS 키체인은 #17 재검토, ADR-0004 부록). 기본 모델은 무료 티어가 가장 넉넉한 `gemini-flash-lite-latest`(`internal/infra/llm/gemini` DefaultModel), `NEULSANG_GEMINI_MODEL`로 override. 로컬 편의를 위해 `internal/config/dotenv.go`가 cwd에서 상위로 올라가며 `.env`를 찾아 로드(실제 환경변수 우선, `main`에서만 호출해 테스트 오염 방지, `.env`는 gitignore됨). 실 키로 전체 루프 E2E 검증 완료(검색→실제 한국어 해석→sub_items 추출→candidate→mark-unknown→review card→due 조회). 실 검증으로 드러난 AI 계약 견고화(`internal/infra/llm/gemini`): (1) flash-lite가 difficulty/importance를 0~1 밖으로 반환 → `parseResponse`에서 `clamp01` 정규화(안 하면 Validate 실패로 해석 폐기), (2) sub_items의 item_type 빈 값·candidate 미생성 → responseSchema에서 sub_items·review_card_candidates를 top-level `required`+`minItems:1`, item_type/card_type `enum`, sub_item 필수필드 `required` 강제 + 프롬프트 명시, (3) 빈 detected_language(soft 메타) → `und` 기본값(brief/detailed 같은 핵심 내용은 strict 유지). codex 리뷰 반영. mock은 항상 완벽한 sub_items라 이 문제들이 안 보였음 → 실 provider 검증의 가치.
백로그 #7 완료(2026-07-08): Knowledge/Learner item 추출(PRD Task05) — `internal/db/sqlite/explain_repo.go`의 `SaveSuccess` 트랜잭션 안에서 `extractKnowledge`/`upsertKnowledgeItem` 실행(explanation 저장과 원자적 커밋). AI `sub_items`를 `knowledge_items`로 `(normalized_key,item_type)` 병합(SELECT-then-INSERT/UPDATE, `first_seen_at` 보존·`last_seen_at` 갱신, 빈 key skip), `capture_items` 연결(role=`sub_item`, confidence=`importance`), `learner_items` `ON CONFLICT` upsert(`ask_count`+1, `last_asked_at`). codex 교차검토 반영: 결과 단위 sub_item dedup(이중 계상 방지), 재검색 빈 값은 `COALESCE`로 기존 pronunciation/meaning 보존, TOCTOU는 `SetMaxOpenConns(1)`로 안전. agy는 provider 쿼터 소진으로 미실행. 추출은 infra/sqlite에 국한(domain은 infra 무의존 유지).
백로그 #8 완료(2026-07-08): Mark-unknown API + review card candidate(PRD §15.4/§12.1/§14.3). **Part A**: 마이그레이션 `0002_review_card_candidates.sql`로 `review_card_candidates` 테이블 추가, `SaveSuccess` 트랜잭션 안에서 candidate를 capture+**primary knowledge_item**(importance 최고, sub_item 0개면 NULL)에 연결해 원자 저장(이전엔 `raw_response_json`에만 존재). **Part B**: `internal/domain/knowledge` 도메인 신설 — `POST /v1/knowledge/{id}/mark-unknown`(`learner_items` wrong_count+1·last_wrong_at·status=active)·`mark-known`(status=known), 없으면 404, 응답에 candidate_count 포함. `knowledge_repo.go`/handler/router(5번째 파라미터) 배선. codex 정확성 리뷰 통과, agy는 쿼터 소진으로 미실행(Claude 직접 아키텍처 검토 SOUND). **알려진 편차**: §14.3은 knowledge item upsert도 knowledge 도메인 소관이나, explanation과 원자적이어야 해 현재 sqlite explain repo(#7)에 위치 — 도메인 성장 시 분리 재검토. **DB 스키마: 마이그레이션 2개(0001 init, 0002 candidates)**.
백로그 #9 완료(2026-07-08): Review card 생성(PRD Task06). mark-unknown(#8)이 이제 해당 knowledge item의 unconsumed `review_card_candidates`를 `review_cards`로 **같은 트랜잭션에서 원자적으로** 생성(state=`new`, due_at=`now`, card_type 없으면 `meaning`), 응답에 `cards_created`. 멱등성: 마이그레이션 `0003`로 `review_card_candidates.consumed_at` 추가 — unconsumed만 소비, 재호출 시 카드 0개(wrong_count은 증가), 나중 캡처가 추가한 candidate는 다음 mark-unknown에서 카드화. `internal/domain/review` 도메인 신설(Card/Repository.DueCards/Service.Due limit 50·max 200) + `internal/db/sqlite/review_repo.go` + `GET /v1/reviews/due?limit=N`(due_at<=now soonest-first, answer 미노출; grading은 #10) + router 6번째 파라미터. codex 정확성 리뷰 통과, agy는 쿼터 소진으로 skip. **알려진 편차**: §14.3은 카드 생성이 review 도메인 소관이나 learner 변경과 원자적이어야 해 생성 SQL은 knowledge mark-unknown tx(sqlite)에 위치. **DB 스키마: 마이그레이션 3개(0001 init, 0002 candidates, 0003 consumed_at)**.
백로그 #10 완료(2026-07-08): Review session + FSRS-lite(PRD Task07/§13.1/§15.5·15.6). FSRS-lite 코어는 `internal/domain/review/schedule.go` 순수함수 `NextSchedule`(첫복습 초기간격 Again10분/Hard1일/Good3일/Easy7일, 이후 ×1.2/×2.5/×4.0, **Again은 reps=0 리셋=재학습**). `POST /v1/reviews/{id}/grade`→sqlite `Grade` 트랜잭션(카드 읽기→NextSchedule→review_cards 갱신+`review_logs` append-only+`learner_items` review_count upsert, `stability`=현재 간격일수, 없는 카드 404). `POST /v1/reviews/session/start`=due 목록. mastery/weakness는 #11. codex 리뷰 통과, agy skip. **다음 작업: #11 mastery/weakness score 계산(§13.2~13.3, learner_items 갱신) — depends #10(완료).**
