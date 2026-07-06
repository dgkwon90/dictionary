# ADR-0003: SQLite 로컬 우선 저장소

- 날짜: 2026-07-03 / 상태: 승인

## 맥락
"Local-first" 원칙(PRD §2.2, §4)상 서버 없이도 전체 기능이 동작해야 한다. 단일 사용자, 단일 프로세스(desktopd)가 배타적으로 접근하는 로컬 파일 DB가 필요하다.

## 결정
SQLite, WAL 모드, FTS5(검색용)를 로컬 저장소로 채택한다. 스키마는 PRD §11의 정의(`captures`, `lookup_jobs`, `explanations`, `knowledge_items`, `capture_items`, `learner_items`, `review_cards`, `review_logs`, `reminders`, `sync_outbox`)를 그대로 따르고, 변경은 migration 파일로만 한다.

## 근거
- 단일 사용자 로컬 앱에 가장 단순한 배포(파일 하나) — 별도 DB 서버 불필요
- WAL 모드로 desktopd 내 동시 읽기/쓰기(HTTP 핸들러 다수 + 백그라운드 스케줄러)를 지원
- 향후 중앙 서버(PostgreSQL, PRD §16.3)로의 데이터 이전 경로가 `sync_outbox`(append-only 이벤트)로 이미 설계되어 있음

## 결과·트레이드오프
- 장점: 백업이 파일 복사로 끝남 (PRD §5.2-10 내보내기 기능과 자연히 맞음)
- 트레이드오프: 다중 기기 동시 편집은 지원하지 않음 — PRD §17이 명시하듯 실시간 동기화는 범위 밖이며, 충돌은 이벤트 기반 재계산으로 처리
