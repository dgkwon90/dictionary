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

### [ ] #7 Knowledge/Learner item 추출
`area:desktopd` `kind:feat` — depends #4 (mock의 `sub_items`로 개발·테스트 가능, 실제 provider #6는 선행 조건 아님)
- 목적: AI `sub_items`를 `knowledge_items`로 upsert, `learner_items` 갱신 (PRD Task05)
- 수용 기준:
  - 같은 단어는 `normalized_key`+`item_type` 기준 하나의 knowledge_item으로 병합
  - `ask_count` 증가, `first_seen_at`/`last_seen_at` 갱신
  - `capture_items` 연결(role, confidence)

### [ ] #8 Mark-unknown API + review card candidate
`area:desktopd` `kind:feat` — depends #7
- 목적: `POST /v1/knowledge/{item_id}/mark-unknown` (§15.4), AI가 생성한 `review_card_candidates`를 다음 이슈(#9)가 소비할 수 있는 형태로 보존
- 수용 기준:
  - mark-unknown 호출 시 `learner_items` 상태 갱신(`wrong_count`/`last_wrong_at` 등) 확인 테스트
  - 해당 knowledge item의 card candidate(§12.1 `review_card_candidates`)가 조회 가능한 형태로 저장·연결됨
  - "알고 있음" 표시 API도 함께 (§5.2-6 Inbox 동작 세트의 반대 방향)

---

## Milestone: v0.3 — 복습 엔진

### [ ] #9 Review card 생성
`area:desktopd` `kind:feat` — depends #8
- 목적: PRD Task06 — candidate 기반 카드 생성, `due_at` 설정
- 수용 기준: "모름" 처리한 단어에 대해 `review_cards` row 생성, due card 조회 가능

### [ ] #10 Review session + FSRS-lite 스케줄링
`area:desktopd` `kind:feat` — depends #9
- 목적: PRD Task07, §13.1 — `POST /v1/reviews/session/start`(§15.5), `POST /v1/reviews/{card_id}/grade`(§15.6)
- 수용 기준:
  - Again/Hard/Good/Easy 처리, §13.1 간격 규칙(초기 10분/1일/3일/7일, 이후 배율 적용) 구현
  - `review_logs` append-only 저장

### [ ] #11 mastery/weakness score 계산
`area:desktopd` `kind:feat` — depends #10
- 목적: PRD §13.2~13.3 계산식 구현, `learner_items` 갱신
- 수용 기준: review 완료마다 재계산, 0.0~1.0 clamp 확인 테스트

### [ ] #12 Dashboard summary API
`area:desktopd` `kind:feat` — depends #11
- 목적: PRD Task08, §15.7
- 수용 기준: `GET /v1/dashboard/summary`로 오늘/주간 검색 수, 많이 검색·많이 틀린 단어, 카테고리별 약점, due card 수 반환

---

## Milestone: v0.4 — Tauri 데스크톱 UI

### [ ] #13 Tauri 셸 부트스트랩
`area:desktop-ui` `kind:feat` — depends #5, **ADR-0005 확정 필요**
- 목적: 트레이 아이콘, 기본 윈도우, desktopd 프로세스 생명주기 관리(자식 프로세스로 실행/종료), 로컬 API 클라이언트 골격 (PRD Task09 일부)
- 수용 기준: 트레이 메뉴(Quick Search/Inbox/Today Review/Dashboard/Settings/Quit, §10.1)에서 각 항목 클릭 시 해당 창(빈 화면이라도)이 뜬다

### [ ] #14 글로벌 단축키 + Quick Search 화면
`area:desktop-ui` `kind:feat` — depends #13, #4
- 목적: PRD §5.2-2, §10.2, §9.1~9.3 흐름
- 수용 기준:
  - 단축키(macOS `Cmd+Shift+E`, Win/Linux `Ctrl+Shift+E`)로 어디서든 팝업 호출
  - 클립보드 자동 삽입 + 직접 입력 두 경로 모두 동작, `POST /v1/captures` 연동

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

### [ ] #21 한글 발음 유사 영어 후보 추론
`area:desktopd` `kind:feat` — depends #4, 알고리즘 방향 확정 필요
- 목적: PRD §5.2-3 — "스테일"→stale, "리졸브"→resolve 같은 한글 발음 입력에서 영어 후보 제시
- 수용 기준(안): 입력 한글 발음에 대해 후보 N개(예: 3개) 반환, 사용자가 선택하면 해당 단어로 #4 파이프라인 진입
- 미해결: 후보 생성 방식 — (a) 정적 음차 사전/편집 거리 매칭 (b) AI 호출 기반 추론. 비용·정확도 트레이드오프 있음 — 착수 전 결정 필요
