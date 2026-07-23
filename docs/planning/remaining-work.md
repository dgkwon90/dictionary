# Remaining Work

- 기준일: 2026-07-23
- 기준 버전: `fad4fe3` (`v0.1.0`)
- 입력: [`backlog.md`](backlog.md), [`2026-07-22-project-review.md`](../reviews/2026-07-22-project-review.md), PRD MVP 완료 기준
- 목적: 완료 이력과 분리된 **현재 실행 목록**. 구현이 끝나면 이 문서의 체크박스와 원본 backlog 기록을 함께 갱신한다.
- **codex 교차검증(2026-07-23)**: R-01~R-11 전 항목 CONFIRMED(사실관계 이상 없음). RW-03(R-03, sidecar 강제종료)은 영향 범위가 capture 1건이 `running`에 고착되는 정도라 리뷰의 High보다 낮은 심각도가 적절하다는 의견 — 다만 RW-04(R-02, 백업 복원 실패)의 "조용한 데이터 손실"이 사용자 피해가 더 크므로 두 항목을 동급 우선순위(P0 병렬)로 유지하는 현재 구성이 적절.
- **완료(2026-07-23)**: RW-01~RW-10 전 항목을 이슈별 독립 브랜치로 구현→codex 교차검토→수정→재검증 후 지정된 순서(RW-05→06→07→02→01→03→04→08→09→10)로 `main`에 병합 완료(로컬 브랜치는 병합 후 삭제, 원격 `main`에 push 완료). 각 항목의 구현 상세·codex 지적사항·수정 내역은 [`backlog.md`](backlog.md)의 "v0.1.1 — 리뷰 하드닝" 마일스톤에 기록했다. 남은 것은 RW-11(플랫폼별 GUI 수동 검증, 별도 진행 예정)과 RW-12(이 문서·backlog·README 동기화, 진행 중).

## 우선순위 정의

| 우선순위 | 의미 |
|---|---|
| P0 | 다음 릴리스 전에 반드시 해결. 보안, 데이터 손실, 복구 불능 문제 |
| P1 | MVP 사용자 흐름과 지원 플랫폼을 완료하기 위한 작업 |
| P2 | 운영·배포 품질과 제품 완성도를 높이는 후속 작업 |
| Deferred | 외부 API/플랫폼 제약 또는 비용 때문에 재개 조건까지 대기 |

상대 크기: `S`는 국소 변경, `M`은 여러 계층을 건드리는 변경, `L`은 설계·통합 테스트가 필요한 변경이다. 일정 추정치는 아니다.

## 권장 실행 순서

```text
RW-05 ──→ RW-10(기본 CI)

RW-01 ──→ RW-02 ───────────────┐
RW-03 ──────────────────────────┤
RW-04 ──→ RW-09 ───────────────┤
RW-06 ──→ RW-08 ───────────────┼─→ RW-10(회귀/E2E 보강) ─→ RW-11 ─→ RW-12
RW-07 ──────────────────────────┤
          RW-08, RW-09 ─────────┘
```

`RW-05`, `RW-06`, `RW-07`은 서로 독립적이라 먼저 병렬 처리할 수 있다. `RW-10`의 기본 정적 검사는 즉시 추가하고, 각 기능의 회귀 테스트는 해당 작업과 함께 누적한다.

## P0: v0.1.1 보안·안정화

### [x] RW-01 Sidecar 연결 계약과 로컬 API 신뢰 경계 확립 (`L`) — 완료 (2026-07-23, backlog 참고)

**근거:** 리뷰 R-01, R-05

**작업**

- Tauri가 시작마다 임의 세션 토큰을 생성해 sidecar 환경변수와 React client에 전달한다.
- Rust 알림 루프와 React client가 동일한 endpoint/token 상태를 사용하게 한다.
- `/v1/*`는 토큰을 필수 검증하고, `Host`와 bind 주소는 loopback으로 제한한다.
- JSON endpoint는 `application/json`만 허용하고 공격자 `Origin` 요청을 거부한다.
- 중복 앱 실행 또는 포트 선점 시 다른 프로세스를 정상 sidecar로 오인하지 않게 handshake를 추가한다.
- Tauri CSP를 활성화하고 사용하지 않는 plugin/capability 권한을 제거한다.
- 토큰은 webview에 빌드타임 상수로 굽지 않고 Tauri command로 런타임 전달한다(`main`·`quicksearch` 두 창 모두).
- `/healthz`는 부작용 없는 엔드포인트이므로 토큰 검사에서 제외한다.
- 토큰 미설정 시(dev) 인증을 우회하거나 기동 로그에 토큰을 출력하는 escape hatch를 둔다 — 이 프로젝트는 backend curl E2E 검증에 크게 의존하므로, 없으면 기존 검증 워크플로 전체가 깨진다.

**완료 조건**

- 정상 Tauri UI와 Rust 알림 폴링은 모두 동작한다.
- 무토큰, 잘못된 토큰, 공격자 `Origin`, `text/plain`, 비-loopback `Host` 요청은 거부된다.
- `NEULSANG_ADDR`를 유지한다면 endpoint가 모든 계층에 공유되고, 그렇지 않다면 설치 앱에서 지원하지 않는 override로 명확히 축소된다.
- 기존 재현 요청이 더 이상 `201 Created`를 반환하지 않는다.
- dev 모드에서 기존 curl 기반 검증 절차가 여전히 동작한다(토큰 없이 또는 로그에서 토큰을 얻어).

**참고(codex 교차검증, 2026-07-23):** 커스텀 인증 헤더는 브라우저의 프리플라이트를 강제하는 부수효과가 있고 desktopd는 프리플라이트에 응답하지 않으므로, 토큰은 인증과 동시에 단순요청 차단 역할도 겸한다. 다만 `0.0.0.0` bind 자체는 토큰이 막지 못하므로 loopback bind 강제(R-05)는 별도로 반드시 필요.

**검증**

- Go middleware/handler 통합 테스트
- Tauri sidecar handshake 단위 테스트
- `tauri dev`에서 health, capture, notification smoke test

### [x] RW-02 요청 크기·비용·동시성 제한 (`M`) — 완료 (2026-07-23, backlog 참고)

**근거:** 리뷰 R-01, R-08

**작업**

- `POST /v1/import`에 문서화된 최대 byte/row 수와 `413` 응답을 추가한다.
- capture 텍스트 길이 제한을 도메인 규칙으로 추가한다.
- 동시 explain 작업 수를 semaphore/queue로 제한하고 초과 요청 정책을 정한다.
- JSON decoder가 단일 값 뒤의 trailing payload를 거부하도록 공통 helper를 도입한다.
- Gemini·sync 응답 본문에도 합리적인 최대 크기를 둔다.

**완료 조건**

- 제한을 넘는 import/capture가 DB나 AI 호출을 만들지 않는다.
- 정상 크기의 백업과 검색 흐름은 회귀 없이 동작한다.
- 제한 값과 사용자 오류 메시지가 개발 문서/API 계약에 기록된다.

### [x] RW-03 Graceful sidecar 종료와 중단 작업 복구 (`L`) — 완료 (2026-07-23, backlog 참고, Windows Job Object는 미해결로 남김)

**근거:** 리뷰 R-03, 기존 Windows watchdog 한계

**작업**

- Tauri Quit이 먼저 graceful shutdown을 요청하고 제한 시간 뒤에만 강제 종료한다.
- Go shutdown은 HTTP 접수 중단, explain/outbox/scheduler 취소, 진행 작업 상태 기록, DB close 순서로 끝난다.
- 시작 시 남은 `queued`/`running` lookup을 재큐잉할지 `failed`로 전환할지 정책을 확정·구현한다.
- Windows는 Job Object 또는 동등한 kill-on-parent-close 메커니즘을 사용한다.
- 중복 앱 실행 시 sidecar 소유권과 종료 책임을 명확히 한다.

**완료 조건**

- 느린 explainer 실행 중 Quit→재시작 후 영구 `running` 행이 없다.
- 정상 종료는 timeout 안에 끝나며, timeout 때만 kill fallback이 실행된다.
- macOS와 Windows 비정상 부모 종료에서 고아 sidecar가 남지 않는다.

### [x] RW-04 JSON backup v2와 기능 단위 복원 (`L`) — 완료 (2026-07-23, backlog 참고)

**근거:** 리뷰 R-02

**작업**

- snapshot v2 계약에 `lookup_jobs`, `review_card_candidates`를 포함하거나 import 시 손실 없이 재구성한다.
- 실패 상태, explanation 조회 가능성, 미소비 카드 후보를 보존한다.
- 지원하지 않는 미래 version은 명확히 거부하고 v1→v2 호환 정책을 구현한다.
- import 전 참조 무결성과 enum/time 값을 검증한다.

**완료 조건**

- 빈 DB 복원 후 기존 explanation API가 `done` 결과를 반환한다.
- 실패 capture는 Inbox `failed` 상태를 유지한다.
- 복원 후 아직 소비하지 않은 knowledge item의 “모름”이 카드를 생성한다.
- 동일 snapshot 재-import가 멱등이고 기존 라이브 SRS 상태를 훼손하지 않는다.

**검증**

- API 기반 export→새 DB import→explanation/Inbox/mark-unknown/review E2E
- v1 fixture 호환 테스트와 미래 version 거부 테스트

### [x] RW-05 Go toolchain 보안 패치 (`S`) — 완료 (2026-07-23, backlog 참고)

**근거:** 리뷰 R-09, `GO-2026-5856`

**작업**

- `go.work` toolchain을 `go1.26.5` 이상 패치 버전으로 올린다.
- 로컬 가이드와 CI Go 버전을 같은 최소 버전으로 맞춘다.

**완료 조건**

- `go test -race ./...`, `go vet ./...`, `golangci-lint run ./...` 통과
- `govulncheck ./...`에서 호출 가능한 취약점 0건

## P1: MVP 사용자 흐름 완성

### [x] RW-06 AI provider 결정 단일화 (`S`) — 완료 (2026-07-23, backlog 참고)

**근거:** 리뷰 R-06

**작업**

- provider 해석을 한 함수에서 수행해 explainer, suggester, Settings effective 표시가 공유한다.
- `NEULSANG_AI_PROVIDER=mock`이면 API key가 있어도 suggest가 Gemini를 호출하지 않게 한다.
- 알 수 없는 provider는 시작 오류로 막거나 mock fallback을 일관되게 표시한다.

**완료 조건**

- mock/gemini/자동 선택/키 없음/알 수 없는 값 조합 테스트가 모두 통과한다.
- Settings에 표시된 provider와 실제 외부 호출 여부가 일치한다.

### [x] RW-07 Dashboard 로컬 날짜 경계 수정 (`S`) — 완료 (2026-07-23, backlog 참고)

**근거:** 리뷰 R-07

**작업**

- 사용자 로컬 시간대에서 오늘 자정을 계산한 뒤 UTC instant로 DB에 질의한다.
- “이번 주”가 rolling 7일인지 달력 주인지 제품 용어를 확정해 UI와 API를 맞춘다.

**완료 조건**

- Asia/Seoul 기준 자정에 “오늘” 수치가 초기화된다.
- UTC가 아닌 location과 DST 전환 경계 테스트가 통과한다.

### [x] RW-08 한글 발음 후보 선택 UI 연결 (`M`) — 완료 (2026-07-23, backlog 참고)

**근거:** 리뷰 R-04, backlog #21의 미충족 사용자 흐름

**의존:** RW-01, RW-06

**작업**

- React API client에 suggest/confirm 계약과 타입을 추가한다.
- Quick Search가 한글 발음 입력을 감지해 후보, confidence, 출처, 짧은 뜻을 표시한다.
- 후보 선택 시 confirm cache를 기록하고 선택한 영어 표현으로 capture/explain을 시작한다.
- 후보가 없거나 suggest가 실패하면 원문 검색 또는 직접 입력으로 복구할 수 있게 한다.

**완료 조건**

- `스테일`→후보 표시→`stale` 선택→해석 저장 흐름이 UI에서 끝까지 동작한다.
- cache hit, local fallback, Gemini, 빈 결과, 취소 상태의 컴포넌트 테스트가 있다.

### [x] RW-09 Settings 백업·복원 UI (`M`) — 완료 (2026-07-23, backlog 참고)

**근거:** 리뷰 R-04, backlog #19 후속

**의존:** RW-01, RW-04

**작업**

- Tauri file dialog로 JSON export/import와 SQLite backup 저장 위치를 선택한다.
- import 전에 파일 version/요약을 보여주고 사용자 확인을 받는다.
- 성공 결과의 inserted/merged/updated/skipped를 표시한다.
- 실패·부분 적용처럼 오해할 수 있는 상태를 명확히 표현한다. import는 계속 단일 transaction이어야 한다.

**완료 조건**

- 사용자가 curl 없이 Settings에서 export, 새 DB restore, SQLite file backup을 수행할 수 있다.
- 취소, 잘못된 파일, 지원하지 않는 version, 대용량 거부, 성공 결과 UI 테스트가 있다.

### [x] RW-10 자동화 테스트와 PR 품질 게이트 (`L`) — 완료 (2026-07-23, backlog 참고. Linux 타깃은 미결정으로 남김)

**근거:** 리뷰 R-10

**작업**

- PR용 `.github/workflows/quality.yml`을 추가한다.
- Go race/vet/lint/vuln, npm build/audit/test, Rust fmt/clippy/test를 강제한다.
- React에 Vitest + Testing Library 기반 핵심 상태 전이 테스트를 추가한다.
- sidecar lifecycle, 인증 handshake, backup restore를 포함한 E2E smoke를 추가한다.
- release workflow가 quality 성공을 전제로 하도록 연결한다.

**완료 조건**

- PR에서 필수 gate가 실패하면 merge/release가 차단된다.
- R-01~R-09의 재현 케이스가 자동화 테스트에 포함된다.
- CI와 로컬 명령이 문서에서 동일하다.

### [~] RW-11 지원 플랫폼 릴리스 검증 (`L`) — 착수 (2026-07-23)

**근거:** 리뷰 R-04, backlog #29/#32 후속

**의존:** RW-01~RW-10

**Linux 범위 결정(2026-07-23, ADR-0009)**: 검증 가능한 실기기가 없어 **Linux는 이번 릴리스 범위에서 제외, 후속(미정)으로 defer**. PRD §5.2 각주로 반영. 이 항목은 macOS(arm64/x86_64) + Windows 11만 검증 대상으로 축소한다.

**작업**

- macOS arm64/x86_64에서 설치, sidecar, 단축키, 알림 클릭 이동, 백업 복원을 확인한다.
- Windows 11에서 sidecar 수명주기, 트레이, 단축키, Credential Manager, 알림을 확인한다.
- ~~PRD의 Linux 지원을 유지한다면 CI에 deb/AppImage와 네이티브 smoke를 추가한다. 아니면 PRD 지원 범위를 명시적으로 축소한다.~~ → **완료(ADR-0009, 축소 결정)**
- backlog #29의 “GUI 수동확인 대기” 상태를 실제 결과로 닫는다.

**진행 상황(2026-07-23)**: 상세 체크리스트와 자동화 검증 결과는 [`../rw-11-platform-verification.md`](../rw-11-platform-verification.md) 참고. macOS arm64에서 Claude가 CLI로 자동 검증한 항목(번들 서명·사이드카 spawn·config 주입·토큰 인증 401/200·비정상 종료 시 watchdog 안전망)은 완료. **실제 GUI 클릭이 필요한 항목(트레이 메뉴, 글로벌 단축키 팝업, OS 알림 배너, 백업 다이얼로그)과 Windows 11 전체(사람이 접근 가능한 실기기 없음)는 아직 사람 확인 대기.** 이번 하드닝(RW-01~RW-10)은 `v0.1.0` 태그 이후 커밋이라 기존 릴리스 자산에는 반영 안 됨 — Windows/Intel Mac에서 하드닝까지 검증하려면 새 태그 릴리스가 필요(사용자 승인 필요, 공개 릴리스 행위).

**완료 조건**

- 지원한다고 문서화한 모든 OS/arch에 설치 가능한 산출물과 smoke 기록이 있다.
- 플랫폼별 알려진 제한과 우회 방법이 릴리스 노트에 포함된다.

### [x] RW-12 상태 문서와 완료 정의 정리 (`S`) — 완료 (2026-07-23)

**근거:** 리뷰 R-11

**의존:** 해당 코드 작업 완료 후

**작업**

- 루트 README의 현재 상태와 버전을 갱신한다.
- Settings의 “알림은 이후 업데이트” 문구를 제거한다.
- `NEULSANG_ADDR`, provider, backup, 플랫폼 지원 문서를 실제 동작과 맞춘다.
- backlog에서 “backend 완료”와 “사용자 흐름 완료”를 구분한다.

**완료 조건**

- README, PRD, development guide, backlog, 앱 내 문구가 서로 모순되지 않는다.
- 완료 표시된 수용 기준마다 자동 또는 수동 검증 근거가 연결된다.

## P2: 제품·배포 후속

### [ ] RW-13 제품 UX 부채 정리 (`L`, 분할 필요)

- Result Detail 전체 화면과 바로 “모름/알아요” 처리
- archived 항목 복원, 상태 변경 후 Inbox 즉시 반영
- Dashboard 최근 7일 시계열
- 단축키 런타임 변경과 재등록
- API key 키체인 저장/삭제 UI
- 알림 목록 실시간 갱신
- FTS5를 사용하는 검색/필터

각 항목은 착수 전에 독립 이슈로 분리하고 사용자 가치와 데이터/API 변경 여부로 우선순위를 다시 정한다.

### [ ] RW-14 배포·공급망 하드닝 (`L`, 계정/인증서 의존)

- macOS Developer ID 서명과 notarization
- Windows 코드서명
- GitHub Actions를 commit SHA로 고정
- release 재실행 시 기존 draft/tag를 안전하게 재사용
- SBOM/라이선스 목록과 Rust dependency audit 추가
- 필요 시 Intel+Apple Silicon universal app 제공

### [ ] RW-15 중앙 동기화 확장 (`XL`, 별도 마일스톤)

- `apps/api` 중앙 수신 서버와 서버측 `event_id` 멱등성
- outbox 영구 4xx quarantine/dead-letter와 사용자 상태 표시
- 계정·기기 등록·충돌 정책·교차기기 병합
- 모바일 복습과 원격 백업

이는 PRD 장기 목표이며 v0.1.x 릴리스 차단 작업이 아니다.

## Deferred

### [보류] RW-D01 배달된 OS 알림 회수와 정확한 배너별 딥링크

기존 backlog #23을 유지한다. 현재 Tauri notification desktop API가 배달 알림 제거와 클릭 callback을 제공하지 않는다. 플러그인 지원이 추가되거나 macOS 발화를 `UNUserNotificationCenter` 기반으로 교체할 때 재개한다.

### [보류] RW-D02 앱 완전 종료 상태의 OS 예약 알림

현재 알림 scheduler는 sidecar와 함께 종료된다. 앱이 꺼져도 알림을 보내려면 OS별 background scheduling 설계가 필요하다. MVP 범위 밖으로 유지한다.

### [보류] RW-D03 polling을 SSE/long-poll로 교체

현재 7초 polling은 1인 로컬 앱 규모에서 충분하다. 전력·지연 측정으로 문제가 확인될 때만 재개한다.

## 마일스톤 종료 조건

### v0.1.1 Hardening

- RW-01~RW-07 완료
- RW-10의 기본 CI gate와 R-01~R-07 회귀 테스트 활성화
- `govulncheck` 호출 가능 취약점 0건
- backup/restore와 slow-explain shutdown E2E 통과

### Usable MVP

- v0.1.1 조건 충족
- RW-08, RW-09 완료
- RW-10 전체 E2E, RW-11 지원 플랫폼 검증 완료
- RW-12 문서 동기화 완료

### Public Distribution

- Usable MVP 조건 충족
- RW-14의 대상 플랫폼 서명·공증·공급망 항목 완료
- 공개 설치·업데이트·롤백 절차 검증 완료
