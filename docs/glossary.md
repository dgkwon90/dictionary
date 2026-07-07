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

## Reminder (리마인더)
정해진 시간에 복습을 유도하는 알림.

## Outbox (아웃박스)
로컬에서 생긴 변경 이벤트를 중앙 서버로 나중에 전송하기 위해 쌓아두는 테이블(`sync_outbox`).

## Capture (캡처)
사용자가 검색을 위해 입력한 원문 1건. `captures` 테이블의 row 단위. "검색 기록"과 동의어로 쓰지 않는다 — capture는 원문 자체, explanation은 그 결과.

## Inbox status (인박스 상태)
Inbox 화면(PRD §10.4)의 항목(=capture) 분류. **저장하는 값은 사용자 소유 상태 3종뿐**: `captures.inbox_status` = `new`(기본) / `saved` / `archived`. 화면 탭 중 **Review Added·Failed는 저장하지 않고 조회 시 도출**한다 — Review Added는 capture에서 나온 knowledge item에 review card가 있는지로, Failed는 최신 `lookup_jobs.status=failed`로 판정(도출 근거·설계: ADR-0007). capture:review_card는 1:N이므로 `review_added`를 컬럼에 저장하지 않는다.

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
