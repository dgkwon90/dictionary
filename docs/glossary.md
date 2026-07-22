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

## Inbox / Queue (인박스 / 큐)
검색 요청이 들어오고 결과가 쌓이는 공간. 사용자는 나중에 결과를 확인할 수 있다.

## Review (리뷰)
저장된 단어·용어·문장을 다시 학습하는 과정. "코드 리뷰"의 리뷰와 혼동 주의 — 이 문서와 코드에서 "review"는 항상 복습을 의미한다.

## Practice (연습)
복습 **스케줄(due)과 무관하게** 사용자가 고른 카드를 반복 학습하는 모드(#27·#28). Review(복습)와 달리 **채점(grade)이 없어 서버에 아무것도 쓰지 않는다** → `due_at`·mastery·`review_logs`가 바뀌지 않는다(순수 자가확인). #27은 복습 세션 안에서 방금 본 카드를 세션 큐에 재삽입하는 "한 번 더", #28은 전용 Practice 탭에서 `GET /v1/practice/cards`(due 무시 조회)로 임의 단어를 골라 연습. review 도메인의 읽기 메서드로 구현(별도 도메인 아님).

## Reminder (리마인더)
정해진 시간에 복습을 유도하는 알림.

## Outbox (아웃박스)
로컬에서 생긴 변경 이벤트를 중앙 서버로 나중에 전송하기 위해 쌓아두는 테이블(`sync_outbox`).

## Capture (캡처)
사용자가 검색을 위해 입력한 원문 1건. `captures` 테이블의 row 단위. "검색 기록"과 동의어로 쓰지 않는다 — capture는 원문 자체, explanation은 그 결과.

## Inbox status (인박스 상태)
Inbox 화면(PRD §10.4)의 항목(=capture) 분류. **저장하는 값은 사용자 소유 상태 3종뿐**: `captures.inbox_status` = `new`(기본) / `saved` / `archived`. 화면 탭 중 **Review Added·Failed는 저장하지 않고 조회 시 도출**한다 — Review Added는 해당 capture의 `review_card_candidates`가 실제 카드로 소비됐는지(`consumed_at IS NOT NULL`)로, Failed는 최신 `lookup_jobs.status=failed`로 판정(도출 근거·설계: ADR-0007). capture:review_card는 1:N이므로 `review_added`를 컬럼에 저장하지 않는다.

## Knowledge item (지식 항목)
단어·용어·구·문장 단위로 정규화된 학습 대상. `knowledge_items` 테이블. 여러 capture에서 같은 knowledge item이 반복 추출될 수 있다.

## Learner item (학습자 항목)
사용자가 특정 knowledge item을 얼마나 아는지 나타내는 상태(`familiarity_score`, `mastery_score` 등). `learner_items` 테이블.

## Lookup job (조회 작업)
capture 1건에 대한 AI 해석 작업 단위. `lookup_jobs` 테이블, 상태는 `queued`/`running`/`done`/`failed`(PRD §11.3). capture 생성 시 `queued`로 함께 생성된다(#3). Inbox의 Failed 탭은 이 상태에서 도출한다 — 위 "Inbox status" 항목 참조.

## Explainer (익스플레이너)
capture 원문을 받아 `ExplainResult`(PRD §12.1)를 반환하는 AI 해석 추상화. `internal/domain/explain.Explainer` interface. 1차 구현은 `MockExplainer`(#4, 결정적 목업 응답), 실제 provider(Gemini)는 #6에서 이 interface 뒤에 구현체로 추가된다.

## Mastery score / Weakness score
mastery_score는 얼마나 잘 아는지, weakness_score는 얼마나 취약한지의 지표. 계산식은 `docs/prd.md` §13.2~13.3. 두 값은 반대 개념이지만 별도 필드로 관리하며 한쪽에서 다른 쪽을 유도해 대체하지 않는다.

## Export / Import / Backup (내보내기 / 가져오기 / 백업)
로컬 학습 데이터의 이식·백업 기능(#19, PRD Task11). `internal/domain/backup` 도메인이 총괄한다. **Export**=학습 코어 7테이블을 JSON 스냅샷으로(`GET /v1/export`), **Import**=그 JSON을 멱등·비파괴로 병합(`POST /v1/import`, knowledge_item은 `(normalized_key,item_type)`로 중복 제거), **Backup**=SQLite 파일 스냅샷(`POST /v1/backup`, `VACUUM INTO`). 운영/파생 테이블(lookup_jobs·notifications·suggest_cache·sync_outbox 등)은 export 대상이 아니다. 중앙 서버 동기화(`sync_outbox`)는 별개 기능(#20).

## Sync outbox / Push client (아웃박스 / 푸시 클라이언트)
로컬 변경 이벤트를 나중에 중앙 서버로 보내기 위해 쌓아두는 **아웃박스 패턴**(#20, PRD Task12/§6.1). `sync_outbox` 테이블에 도메인 변경과 **같은 트랜잭션에서 원자적으로** 이벤트를 기록한다(현재 writer=capture 생성의 `capture_created`). `internal/domain/outbox`가 읽기·전송을 담당: `NEULSANG_SYNC_URL`이 설정된 경우에만 백그라운드 flush 루프가 미전송(`acked_at IS NULL`) 이벤트를 oldest-first로 POST하고 2xx면 acked 처리(at-least-once, `event_id` UNIQUE로 서버측 중복 제거). URL 미설정 시 이벤트는 조용히 쌓이기만 하고 로컬 기능은 완전 정상. 중앙 서버(`apps/api`) 자체는 별도 마일스톤.
