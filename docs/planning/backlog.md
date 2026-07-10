# Backlog

> GitHub 연결 전의 이슈 대장. 형식: `../rules/github-workflow.md`.
> 상태: `[ ]` 대기 / `[~]` 진행 중 / `[x]` 완료
> 각 이슈는 PRD(`../prd.md`)의 관련 절을 인용한다 — 상세 스펙(DB 스키마, API 요청/응답 예시)은 PRD를 그대로 따르고 여기서는 반복하지 않는다.

## 착수 전 확정 필요 (블로킹) — 모두 확정됨 (2026-07-04)
- [x] `ADR-0004`: AI provider 1차 연동 대상 → **Gemini** 확정
- [x] `ADR-0005`: Tauri UI 프레임워크 → **React** 확정
- [x] 제품명 → **Neulsang** 확정 (`ADR-0006`). go module path·package명은 #1에서 `neulsang` 기준으로 정한다

## PRD 대비 발견한 갭
PRD MVP 필수 기능(§5.2)을 Task 01~12(§18.2)·Phase 1~5(§22)·DB 스키마(§11)와 대조한 결과:

1. **한글 발음 입력 → 유사 영어 후보 추론**(§5.2-3, 예: "스테일"→stale)이 어느 Task에도 명시적으로 없다. → #21로 이슈화. 착수 전 알고리즘 방향(음차 사전 매칭 vs AI 호출)을 정해야 한다.
2. **Inbox 상태 컬럼 부재**: §10.4는 Inbox 탭 5종(New/Saved/Review Added/Archived/Failed)을, §15.3은 `GET /v1/inbox?status=new`를 정의하지만, §11 스키마 어디에도 이 상태를 저장할 컬럼이 없다 (`lookup_jobs.status`는 queued/running/done/failed로 AI 작업 상태일 뿐, Saved/Archived 같은 사용자 분류를 담지 못함). → #2에서 스키마 확장으로 해소 (수용 기준에 반영).
3. **sidecar → UI 알림 채널 미정의**: §7.1은 알림 표시를 Tauri 담당으로, §8.1은 Notification을 Go sidecar 아래에 그린다. §15의 로컬 API는 UI→sidecar 단방향 요청뿐이라, "검색 결과 준비 완료"·"복습 시간" 같은 sidecar발 이벤트를 UI가 알 방법(폴링/SSE/OS 알림 직접 발송)이 정의되지 않았다. → #18 착수 시 설계 결정 (수용 기준에 반영).

## 의존 그래프 (착수 순서)

```
[Backend 트랙]
#1 desktopd bootstrap ──→ #2 SQLite schema ──→ #3 Capture API ──→ #4 AI explain(mock) ──→ #5 Inbox API
                                                                        │
                                                                        ├─→ #6 실제 AI provider (Gemini, ADR-0004 — 다른 이슈를 막지 않는 병렬 트랙)
                                                                        │
                                                                        └─→ #7 Knowledge/Learner (mock의 sub_items로 진행 가능) ──→ #8 mark-unknown+card candidate
                                                                                                                                        │
                                                                      #9 Review card 생성 ←────────────────────────────────────────────┘
                                                                            │
                                                                            ▼
                                                          #10 Review session + FSRS-lite ──→ #11 mastery/weakness score ──→ #12 Dashboard API

[Frontend 트랙 — #5 이후 아무 때나 착수 가능, 각 화면은 해당 API 이슈 완료 후]
#13 Tauri 셸 부트스트랩 ──┬─→ #14 글로벌 단축키 + Quick Search (needs #4)
                          ├─→ #15 Inbox 화면 (needs #5, #8)
                          ├─→ #16 Review 화면 (needs #10)
                          └─→ #17 Dashboard + Settings (needs #12)
#14, #16 ──→ #18 알림·리마인더

[마무리]
#17 ──→ #19 Export/Import ──→ #20 sync_outbox + push client skeleton

[독립]
#21 한글 발음 유사 영어 후보 추론 (needs #4, 알고리즘 방향 확정 필요)
```

- **막히면 이슈를 더 쪼갠다** — 의존 위반 착수 금지
- Backend 트랙(#1~#12)을 curl/httpie로 먼저 완결시키고, Frontend 트랙(#13~)을 뒤에 붙이는 순서를 권장 (UI 없이도 API 단위 검증이 가능해 디버깅이 쉬움) — 단, 이는 순서 제안이지 강제 아님

---

## Milestone: v0.1 — Core capture loop (backend, API 단위 검증)

### [x] #1 desktopd bootstrap — 완료 (2026-07-07)
`area:desktopd` `kind:feat`
- 목적: Go sidecar 프로세스의 골격 확보 (PRD Task01)
- 수용 기준:
  - 루트 `go.work` + `apps/desktopd/go.mod` 생성 (PRD §14.1 monorepo 구조)
  - `go run ./cmd/desktopd`로 실행, `GET /healthz` → 200 OK
  - 설정 로딩(config), 구조화 로깅(logger) 구성
  - SQLite 연결 구조만 준비 (migration은 #2에서)
  - `internal/{domain,infra,transport}` 경계 골격 생성 (빈 패키지라도 구조는 잡는다)
- 범위 밖: AI 기능, 실제 migration

### [x] #2 SQLite schema & migration — 완료 (2026-07-07, ADR-0007)
`area:desktopd` `kind:feat` — depends #1
- 목적: PRD §11의 로컬 DB 스키마 전체를 migration으로 적용
- 수용 기준:
  - 앱 시작 시 DB 파일 자동 생성, WAL 모드 설정
  - migration 도구로 §11.1~11.11 전 테이블 생성 (`sync_outbox`, `reminders`까지 포함 — 이후 이슈에서 채우더라도 스키마는 한 번에)
  - **PRD 갭 2 해소**: Inbox 상태(New/Saved/Review Added/Archived — Failed는 `lookup_jobs.status`에서 도출)를 담을 컬럼을 스키마에 추가하고 이 문서·glossary에 기록
  - `make migrate` 또는 동등 명령으로 재실행 시 idempotent 확인
- 범위 밖: FTS5 검색 인덱스 활용(실제 검색 기능은 후속 이슈)

### [x] #3 Capture API — 완료 (2026-07-07)
`area:desktopd` `kind:feat` — depends #2
- 목적: `POST /v1/captures` 구현 (PRD Task03, §15.1)
- 수용 기준:
  - capture 저장, `text_hash`로 중복 감지 가능
  - `lookup_jobs` status=queued 생성
  - `sync_outbox`에 `capture_created` 이벤트 기록 (전송은 아직 안 함)
- 범위 밖: 실제 AI 호출

### [x] #4 AI Explain pipeline (mock provider) — 완료 (2026-07-07)
`area:desktopd` `kind:feat` — depends #3
- 목적: `Explainer` 인터페이스 정의 + mock 구현으로 파이프라인 완결 (PRD Task04, §7.5, §12.1)
- 수용 기준:
  - mock AI 결과로 `explanations` 저장 가능
  - `ExplainResult` JSON schema validation (실패 시 `lookup_jobs.status=failed` + `error_message`)
  - `GET /v1/captures/{id}/explanation` (§15.2) 동작
- 범위 밖: 실제 provider 연동(#6)

### [x] #5 Inbox API — 완료 (2026-07-08)
`area:desktopd` `kind:feat` — depends #4
- 목적: `GET /v1/inbox?status=new&limit=50` (§15.3)
- 수용 기준: New/Saved/Review Added/Archived/Failed 상태별 조회(#2에서 추가한 상태 컬럼 + `lookup_jobs.status` 조합), 상태 변경 API(저장/보관 등), 최신순 정렬
- 범위 밖: UI (프론트엔드 트랙 #15)

---

## Milestone: v0.2 — 실제 AI + 지식 모델

### [x] #6 실제 AI Provider 연동
`area:desktopd` `kind:feat` — depends #4 (ADR-0004: Gemini 확정)
- 목적: mock 대신 실제 provider(**Gemini**) 연동. `Explainer` 인터페이스 뒤 구현체 교체이므로 **#7~#12를 막지 않는다** — mock으로 전체 파이프라인을 먼저 완성해도 된다
- 수용 기준:
  - Gemini 구조화 출력(response schema)이 `ExplainResult`를 강제하는지 검증, 부족하면 구현체 내부에 파싱·검증 레이어 추가
  - API key는 평문 저장 금지 (저장 방식은 설계 시 결정)
  - 재시도/타임아웃 정책 명시, 실패 시 mock과 동일하게 `failed` 처리
  - `raw_response_json` 보존 (PRD §18.1)
- 범위 밖: provider 여러 개 동시 지원 UI(설정에서 선택은 #17)
- 결과(2026-07-08 완료):
  - `internal/infra/llm/gemini` REST 클라이언트 — `x-goog-api-key` 헤더, `responseSchema`로 `ExplainResult` 구조화 출력 강제 + `parseResponse` 검증 레이어
  - 재시도/타임아웃: attempt당 20s, 최대 2회 재시도(총 3회), 지수 백오프, 429/5xx/네트워크 오류만 재시도. 파싱·검증 실패 시 `Explainer` 에러 → 상위에서 `failed` 처리
  - `Explainer.Explain`이 `(ExplainResult, rawResponseJSON, error)` 반환 → `raw_response_json` 보존
  - **API key 저장: 안 함.** `NEULSANG_GEMINI_API_KEY` 환경변수로만 읽고 DB·파일·로그 어디에도 쓰지 않음 → "평문 저장 금지"를 저장 자체를 안 함으로 충족. 암호화 DB 저장/OS 키체인은 #17 Settings UI 시점 재검토 (ADR-0004 부록)
  - **범용성**: `NEULSANG_AI_PROVIDER`(gemini/mock)로 provider 선택, 미지정 시 API key 유무로 자동 선택. OpenAI/Claude 추가 = `internal/infra/llm/<name>` 패키지 + `newExplainer()` case 1개

### [x] #7 Knowledge/Learner item 추출 — 완료 (2026-07-08)
`area:desktopd` `kind:feat` — depends #4 (mock의 `sub_items`로 개발·테스트 가능, 실제 provider #6는 선행 조건 아님)
- 목적: AI `sub_items`를 `knowledge_items`로 upsert, `learner_items` 갱신 (PRD Task05)
- 수용 기준:
  - 같은 단어는 `normalized_key`+`item_type` 기준 하나의 knowledge_item으로 병합
  - `ask_count` 증가, `first_seen_at`/`last_seen_at` 갱신
  - `capture_items` 연결(role, confidence)
- 결과(2026-07-08 완료):
  - `internal/db/sqlite/explain_repo.go` — `SaveSuccess` 트랜잭션 안에서 `extractKnowledge`/`upsertKnowledgeItem` 실행 → explanation 저장과 원자적으로 커밋. explanation insert가 먼저라 중복 등 실패 시 전체 롤백(기존 `TestExplainRepositorySaveSuccessDuplicateRollsBack`로 확인)
  - 병합: `(normalized_key, item_type)` SELECT-then-INSERT/UPDATE. `first_seen_at` 보존, `last_seen_at`·surface/pronunciation/meaning 갱신. 빈 `normalized_key`/`item_type` sub_item은 skip
  - `capture_items`: role=`"sub_item"`(캡처 주 용어와 구분, #8+에서 다른 role 사용 예정), confidence=`sub_item.importance`
  - `learner_items`: `ON CONFLICT(knowledge_item_id)` upsert로 `ask_count`+1, `last_asked_at` 갱신 (신규는 1)
  - **codex 교차검토 반영**: (1) 한 lookup 내 동일 `(normalized_key,item_type)` sub_item 중복 시 ask_count·capture_items 이중 계상 → 결과 단위 dedup. (2) 재검색이 빈 pronunciation/meaning을 주면 기존 값 삭제되던 문제 → `COALESCE(NULLIF(?,''), col)`로 보존. (3) select-then-insert TOCTOU는 `SetMaxOpenConns(1)`로 안전(주석 명시). agy 리뷰는 provider 쿼터 소진(429, ~34h)으로 미실행 → 아키텍처 검토는 Claude가 직접 수행(경계 위반 없음: 추출은 infra/sqlite에 국한, domain은 infra 무의존 유지)
  - E2E 스모크(mock provider, 실 바이너리): capture POST → knowledge_items/capture_items/learner_items 실제 기록 확인

### [x] #8 Mark-unknown API + review card candidate — 완료 (2026-07-08)
`area:desktopd` `kind:feat` — depends #7
- 목적: `POST /v1/knowledge/{item_id}/mark-unknown` (§15.4), AI가 생성한 `review_card_candidates`를 다음 이슈(#9)가 소비할 수 있는 형태로 보존
- 수용 기준:
  - mark-unknown 호출 시 `learner_items` 상태 갱신(`wrong_count`/`last_wrong_at` 등) 확인 테스트
  - 해당 knowledge item의 card candidate(§12.1 `review_card_candidates`)가 조회 가능한 형태로 저장·연결됨
  - "알고 있음" 표시 API도 함께 (§5.2-6 Inbox 동작 세트의 반대 방향)
- 결과(2026-07-08 완료):
  - **Part A — candidate 보존**: 마이그레이션 `0002_review_card_candidates.sql`로 `review_card_candidates` 테이블 추가(capture_id, knowledge_item_id nullable, card_type/question/answer/explanation, 두 컬럼 인덱스). `SaveSuccess` 트랜잭션 안에서 explanation·knowledge 추출과 원자적으로 저장 — candidate는 capture와 **primary knowledge_item**(추출된 sub_item 중 importance 최고, 동점 시 first)에 연결. sub_item 0개면 knowledge_item_id NULL(capture로만 조회). question/answer 빈 candidate는 skip. 이전엔 `raw_response_json`에만 있던 것을 조회 가능한 형태로 승격
  - **Part B — mark-unknown/known**: PRD §14.3 `knowledge` 도메인 신설(`internal/domain/knowledge`: service+Repository interface). `POST /v1/knowledge/{id}/mark-unknown`→`learner_items` upsert(`wrong_count`+1, `last_wrong_at`, status=`active`), `POST /v1/knowledge/{id}/mark-known`→status=`known`(반대 방향). 존재 확인 후 없으면 404. 응답에 status·ask_count·wrong_count·candidate_count(연결 증명) 포함. `internal/db/sqlite/knowledge_repo.go`(트랜잭션+read-back), 핸들러, router 5번째 파라미터로 배선
  - **교차검토**: codex 정확성 리뷰 통과(블로커·should-fix 0, SQL/플레이스홀더/트랜잭션/엣지케이스 전수 확인). agy는 provider 쿼터 소진(429)으로 미실행 → 아키텍처는 Claude 직접 검토: SOUND. **알려진 편차(문서화)**: §14.3은 knowledge 도메인이 knowledge item upsert도 담당한다고 하나, 현재 upsert는 explanation 저장과 원자적이어야 해 sqlite explain repo에 위치(#7). 도메인이 커지면 분리 재검토. primary-anchor는 MVP 허용(캡처당 보통 1개 용어)
  - E2E 스모크(mock, 실 바이너리): capture→candidate 저장·연결, mark-unknown(wrong_count=1/active/candidate_count=1)→mark-known(known)→없는 id 404 확인

---

## Milestone: v0.3 — 복습 엔진

### [x] #9 Review card 생성 — 완료 (2026-07-08)
`area:desktopd` `kind:feat` — depends #8
- 목적: PRD Task06 — candidate 기반 카드 생성, `due_at` 설정
- 수용 기준: "모름" 처리한 단어에 대해 `review_cards` row 생성, due card 조회 가능
- 결과(2026-07-08 완료):
  - **카드 생성**: `POST /v1/knowledge/{id}/mark-unknown`(#8)이 이제 해당 knowledge item의 unconsumed `review_card_candidates`를 `review_cards`로 생성 — **mark-unknown 트랜잭션 안에서** 원자적으로(learner 상태 갱신과 함께). 새 카드는 `state="new"`, `due_at=now`(즉시 due). `card_type` 없으면 `meaning` 기본. 응답에 `cards_created` 추가
  - **멱등성**: 마이그레이션 `0003`으로 `review_card_candidates.consumed_at` 추가. 생성 시 unconsumed만 소비→consumed 표시. 재-mark-unknown은 카드 0개 생성(중복 방지)하되 `wrong_count`는 계속 증가. **나중 캡처가 추가한 새 candidate는 다음 mark-unknown에서 카드화**(테스트로 검증)
  - **due 조회**: `internal/domain/review` 도메인 신설(Card, Repository.DueCards, Service.Due — limit 기본 50/최대 200/음수 400). `internal/db/sqlite/review_repo.go`(`DueCards` + candidate→card 생성 헬퍼), `GET /v1/reviews/due?limit=N`(`due_at<=now` soonest-first, answer 미노출 — grading은 #10). router 6번째 파라미터로 배선
  - **교차검토**: codex 정확성 리뷰 통과(블로커·should-fix 0; row drain/placeholder/멱등성/트랜잭션/타임존 전수 확인). codex 제안대로 "나중 candidate 소비" 테스트 추가. agy는 쿼터 소진(~2026-07-10 리셋)으로 skip
  - **알려진 편차**: §14.3은 카드 생성을 review 도메인 소관으로 두나, learner 상태 변경과 원자적이어야 해 생성 SQL은 sqlite(knowledge mark-unknown tx)에 위치. review 도메인은 Card 모델·due 조회를 소유(#7/#8과 동일한 원자성 우선 절충)
  - E2E 스모크(mock, 실 바이너리): due 빈 목록→mark-unknown(cards_created=1)→재호출(cards_created=0/wrong_count=2)→due 1건(new, answer 미노출) 확인
  - **DB 스키마: 마이그레이션 3개(0001 init, 0002 candidates, 0003 consumed_at)**

### [x] #10 Review session + FSRS-lite 스케줄링 — 완료 (2026-07-08)
`area:desktopd` `kind:feat` — depends #9
- 목적: PRD Task07, §13.1 — `POST /v1/reviews/session/start`(§15.5), `POST /v1/reviews/{card_id}/grade`(§15.6)
- 수용 기준:
  - Again/Hard/Good/Easy 처리, §13.1 간격 규칙(초기 10분/1일/3일/7일, 이후 배율 적용) 구현
  - `review_logs` append-only 저장
- 결과(2026-07-08 완료):
  - **FSRS-lite 코어**: `internal/domain/review/schedule.go`의 순수 함수 `NextSchedule(reps, prevIntervalDays, rating, now)` — 첫 복습(reps==0) 초기 간격(Again 10분/Hard 1일/Good 3일/Easy 7일), 이후 Hard×1.2·Good×2.5·Easy×4.0. **Again은 lapse로 reps=0 리셋(재학습)** → 다음 채점은 초기 간격 사용(§13.1 "간격 초기화" 해석, 코드 주석에 명시). 단위 테스트로 검증
  - **채점 API**: `POST /v1/reviews/{id}/grade`(rating+elapsed_ms) → sqlite `Grade`가 트랜잭션에서 카드 읽기→NextSchedule→review_cards 갱신(state/due_at/stability=간격일수/reps/lapses/last_review_at)+`review_logs` append-only 삽입+`learner_items` review_count upsert. 없는 카드 404. `stability` 컬럼이 현재 간격(일)을 보관
  - **세션**: `POST /v1/reviews/session/start`(§15.5) = 현재 due 목록 반환(MVP, 세션 상태 없음). GET /v1/reviews/due(#9)와 공존
  - **범위 밖(#11)**: mastery/weakness score 계산은 #11 소관 — review_logs가 append-only라 사후 파생 가능
  - codex 정확성 리뷰 통과(블로커 0; 스케줄 수학·트랜잭션·라우팅 정합 확인, Again 해석 defensible), agy는 쿼터 소진으로 skip
  - E2E 스모크(mock): mark-unknown→session/start(due 1)→grade good(state=review/reps=1/due+3d, due 목록서 제거)→review_logs 기록→grade again(state=learning/reps=0 lapse) 확인

### [x] #11 mastery/weakness score 계산 — 완료 (2026-07-08)
`area:desktopd` `kind:feat` — depends #10
- 목적: PRD §13.2~13.3 계산식 구현, `learner_items` 갱신
- 수용 기준: review 완료마다 재계산, 0.0~1.0 clamp 확인 테스트
- 결과(2026-07-08 완료):
  - **계산식**: `internal/domain/review/scoring.go` 순수 함수 — `MasteryScore(GradeCounts)`(§13.2, clamp 0..1), `WeaknessScore(ask,wrong,mastery,recentRepeatBonus)`(§13.3, sort key용 0 floor). 단위 테스트로 검증
  - **mastery 저장**: `Grade` 트랜잭션에서 review_log 삽입 직후 `gradeCountsForKnowledgeItem`(해당 knowledge_item의 모든 카드 review_logs를 rating별 집계, `source='review'` 필터)으로 전체 이력 재계산 → `learner_items.mastery_score` upsert. grade 응답·GradeResult에 `mastery_score` 포함
  - **weakness 미저장**: `learner_items`에 weakness_score 컬럼이 없어 파생값으로 취급(정렬·대시보드 #12에서 사용). recent_repeat_bonus는 호출자 제공(MVP 0)
  - codex 리뷰 통과(CLEAN); 제안한 `source='review'` 필터·다중 카드 무중복 테스트 반영. agy는 스모크로 재확인했으나 여전히 쿼터 소진(~07-10 리셋)으로 skip
  - E2E(mock): grade good→mastery 0.2, grade easy→0.5, learner_items 저장 확인
  - **참고**: §14.3의 `stats` 도메인(대시보드 집계)은 #12에서 신설 예정 — weakness 소비처

### [x] #12 Dashboard summary API — 완료 (2026-07-08)
`area:desktopd` `kind:feat` — depends #11
- 목적: PRD Task08, §15.7
- 수용 기준: `GET /v1/dashboard/summary`로 오늘/주간 검색 수, 많이 검색·많이 틀린 단어, 카테고리별 약점, due card 수 반환
- 결과(2026-07-08 완료):
  - **stats 도메인 신설**(§14.3): `internal/domain/stats`(Summary/Window/Repository) + `internal/db/sqlite/stats_repo.go`(7개 읽기 쿼리) + `internal/transport/http/handlers/dashboard.go` + `GET /v1/dashboard/summary`(router 7번째 파라미터)
  - **응답**: today/week 검색 수(captures created_at, UTC 오늘/rolling 7일), 오늘 완료 복습 수(review_logs), due card 수(review_cards due_at<=now), most_searched·most_wrong(learner_items ask_count·wrong_count TOP 10), category_weakness
  - **카테고리별 약점**: knowledge_items.domain_category별 집계(ask/wrong/mastery 평균)에 `review.WeaknessScore` 적용 = **평균 아이템 약점**(공식 단일 소스, stats→review 도메인 import). codex 지적 반영: 합계 대신 **평균**으로 계산해 아이템 수 편향·다수 숙달 아이템의 마스킹 제거. `WeaknessScore`를 float 파라미터로 변경
  - codex 리뷰(no blockers) 반영, agy는 스모크 재확인했으나 여전히 쿼터 소진(~07-10)으로 skip
  - E2E(mock): 3회 검색+mark-unknown+grade → today/week=3, 완료복습=1, due=0, most_searched 3건, most_wrong=stale, general 약점 계산 확인
  - **v0.3 백엔드 마일스톤(#1~#12) 완료**

---

## Milestone: v0.4 — Tauri 데스크톱 UI

### [x] #13 Tauri 셸 부트스트랩 (2026-07-10 완료)
`area:desktop-ui` `kind:feat` — depends #5, ADR-0005(React) 확정
- 목적: 트레이 아이콘, 기본 윈도우, desktopd 프로세스 생명주기 관리(자식 프로세스로 실행/종료), 로컬 API 클라이언트 골격 (PRD Task09 일부)
- 수용 기준: 트레이 메뉴(Quick Search/Inbox/Today Review/Dashboard/Settings/Quit, §10.1)에서 각 항목 클릭 시 해당 창(빈 화면이라도)이 뜬다
- 구현: `apps/desktop-ui` Tauri 2 + React 19 + Vite 스캐폴드. `src-tauri/src/tray.rs`(트레이 메뉴→메인 윈도우 show/focus + `navigate` 이벤트, Quit=`app.exit`), `sidecar.rs`(desktopd 자식 프로세스 spawn/kill, 바이너리 탐색 env→실행파일옆→dev경로, 없으면 UI 단독 기동), `lib.rs`(RunEvent::Exit에서 사이드카 정리). 프론트: `src/App.tsx`(navigate 리스너로 화면 전환 + 사이드카 헬스 폴링), `src/api/client.ts`(DesktopdClient, `@tauri-apps/plugin-http` fetch로 CORS 우회). codex 리뷰 반영: (1) 트레이가 id(snake)가 아니라 라벨을 emit하도록 수정(안 하면 네비게이션 전부 무동작=수용기준 실패), (2) webview→127.0.0.1 fetch는 CORS/mixed-content로 막혀 plugin-http로 전환(desktopd에 CORS 안 엶), (3) capability 슬림화(Rust 직접 호출은 capability 무관), 트레이 아이콘 non-panic. 검증: `cargo check`/`clippy -D warnings`/`fmt`, `npm run build` 통과.
- **부모-사망 watchdog 추가(2026-07-10)**: 셸 비정상 종료(SIGKILL/패닉) 시 desktopd 고아 방지. `internal/watchdog` — 셸이 `NEULSANG_PARENT_PID`로 게이트를 켜고(사이드카 spawn 시 `.env` 설정), desktopd가 시작 시 `os.Getppid()`를 기록해 2초 폴링으로 재입양(부모 PID 변화)을 감지하면 종료 컨텍스트를 취소(SIGINT와 동일한 graceful shutdown). PID 재사용에 안전(생존 probe 아닌 재입양 감지). codex 리뷰 no blockers. **남은 한계**: (a-win) Windows는 고아 재입양이 없어 이 방식 미동작 → Job Object로 별도 처리(macOS 우선이라 후속). (b) 창 닫기=앱 종료(현재). 트레이 상주 앱이면 hide-to-tray가 자연스러우나 의식적 결정 필요.

### [x] #14 글로벌 단축키 + Quick Search 화면 (2026-07-10 완료)
`area:desktop-ui` `kind:feat` — depends #13, #4
- 목적: PRD §5.2-2, §10.2, §9.1~9.3 흐름
- 수용 기준:
  - 단축키(macOS `Cmd+Shift+E`, Win/Linux `Ctrl+Shift+E`)로 어디서든 팝업 호출
  - 클립보드 자동 삽입 + 직접 입력 두 경로 모두 동작, `POST /v1/captures` 연동
- 구현: Quick Search를 **프레임리스 팝업 윈도우**(`quicksearch`, tauri.conf.json에 visible:false·decorations:false·alwaysOnTop·skipTaskbar·center 선언)로. `tauri-plugin-global-shortcut`으로 `CommandOrControl+Shift+E` 등록(Rust `lib.rs`, 핸들러→`popup::show`), 트레이 "Quick Search"도 같은 팝업 호출(`popup.rs`). 팝업은 열릴 때 `quicksearch:activate` 이벤트로 클립보드 자동삽입·입력 초기화·포커스. 프론트: `main.tsx`가 `getCurrentWindow().label`로 main/quicksearch 렌더 분기, `quicksearch/QuickSearch.tsx`가 입력→`POST /v1/captures`(input_mode=clipboard/manual 판별)→`GET .../explanation` 폴링(700ms, 90s 타임아웃, 세대번호로 재활성화 취소)→brief/pron/detailed/examples/sub_items 표시, Esc로 숨김. `api/client.ts`에 createCapture/getExplanation + 타입 추가. 클립보드는 `tauri-plugin-clipboard-manager`. capability: `clipboard-manager:allow-read-text`·`core:window:allow-hide`, 창 스코프 `["main","quicksearch"]`(글로벌 단축키는 Rust 전용이라 capability 불요). codex 리뷰 반영: (1) 단축키 등록 실패가 앱 기동을 막지 않도록 `?`→경고 로그(OS 점유 충돌 대비), (2) 폴링 await 후 세대 재확인해 stale 결과가 재활성화 입력을 덮어쓰지 않게, (3) Cargo.toml에서 log/serde가 desktop-only `[target]`에 딸려간 것 top-level로 복원. 검증: `cargo check`/`clippy -D warnings`/`fmt`, `npm run build` 통과(GUI 수용기준=단축키·클립보드·검색은 `tauri dev`로 수동 확인).
- **알려진 편차**: 결과 상세는 팝업 안에 간략 표시(브리프/발음/상세/예문/sub_items). Result Detail 전체 화면(§10.3)·Inbox 연동은 #15에서.

### [ ] #15 Inbox 화면
`area:desktop-ui` `kind:feat` — depends #13, #5, #8
- 목적: PRD §10.4, §9.4
- 수용 기준: New/Saved/Review Added/Archived/Failed 탭, "모름" 클릭 시 review card 생성 흐름까지 연결

### [ ] #16 Review 화면
`area:desktop-ui` `kind:feat` — depends #13, #10
- 목적: PRD §10.5, §9.5
- 수용 기준: 카드 표시 → 답변 → Again/Hard/Good/Easy 선택 → 다음 카드로 이동

### [ ] #17 Dashboard + Settings 화면
`area:desktop-ui` `kind:feat` — depends #13, #12
- 목적: PRD §10.6, §10.7
- 수용 기준: Dashboard 지표 표시, Settings에서 단축키·AI provider·API key·알림 시간·DB 경로 설정 가능

### [ ] #18 알림·리마인더
`area:desktop-ui` `kind:feat` — depends #14, #16
- 목적: PRD Task10, §9.6, §23.2(알림 과다 방지)
- 수용 기준:
  - **PRD 갭 3 해소**: sidecar발 이벤트(결과 준비·복습 시간)를 UI가 아는 방식(폴링/SSE/OS 알림 직접 발송) 설계 결정을 이 이슈 착수 시 먼저 문서화 (되돌리기 어려우면 ADR로)
  - 검색 결과 준비, due card 알림이 각각 **하나로 묶여서** 표시 (문장 하나에서 단어 여러 개 추출돼도 알림 1개)
  - 아침/저녁 설정 시각에 알림, 클릭 시 Review 화면으로 이동

---

## Milestone: v0.5 — 백업 & 중앙 동기화 준비

### [ ] #19 Export/Import
`area:desktopd` `kind:feat` — depends #17
- 목적: PRD Task11
- 수용 기준: JSON export/import, SQLite 파일 백업, import 시 knowledge_item 중복 병합 확인

### [ ] #20 sync_outbox + push client skeleton
`area:desktopd` `kind:feat` — depends #19
- 목적: PRD Task12, §17
- 수용 기준: 중앙 서버 없이도 로컬 기능 정상 동작, API URL만 설정하면 outbox 전송 가능한 구조 (실제 전송 대상 서버는 이 이슈 범위 밖)
- 범위 밖: `apps/api` 중앙 서버 구현 자체 (별도 마일스톤)

---

## 독립 이슈

### [~] #21 한글 발음 유사 영어 후보 추론 — 방향 확정(2026-07-09), 구현 중
`area:desktopd` `kind:feat` — depends #4
- 목적: PRD §5.2-3 — "스테일"→stale, "리졸브"→resolve 같은 한글 발음 입력에서 영어 후보 제시
- 수용 기준: 입력 한글 발음에 대해 후보 N개(예: 3개) 반환, 사용자가 선택하면 해당 단어로 #4 파이프라인 진입
- **방향 결정(2026-07-09, Opus 외부조사 기반)**: **하이브리드(AI 우선 + 캐시), 단계적**. 근거: 역-음차는 외래어표기법 상 1:다 모호(→후보 N개 UX 정당), Metaphone류는 영어 전용이라 크로스스크립트 불가+개발용어 사전 부재+콜드스타트 치명적, Gemini flash-lite는 유명 개발 로안워드에 강하고 무료 1,500/일로 1인 충분+이미 통합, 선택 후보는 어차피 네트워크 필요한 explain 파이프라인行이라 오프라인 이점 미미
  - **Phase1(MVP)**: 순수 Gemini 구조화출력으로 후보 3개(english/confidence/gloss). 기존 Explainer/gemini 재사용, 신규 `suggest` 도메인 + `GET /v1/suggest`
  - **Phase2 완료(2026-07-09)**: 마이그레이션 `0004_suggest_cache.sql` + `internal/db/sqlite/suggest_repo.go`(Cached/SavePick) + suggest 도메인에 Repository·Candidate.Source(ai/cache) 추가, Service는 **캐시 우선**(hit→반환, miss→AI). `POST /v1/suggest/confirm`으로 확정 픽 저장(hit_count 누적, gloss COALESCE 보존). codex 리뷰 통과(블로커 0, NULLIF 중복 nit 반영). test/race/lint(0) 통과. E2E: miss→AI(source=ai)→confirm→동일 쿼리 cache(source=cache, AI 미호출)→재확인 hit_count=2. glossary 시드는 선택(미적용)
  - **Phase3(선택)**: 실 미스 축적 시 KoG2P 로마자화+편집거리 퍼지매칭
  - 참고 출처: KoG2P/g2pK, 외래어표기법, Gemini pricing/free-tier, transliteration 논문(backlog 결정 기록)
- **Phase1(MVP) 완료(2026-07-09)**: `internal/domain/suggest`(Suggester interface·Service·Mock) + gemini `Suggest`(전용 프롬프트/스키마, confidence clamp·빈 english drop) + `GET /v1/suggest?q=`(router 8번째 파라미터). gemini Client가 Explainer·Suggester 둘 다 구현(공용 `generate` 헬퍼로 retry 로직 공유). codex 리뷰 통과(블로커 0, provider 로그 nit 반영), test/lint(0) 통과. **실 Gemini 검증**: 스테일→stale/style/stall, 뮤텍스→mutex, 이디엠포턴트→idempotent, 카디널리티→cardinality. Phase2(픽 캐시)·Phase3(퍼지)는 후속

### [x] #22 review_card_candidate ↔ sub_item 매핑 (비-primary 단어 카드 생성) — 완료 (2026-07-09)
`area:desktopd` `kind:feat` — depends #8/#9 (홀리스틱 리뷰 발견, 2026-07-09)
- 결과(2026-07-09 완료, 방향 (a) 중첩 채택):
  - AI 계약 재구성: 최상위 `review_card_candidates` 제거, `explain.SubItem.CardCandidates`로 **각 sub_item 안에 중첩**. `responseSchema`도 `card_candidates`를 sub_items item 안으로 이동(minItems:1+required), 프롬프트에 "각 sub_item마다 해당 카드" 명시
  - `extractKnowledge`가 sub_item 루프에서 그 sub_item의 knowledge_item_id에 candidate 직접 연결(primary anchor·index 참조 불필요 → 할루시네이션 FK 없음). `review_card_candidates.knowledge_item_id`는 이제 항상 non-null
  - dedup된 중복 term의 candidate는 의도적으로 drop(첫 occurrence가 이미 저장)
  - codex 정확성 리뷰 통과(블로커 0). build/vet/race/lint(0)/govulncheck(0) 통과
  - **실 Gemini E2E**: "connection pool exhausted" → connection pool(0.9)·exhausted(0.7) 각각 candidate 1개 → **비-primary "exhausted" mark-unknown → cards_created=1**(이전엔 0) 확인
- 문제: 현재 AI `review_card_candidates`는 캡처의 **primary(importance 최고) knowledge_item에만** 연결된다. 따라서 비-primary 추출 단어를 mark-unknown하면 unconsumed candidate가 없어 **카드가 0개** 생성됨(learner 상태만 갱신). 실 Gemini E2E로 확인됨.
- 외부 조사 결론(2026-07-09, Opus): Gemini `responseSchema`는 배열 간 참조 무결성(한 배열 항목이 다른 배열 항목을 가리키는 FK)을 **지원하지 않음**(enum/required/minItems/정수 bound만). SR 카드 생성 툴들은 용어별 **구조적 중첩**으로 연결해 할루시네이션 FK를 회피.
- 방향(택1):
  - (a) **최선**: `review_card_candidates`를 top-level 배열이 아니라 **각 `sub_items` 항목 안에 중첩**(`card_candidates`). `SaveSuccess`의 sub_item 루프에서 그 sub_item의 knowledge_item_id에 바로 연결 → 참조 무결성 문제 자체가 사라짐
  - (b) 플랫 유지 시 candidate에 `target_sub_item_index`(0-based 정수, `minimum:0`+required) 추가. `target_normalized_key`(문자열)보다 정규화 드리프트에 강함
- 공통 주의(스키마가 강제 못 하므로 앱에서 검증): 인덱스 범위 초과/미스매치 → **현재 primary anchor로 폴백**(오늘 동작보다 나빠지지 않음), sub_items 재정렬 전 원본 순서로 인덱스 바인딩, `consumed_at` 멱등성은 그대로 유효
- 참고: Gemini structured-output 문서, LLM structured-output 검증 가이드(schema-valid≠semantic-valid), SR 플래시카드 LLM 생성 사례
