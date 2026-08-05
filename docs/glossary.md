# 용어집 (Glossary)

새 도메인 용어는 등장한 커밋에서 여기 등재한다. 같은 개념에 두 단어를 쓰지 않는다 — 이 문서가 표준어를 결정한다. (`docs/rules/development-cycle.md` 참조)

원본 정의: `docs/prd.md` §4. 이후 새 용어는 이 문서에 추가하고 PRD는 원본 그대로 둔다.

## Local-first (로컬 퍼스트)
데이터와 핵심 기능을 서버가 아니라 사용자 PC에 먼저 저장하고 실행하는 방식. Neulsang은 초기에는 서버 없이도 동작해야 한다.

## Sidecar (사이드카)
메인 앱 옆에서 함께 실행되는 보조 프로세스. Neulsang에서는 Tauri UI 옆에 Go sidecar(`desktopd`) 프로세스를 실행한다.

## System Tray (시스템 트레이)
Windows 우측 하단, macOS 메뉴바, Linux 패널에 상주하는 작은 앱 아이콘. Neulsang은 트레이 앱으로 항상 접근 가능해야 한다.

## Global Shortcut (글로벌 쇼트컷)
앱이 포커스되어 있지 않아도 동작하는 전역 단축키. 초기값: macOS `Cmd+Shift+E`, Windows/Linux `Ctrl+Shift+E`.

## Clipboard (클립보드)
사용자가 복사한 텍스트가 임시 저장되는 OS 영역. 초기 MVP는 선택 텍스트를 직접 읽기보다 클립보드를 읽는 방식을 우선한다.

## Search History (검색 기록)
찾아본 것이 쌓이고, 각각을 학습에 담을지 버릴지 정하는 화면.
**개명(2026-08-05, 재설계)**: 이전 이름은 "Inbox / 인박스 / 검색함"이었다. 인박스 모델은
새 것/저장/보관으로 분류하는 축이었는데 실사용에서 저장·보관의 뜻이 모호했고, 실제로
중요한 축은 "이 검색을 어떻게 할지 정했는가"였다 — 그래서 화면과 함께 이름도 바꿨다.
도메인·패키지명도 `search`다(`internal/domain/search`).
Route 식별자(`"Search History"`)는 Go(`notification.RouteSearchHistory`)·Rust(`tray.rs`)·
프론트(`labels.ts`의 `ROUTES`) 세 언어가 같은 문자열을 쓰기로 한 계약이고, 컴파일러가
연결해 주지 않는다. **DB `notifications.route`에 저장까지 되므로 개명하려면 세 곳 + 저장된
행(마이그레이션)을 한 번에 바꿔야 한다** — 0002가 그 선례다.

## Review (리뷰)
저장된 단어·용어·문장을 다시 학습하는 과정. "코드 리뷰"의 리뷰와 혼동 주의 — 이 문서와 코드에서 "review"는 항상 복습을 의미한다.
**채점 라벨(2026-08-05 갱신)**: `again`=모르겠어요, `hard`=어려웠어요, `good`=알아요,
`easy`=쉬워요. **복습·연습·설정 세 화면이 글자 그대로 같은 말을 쓴다** — 설정에서
"모르겠어요 10분 뒤"를 고쳐 놓고 복습에 와서 다른 단어를 보면 그게 그 버튼인지 알 수 없다.
(옛 라벨 "다시/어려움/보통/쉬움"은 "다시"가 "지금 바로 한 번 더"로 읽혀 실제로 혼동을
일으켰다. 지금 바로 다시 보는 것은 별도 버튼 [한 번 더 (연습)]이다.)
`ReviewRating` 값(again/hard/good/easy)은 서버 계약이라 그대로 두고 표시 문구만 바꾼 것.

## Card type (카드 유형, `review_cards.card_type`)
Gemini가 `card_candidates` 생성 시 고르는 카드 형태(`internal/infra/llm/gemini/schema.go`
enum). 값 자체는 안 바꾸고, 화면 표시 라벨만 `src/labels.ts`의 `cardTypeLabel()`로 통일한다
(2026-07-24, Review·Practice 화면에서 공유): `meaning`=뜻 맞추기, `reverse`=영어로 떠올리기,
`cloze`=빈칸 채우기, `context`=쓰임 고르기, `sentence_translation`=문장 해석하기. PRD §5.2
"학습 카드 생성"의 카드 유형 목록과 대응.

## Practice (연습)
복습 **스케줄(due)과 무관하게** 사용자가 고른 카드를 반복 학습하는 모드(#27·#28). 전용 Practice
탭에서 `GET /v1/practice/cards`(due·status 무시 조회)로 단어를 골라 연습한다.
**v2 갱신**: 연습도 이제 채점한다(`POST /v1/practice/{id}/grade`) — `review_logs(source='practice')`
1행과 정답률 카운터를 쓴다("연습도 실력이다"). 단 **`review_cards`의 `due_at`/`state`/`reps`/
`interval_days`는 건드리지 않는다**: 아무리 연습해도 내일 복습할 목록은 그대로라, "복습 카드를
미리 소진할까 봐 연습을 못 하는" 상황이 생기지 않는다. review 도메인 안에 있다(별도 도메인 아님).

## Reminder (리마인더)
정해진 시간에 복습을 유도하는 알림.

## Dashboard (대시보드)
학습 지표(오늘/주간 검색 수, 완료 복습 수, due card 수)와 많이 검색·많이 틀린 단어,
카테고리별 약점을 보여주는 읽기전용 화면(#12/#17, `GET /v1/dashboard/summary`).
**화면 표시명(2026-07-24 갱신)**: 외래어라 쉽지 않다는 피드백으로 실제 화면 탭은
"내 기록"을 쓴다(`App.tsx`의 `LABELS.Dashboard`). "대시보드"는 코드·문서의 도메인
개념명으로 유지하고, Route 식별자(`"Dashboard"`)도 안 바뀐다.

## Outbox (아웃박스)
로컬에서 생긴 변경 이벤트를 중앙 서버로 나중에 전송하기 위해 쌓아두는 테이블(`sync_outbox`).

## Capture (캡처)
사용자가 검색을 위해 입력한 원문 1건. `captures` 테이블의 row 단위. "검색 기록"과 동의어로 쓰지 않는다 — capture는 원문 자체, explanation은 그 결과.

## Triage state (분류 상태)
검색 하나를 사용자가 어떻게 하기로 했는지. `captures.triage_state` — `unseen`(아직 안 정함) /
`needs_selection`(문장을 열었고 모르는 단어를 고르는 중) / `learning`(학습 목록에 담김) /
`discarded`(버림, 소프트 삭제). v1의 `inbox_status`(new/saved/archived + 파생 탭)를 대체한다.
축이 "어디에 보관하는가"에서 **"정했는가"**로 바뀐 것이 재설계의 핵심이다(ADR-0010).
해석 성공/실패는 별개 축이다 — `lookup_jobs.status`에서 읽으며 triage와 섞지 않는다.

## Learn kind (단어/문장)
학습 대상의 종류. `word` / `sentence` 두 가지뿐이며 `captures.learn_kind`·`knowledge_items.learn_kind`에
저장된다. 서버가 AI의 `input_type`(5지선다, 참고용으로만 보존)을 보고 정하고, 어긋나면 **서버가
이긴다**. 사용자가 결과 화면에서 뒤집을 수 있다(`POST /v1/searches/{id}/kind`). 문장으로 정해지면
단어 선택 흐름을 타고, 단어면 한 번에 학습 목록에 담긴다.

## Learning list (학습 목록)
사용자가 **배우기로 한 것만** 모인 화면·개념(`internal/domain/learning`). 검색 기록과 다른
것이며, 이 목록에 들어오는 유일한 경로는 검색 결과에서의 명시적 등록이다(단어 [학습할래요],
문장 [모르는 단어 고르기 → 완료]). 나가는 문은 둘 — [알겠어요](`status='known'`, "배웠다")와
[목록에서 빼기](`status='removed'`, "잘못 담았다"). 둘 다 복습에서 빠지고 행은 남으며,
[제외한 것] 보기에서 되돌린다. 차이는 나중에 그 기록을 어떻게 읽을지뿐이다.

## Knowledge item (지식 항목)
단어·용어·구·문장 단위로 정규화된 학습 대상. `knowledge_items` 테이블. 여러 capture에서 같은 knowledge item이 반복 추출될 수 있다.

## Learner item (학습자 항목)
사용자가 특정 knowledge item을 **배우기로 했다**는 사실과 그 이후의 기록. `learner_items` 테이블.
행이 있다는 것 자체가 "학습 목록에 있다"는 뜻이다 — v1처럼 검색만 해도 생기지 않는다(ADR-0010).
`status`(active/known/removed), `ask_count`(찾아본 횟수), `unknown_count`(모른다고 선언한 횟수),
`attempt_count`/`correct_count`(복습+연습 채점 누적), `registered_at`(담은 시각).

## Lookup job (조회 작업)
capture 1건에 대한 AI 해석 작업 단위. `lookup_jobs` 테이블, 상태는 `queued`/`running`/`done`/`failed`.
capture 생성 시 `queued`로 함께 생성된다(#3). 검색 기록의 "해석 중"·"해석하지 못했어요"는 이 상태에서
읽는다. 실패한 것은 `POST /v1/searches/{id}/retry`가 **같은 capture에 job을 하나 더** 걸며, 목록은
언제나 최신 job을 본다(실패 기록은 이력으로 남는다).

## Explainer (익스플레이너)
capture 원문을 받아 `ExplainResult`(PRD §12.1)를 반환하는 AI 해석 추상화. `internal/domain/explain.Explainer` interface. 1차 구현은 `MockExplainer`(#4, 결정적 목업 응답), 실제 provider(Gemini)는 #6에서 이 interface 뒤에 구현체로 추가된다.

## Accuracy / Weakness score (정답률 / 약점 점수)
**정답률 = `correct_count / attempt_count`**, 저장하지 않고 계산한다(원장과 어긋날 수 없게).
`again`만 오답이고 `hard`는 정답이다 — "떠올렸으나 힘들었다"를 오답 처리하면 정직한 자가보고를
처벌해 등급 부풀리기를 부른다. weakness_score는 얼마나 취약한지의 지표로 남아 있다(`review.WeaknessScore`,
많이 찾아보고 자주 모르겠다고 했는데 정답률은 낮은 쪽이 위로).
**`mastery_score`는 v2에서 삭제됐다**(ADR-0010 D5) — 정답률이 아니면서 그 자리를 차지했다.

## Export / Import / Backup (내보내기 / 가져오기 / 백업)
로컬 학습 데이터의 이식·백업 기능(#19, PRD Task11). `internal/domain/backup` 도메인이 총괄한다. **Export**=학습 코어 7테이블을 JSON 스냅샷으로(`GET /v1/export`), **Import**=그 JSON을 멱등·비파괴로 병합(`POST /v1/import`, knowledge_item은 `(normalized_key,learn_kind)`로 중복 제거), **Backup**=SQLite 파일 스냅샷(`POST /v1/backup`, `VACUUM INTO`). 운영/파생 테이블(lookup_jobs·notifications·suggest_cache·sync_outbox 등)은 export 대상이 아니다. 중앙 서버 동기화(`sync_outbox`)는 별개 기능(#20).

## Sync outbox / Push client (아웃박스 / 푸시 클라이언트)
로컬 변경 이벤트를 나중에 중앙 서버로 보내기 위해 쌓아두는 **아웃박스 패턴**(#20, PRD Task12/§6.1). `sync_outbox` 테이블에 도메인 변경과 **같은 트랜잭션에서 원자적으로** 이벤트를 기록한다(현재 writer=capture 생성의 `capture_created`). `internal/domain/outbox`가 읽기·전송을 담당: `NEULSANG_SYNC_URL`이 설정된 경우에만 백그라운드 flush 루프가 미전송(`acked_at IS NULL`) 이벤트를 oldest-first로 POST하고 2xx면 acked 처리(at-least-once, `event_id` UNIQUE로 서버측 중복 제거). URL 미설정 시 이벤트는 조용히 쌓이기만 하고 로컬 기능은 완전 정상. 중앙 서버(`apps/api`) 자체는 별도 마일스톤.
