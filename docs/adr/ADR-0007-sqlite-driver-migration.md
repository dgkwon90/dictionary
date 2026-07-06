# ADR-0007: SQLite 드라이버와 마이그레이션 방식

- 날짜: 2026-07-07 / 상태: 승인 (codex/agy tri-review 반영, 요약: `docs/reviews/2026-07-07-adr-0007.md`)

## 맥락
ADR-0003이 SQLite(WAL, FTS5)를 로컬 저장소로 확정했으나, Go 드라이버와 스키마 마이그레이션 적용 방식은 미정이다. 백로그 #2(SQLite schema & migration) 착수를 위해 아래 두 가지를 결정해야 한다.

1. **드라이버**: `database/sql`을 구현하는 SQLite 드라이버 선택
2. **마이그레이션**: PRD §11의 11개 테이블을 앱 시작 시 idempotent하게 적용하는 방법

제약:
- 프로젝트 규칙(`rules/development-cycle.md`): 의존성 최소, 라이선스 Apache-2.0/MIT/BSD/MPL-2.0 우선
- FTS5 필요(ADR-0003) — 드라이버가 FTS5를 지원해야 함
- 배포 대상: macOS(arm64/amd64)·Windows·Linux 데스크톱 → 크로스 컴파일·배포 단순성이 중요
- Tauri sidecar로 번들되는 단일 바이너리여야 함

## 결정

### 드라이버: `modernc.org/sqlite` (순수 Go, CGO 불필요)
- FTS5 내장(기본 빌드에 포함), `database/sql` 표준 인터페이스 구현
- CGO 없음 → `GOOS`/`GOARCH`만 바꿔 크로스 컴파일 가능, C 툴체인 불필요 → Tauri 다중 플랫폼 번들이 단순
- 라이선스: BSD-3-Clause 계열(허용)

### 마이그레이션: 임베드된 SQL 파일 + 최소 자체 마이그레이터
- `internal/db/migrations/NNNN_*.sql`을 `embed.FS`로 바이너리에 포함
- `schema_migrations(version INTEGER PRIMARY KEY, checksum TEXT NOT NULL, applied_at DATETIME NOT NULL)` 테이블로 이력 추적
- **원자성**(리뷰 반영): 각 마이그레이션의 DDL과 `schema_migrations` INSERT를 **하나의 트랜잭션**에서 실행, 실패 시 rollback. "idempotent"는 미적용 버전만 골라 재실행해도 안전하다는 뜻이며, 원자성과 별개 개념으로 구분해 기술
- **동시 시작 방어**(리뷰 반영): 마이그레이션 트랜잭션은 `BEGIN IMMEDIATE`로 시작해 두 프로세스 동시 기동 시 한쪽만 진행
- **변조 감지**(리뷰 반영): 각 파일의 checksum(SHA-256)을 기록하고, 이미 적용된 버전의 파일이 사후 변경되면 시작 시 에러(적용된 마이그레이션은 수정 금지, 새 버전으로만)
- 외부 라이브러리(golang-migrate/goose) 미도입 — up-only 단일 사용자 로컬 앱에 과하고 의존성 최소 규칙. 단, 데이터 가공형(Go 코드) 마이그레이션이 필요해지거나 자체 마이그레이터가 커지면 **goose 도입을 escape hatch로** 재검토(후속 ADR)

### 연결 시 PRAGMA (per-connection, 리뷰 반영)
- `foreign_keys`·`busy_timeout`은 **연결별** 설정이므로 modernc DSN의 반복 `_pragma`로 적용: `?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)` — `database/sql` 풀이 새 연결을 열 때마다 보장됨 (`db.Exec` 1회로는 불충분)
- **쓰기 직렬화**: WAL은 동시 writer를 늘려주지 않음. 동시 쓰기가 실제로 등장하는 시점(#3+)에 writer 풀 `SetMaxOpenConns(1)` 또는 `_txlock=immediate` 정책을 확정한다. #2(스키마·시작 시 마이그레이션)는 단일 연결·무경합이라 이 결정을 선행 요구하지 않음

### 갭 2 해소: Inbox 상태 컬럼 (리뷰 반영 — 저장값 축소)
- `captures.inbox_status TEXT NOT NULL DEFAULT 'new'` + `CHECK(inbox_status IN ('new','saved','archived'))`
- **저장하는 것은 사용자 소유 상태뿐**: `new`(기본) / `saved` / `archived`. 이것들은 다른 진실의 원천이 없는 사용자 분류라 컬럼으로 저장
- **`review_added`·`failed`는 저장하지 않고 조회 시 도출**: capture→knowledge_item→review_card 존재 여부로 Review Added, 최신 `lookup_jobs.status=failed`로 Failed 판정. capture:card는 1:N이라 단일 컬럼에 `review_added`를 저장하면 "여러 단어 중 일부만 복습 추가"·"카드 삭제 후 롤백"을 표현 못 함 → 파생값으로 처리해 정합성 붕괴를 차단. 도출 쿼리는 #5(Inbox API)에서 구현
- PRD §11.2 원본은 두고, 이 컬럼과 상태 정의는 마이그레이션·glossary에 기록

## 근거
- 순수 Go 드라이버는 CGO 크로스 컴파일 고통(C 툴체인, `CGO_ENABLED=1` 플랫폼별 빌드)을 제거 — 1인 개발·다중 플랫폼 배포에 결정적
- 자체 마이그레이터는 stdlib(`embed`, `database/sql`)만으로 충분하고, up-only·단일 사용자 로컬 앱에서 라이브러리 도입 대비 이득이 없음

## 승인 조건 (리뷰 반영 — 구현 시 검증)
- modernc.org/sqlite **버전을 go.mod에 고정**하고, 그 버전에서 통합 테스트 통과: (a) `CREATE VIRTUAL TABLE ... USING fts5` 생성+검색 smoke test로 FTS5 실동작 확인, (b) `journal_mode` 조회로 WAL 확인, (c) FK 위반 INSERT가 거부되는지로 `foreign_keys` 적용 확인, (d) 마이그레이터 재실행 idempotent·checksum 변조 감지 테스트
- 지원 대상: darwin(arm64/amd64)·windows(amd64)·linux(amd64). 최소 Go 1.26
- 실제 FTS5 가상 테이블 스키마·동기화 트리거 설계는 **범위 밖**(백로그 #2 명시) — 검색 기능 착수 이슈에서 별도 결정

## 결과·트레이드오프
- 장점: 외부 의존성 최소(드라이버 1개 + 전이 의존성), 크로스 컴파일 단순(C 툴체인 불필요), 배포 바이너리 하나
- 트레이드오프(모니터링): `modernc.org/sqlite`는 ccgo로 SQLite C를 트랜스파일한 것이라 `unsafe` 사용이 많고 CGO 기반 `mattn/go-sqlite3`보다 무거운 워크로드에서 느릴 수 있음. 단일 사용자 로컬 워크로드에선 수용 가능하나, FTS/동시 쓰기 성능 이슈가 실측되면 드라이버 재검토(후속 ADR). CGO 크로스컴파일 부담 회피가 이 트레이드오프를 상쇄
- 트레이드오프: 자체 마이그레이터는 down/rollback 미지원 — 로컬 앱은 파일 백업·재생성으로 충분(PRD §5.2-10 export). 개발 편의로 `make db-reset`(DB 파일 삭제) 제공
