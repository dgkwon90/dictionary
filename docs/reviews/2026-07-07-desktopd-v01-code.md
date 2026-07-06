# tri-review 판정 요약 — desktopd #1+#2 구현 코드

- 날짜: 2026-07-07 / 대상: `apps/desktopd` 전체 (bootstrap, config, logger, router, db, migrations, Makefile)
- 엔진: codex(구현·엣지케이스), agy(아키텍처 — Gemini 쿼터 소진으로 Claude Sonnet 모델로 실행). Claude 종합 판정.
- 참고: agy CLI는 Gemini 쿼터 소진 시 exit 0 + 빈 stdout으로 조용히 실패한다. `--model` 지정으로 우회 가능.

| # | 지적 | 출처 | 심각도 | 판정 | 조치 |
|---|---|---|---|---|---|
| 1 | 마이그레이터가 "미지 버전"(DB에 있는데 임베드에 없는 버전)을 검출 안 함 — 구버전 바이너리가 신버전 DB를 열 수 있음 | codex | high | **수용** | #3 착수 전 migrate.go에 양방향 비교 추가 + 테스트 |
| 2 | BEGIN IMMEDIATE 동시 마이그레이션의 회귀 테스트 부재 | codex | med | **수용** | #1 조치와 함께 병렬 Migrate 테스트 추가 |
| 3 | lookup_jobs.status 등 다른 컬럼엔 CHECK 없음(inbox_status만 제약해 비일관) | codex | med | **기각(원칙 유지)** | #2 수용 기준이 "PRD §11 그대로"였음. 상태값 무결성은 쓰기 주체인 domain 계층(#3+)이 강제 — 각 이슈 수용 기준에 반영 |
| 4 | 실패 rollback이 context.Background()라 이론상 무기한 대기 | codex | med | **수용(경미)** | timeout 있는 context로 교체 (#1 조치와 함께) |
| 5 | http.Server에 ReadHeaderTimeout 등 방어 설정 없음 | codex | low | **수용** | #3(요청 본문 받기 시작)에서 타임아웃 설정 추가 |
| 6 | bootstrap의 ready/mutex/Addr() 상태 관리 과설계 — listener 외부 주입이 단순 | codex+agy | low/med | **보류** | agy의 "Addr() 미사용" 주장은 사실 오류(bootstrap_test.go:22에서 사용, 테스트 결정성 목적). 단 listener 주입 리팩터는 #3에서 라우터 시그니처 바뀔 때 함께 검토 |
| 7 | NewRouter(log)에 의존성 슬롯이 없어 #3 handler 못 얹음 | agy | high | **수용(시점 조정)** | 사실이나 "지금 고칠 것"은 아님 — #3에서 handler/서비스 주입으로 시그니처 확장(자연스러운 진화). bootstrap은 composition root 역할이 맞음(PRD §14.2 app/bootstrap) |
| 8 | 이후 마이그레이션 네이밍 규칙 부재 | agy | high(주장) | **수용(문서만)** | 규칙 추가: `NNNN_<도메인>_<요약>.sql`, 적용된 파일 수정 금지(checksum) — 이 문서와 glossary 아닌 rules에 기록할 정도는 아니고 ADR-0007이 이미 원칙 보유. #3부터 준수 |
| 9 | db.Open에 커넥션 풀 설정 없음 — 동시 쓰기 시작 시 writer 경합 | agy | med | **수용(시점 명시)** | ADR-0007이 이미 "#3+에서 확정"으로 스코프. 동시 쓰기 등장하는 이슈에서 SetMaxOpenConns(1) 또는 _txlock=immediate 적용 |
| 10 | sync_outbox를 소비자 없이 선투입 — 지금 빼라 | agy | med | **기각** | 백로그 #2 수용 기준이 명시적으로 "sync_outbox, reminders까지 스키마 한 번에" 요구. PRD §11 전체 적용이 요구사항 |
| 11 | internal/logger 3줄 wrapper 과함 | agy | low | **기각** | PRD §14.2 패키지 구조에 logger/ 명시. 구조 준수 |
| 12 | agy "8개 테이블" 등 수치 오류 | — | — | 참고 | 실제 11개 테이블. 외부 엔진 사실 주장은 검증 후 수용 원칙 재확인 |

## 종합
아키텍처 방향(경계·마이그레이션·수명주기)은 유효. **즉시 조치 4건**(#1·2·4: migrate.go 보강+테스트, #8: 네이밍 준수)은 #3 착수 전 일괄 처리 권장, **#3에 얹을 것 2건**(#5 서버 타임아웃, #7 라우터 의존성 주입), **#10 전 1건**(#9 writer 직렬화).
