# v0.1.0 프로젝트 리뷰

- 날짜: 2026-07-22
- 대상: `fad4fe3` (`main`, `v0.1.0`) 기준 전체 프로젝트
- 범위: Go sidecar, SQLite 데이터 모델, 로컬 HTTP API, React UI, Tauri 셸, 릴리스 워크플로, 문서
- 판정: 핵심 학습 흐름과 자동화 테스트의 기반은 양호하지만, 로컬 API 신뢰 경계와 백업 복원 의미 보존 문제 때문에 현재 상태를 그대로 배포 범위 확대하기에는 위험하다.

## 요약

| ID | 심각도 | 문제 | 권장 우선순위 |
|---|---|---|---|
| R-01 | High | 인증 없는 로컬 API가 교차 출처 `text/plain` POST를 수락 | 즉시 |
| R-02 | High | JSON 복원이 테이블 라운드트립은 통과하지만 제품 동작을 복원하지 못함 | 즉시 |
| R-03 | High | 앱 종료 시 sidecar 강제 종료로 해석 작업이 `running`에 고착될 수 있음 | 즉시 |
| R-04 | High | 완료 처리된 필수 기능 2개가 UI 사용자 흐름에 연결되지 않음 | 출시 전 |
| R-05 | Medium | `NEULSANG_ADDR` override가 UI/Rust와 공유되지 않음 | 출시 전 |
| R-06 | Medium | `NEULSANG_AI_PROVIDER=mock`이어도 suggest는 Gemini를 호출할 수 있음 | 출시 전 |
| R-07 | Medium | Dashboard의 “오늘”이 사용자 로컬 날짜가 아닌 UTC 날짜 기준 | 다음 수정 |
| R-08 | Medium | JSON import 요청 크기가 무제한 | R-01과 함께 |
| R-09 | Medium | 고정 Go toolchain에 도달 가능한 표준 라이브러리 취약점이 탐지됨 | 즉시 |
| R-10 | Medium | PR/릴리스 전에 테스트·린트·취약점 검사를 강제하는 CI가 없음 | 출시 전 |
| R-11 | Low | README/UI 안내와 현재 구현·릴리스 상태가 어긋남 | 다음 문서 수정 |

## 확인된 문제

### R-01. 로컬 HTTP API에 요청 출처를 검증하는 경계가 없다

**근거**

- 라우터는 인증 토큰, `Host`/`Origin` 검사, JSON `Content-Type` 강제를 거치지 않고 handler를 직접 등록한다: [`router.go`](../../apps/desktopd/internal/transport/http/router.go).
- 캡처 handler는 본문 크기만 제한하고 콘텐츠 타입과 출처를 확인하지 않는다: [`capture.go`](../../apps/desktopd/internal/transport/http/handlers/capture.go#L26).
- 재현 결과, 아래 형태의 요청이 `201 Created`로 처리됐다.

```http
POST /v1/captures
Origin: https://attacker.example
Content-Type: text/plain

{"text":"cross-origin-write","input_mode":"manual"}
```

브라우저의 CORS는 응답 읽기를 제한할 뿐 단순 요청의 전송 자체를 항상 막는 방어선이 아니다. 현재 요청은 캡처 저장, AI 작업 생성, outbox 증가를 유발한다. DNS rebinding까지 고려하면 export 조회와 다른 API 조작도 신뢰할 수 없다. `NEULSANG_ADDR`에 비-loopback 주소를 넣는 것도 현재 허용된다.

**권장 수정**

1. Tauri가 sidecar를 시작할 때 세션별 임의 토큰을 주입하고 모든 `/v1/*` 요청에서 검증한다.
2. `Host`를 실제 loopback 주소로 제한하고 `NEULSANG_ADDR`도 loopback bind만 허용한다.
3. JSON body endpoint는 `application/json`만 허용한다. 이것만으로 인증을 대체하지는 않는다.
4. 캡처 텍스트 길이, 동시 explain 작업 수, 요청 빈도에 별도 상한을 둔다.
5. 공격자 `Origin`, `text/plain`, 잘못된 토큰, DNS-rebinding 형태의 `Host`를 거부하는 통합 테스트를 추가한다.

### R-02. JSON backup이 기능적으로 완전한 복원을 제공하지 않는다

**근거**

- export/import 대상은 7개 테이블뿐이다: [`backup_repo.go`](../../apps/desktopd/internal/db/sqlite/backup_repo.go#L24), [`backup.go`](../../apps/desktopd/internal/domain/backup/backup.go#L9).
- `lookup_jobs`는 제외되지만 explanation 조회는 먼저 이 테이블의 최신 상태를 요구하며, 행이 없으면 capture 자체가 없는 것으로 반환한다: [`explain_repo.go`](../../apps/desktopd/internal/db/sqlite/explain_repo.go#L218).
- `review_card_candidates`도 제외된다. 아직 카드를 만들지 않은 knowledge item을 복원한 뒤 “모름”으로 표시해도 생성할 후보가 없다.
- 실패한 lookup의 상태도 제외되므로 복원 후 Inbox에서 `failed`가 `new`로 바뀔 수 있다.
- 현재 round-trip 테스트는 다시 export한 7개 테이블의 JSON 동등성만 확인하므로 위 동작 회귀를 잡지 못한다.

**영향**

- 복원된 설명이 DB에 존재해도 `GET /v1/captures/{id}/explanation`은 404가 된다.
- 백업 당시 아직 학습 카드로 전환하지 않은 항목은 이후 카드 생성 능력을 잃는다.
- 사용자는 성공/실패 상태가 보존된다고 기대하지만 Inbox 상태가 달라진다.

**권장 수정**

`lookup_jobs`와 `review_card_candidates`를 스냅샷에 포함하는 방안이 가장 단순하다. 운영 테이블을 계속 제외하려면 import 시 explanation마다 `done` lookup을 합성하고, 보존된 `terms_json`(또는 `raw_response_json`)에서 후보를 검증·재구성하는 명시적 마이그레이션이 필요하다. 테스트는 테이블 비교가 아니라 복원 후 explanation 조회, Inbox 상태, “모름→카드 생성”까지 검증해야 한다.

### R-03. 정상 종료가 비동기 해석을 강제 중단한다

**근거**

- capture 생성은 DB 상태를 `running`으로 바꾼 뒤 별도 goroutine에서 AI 호출을 수행한다: [`service.go`](../../apps/desktopd/internal/domain/explain/service.go#L35), [`bootstrap.go`](../../apps/desktopd/internal/app/bootstrap/bootstrap.go#L217).
- Tauri 종료 경로는 자식에게 graceful signal을 보내지 않고 `Child::kill()` 후 `wait()`한다: [`sidecar.rs`](../../apps/desktop-ui/src-tauri/src/sidecar.rs#L48).
- 시작 시 남아 있는 `queued`/`running` 작업을 실패 처리하거나 재시도하는 복구 로직이 없다.

**영향**

Gemini 응답을 기다리는 동안 사용자가 Quit하면 해당 capture의 job은 영구적으로 `running`에 남을 수 있다. 재시작 후 Quick Search 폴링은 그 작업을 완료하거나 실패로 전환할 수 없다.

**권장 수정**

Unix에서는 `SIGTERM` 후 제한 시간만큼 기다리고, Windows에서는 명시적인 shutdown IPC 또는 Job Object 기반 수명주기를 사용한 뒤 최후에만 kill한다. 별개로 시작 시 오래된 `queued`/`running` 작업을 `failed`로 전환하거나 안전하게 재큐잉해야 한다. “느린 explainer 실행 중 앱 종료→재시작” 통합 테스트가 필요하다.

### R-04. 필수 기능이 backend에만 있고 사용자 흐름에는 없다

**한글 발음 후보**

백로그 #21의 수용 기준은 후보를 보여주고 사용자가 선택하면 explain 파이프라인으로 진입하는 것이다. 그러나 API client에는 suggest 메서드/타입이 없고, Quick Search는 모든 입력을 즉시 `createCapture`로 보낸다: [`client.ts`](../../apps/desktop-ui/src/api/client.ts#L61), [`QuickSearch.tsx`](../../apps/desktop-ui/src/quicksearch/QuickSearch.tsx#L91).

**백업/복원**

PRD의 MVP 완료 기준에는 로컬 DB 내보내기가 포함되지만 Settings에는 export/import/SQLite backup 조작이 없다: [`Settings.tsx`](../../apps/desktop-ui/src/settings/Settings.tsx#L67). 현재 기능은 curl로만 접근 가능하다.

**플랫폼 범위**

PRD는 Linux 실행을 필수로 적었고 sidecar 빌더도 Linux triple을 알지만, 릴리스 매트릭스는 macOS 2종과 Windows만 게시한다: [`.github/workflows/release.yml`](../../.github/workflows/release.yml#L41).

완료 상태를 “backend API 완료”와 “사용자 흐름 완료”로 분리하거나, UI 연결과 Linux 산출물 검증 전에는 해당 MVP 항목을 완료로 표시하지 않는 편이 정확하다.

### R-05. 주소 override가 통합 앱을 분리한다

Go는 `NEULSANG_ADDR`를 지원하고 문서에도 공개 설정으로 적혀 있지만, React client와 Rust 알림 루프는 각각 `127.0.0.1:48989`를 상수로 사용한다: [`config.go`](../../apps/desktopd/internal/config/config.go#L11), [`client.ts`](../../apps/desktop-ui/src/api/client.ts#L12), [`notifications.rs`](../../apps/desktop-ui/src-tauri/src/notifications.rs#L19). Tauri HTTP capability도 같은 주소만 허용한다.

따라서 주소를 변경하면 sidecar는 새 포트에서 정상 실행하지만 UI는 이전 포트를 계속 조회한다. 설정을 지원하려면 Tauri가 선택한 주소를 UI와 알림 모듈에 단일 소스로 전달해야 한다. 그렇지 않으면 설치 앱에서는 override를 제거하고 backend 단독 개발 옵션으로만 문서화해야 한다.

### R-06. mock provider가 suggest의 Gemini 호출을 막지 않는다

`newExplainer`는 `AIProvider`를 반영하지만 `newSuggester`는 API key 존재 여부만 확인한다: [`bootstrap.go`](../../apps/desktopd/internal/app/bootstrap/bootstrap.go#L246). 문서의 “키가 있어도 `NEULSANG_AI_PROVIDER=mock`이면 mock 강제”와 다르며, suggest API 호출 시 외부 전송과 비용이 발생할 수 있다.

provider 결정을 하나의 함수에서 확정하고 explainer, suggester, Settings의 effective 표시가 모두 같은 결과를 사용해야 한다. 알 수 없는 provider도 현재 Settings에는 그대로 노출되지만 explainer는 mock으로 fallback하므로 함께 교정해야 한다.

### R-07. “오늘” 통계가 UTC 자정에 바뀐다

Stats service는 현재 시간을 즉시 UTC로 바꾸고 UTC 00:00을 `TodayStart`로 사용한다: [`stats/service.go`](../../apps/desktopd/internal/domain/stats/service.go#L20). 한국 표준시에서는 오전 09:00에 “오늘 검색/복습” 수치가 초기화되고, 자정부터 오전 9시까지 전날 데이터가 섞인다.

로컬 시간대에서 당일 자정을 만든 뒤 UTC instant로 변환해 DB와 비교해야 한다. UTC가 아닌 location과 DST 경계를 포함한 테스트를 추가한다.

### R-08. JSON import 본문 크기가 제한되지 않는다

다른 JSON handler와 달리 `POST /v1/import`는 `http.MaxBytesReader`를 적용하지 않은 채 배열 전체를 decode한다: [`backup.go`](../../apps/desktopd/internal/transport/http/handlers/backup.go#L38). 인증 부재와 결합하면 메모리·DB 작업량을 원격 웹 콘텐츠가 증폭할 수 있고, 정상 사용에서도 비정상적으로 큰 파일이 프로세스를 압박한다.

명시적인 최대 스냅샷 크기를 정해 413으로 거부하고, row 수 상한과 import 전 구조 검증도 추가해야 한다. 큰 백업이 실제 요구라면 임시 파일 기반 streaming/2-pass 검증으로 설계를 바꿔야 한다.

### R-09. Go patch release 취약점이 남아 있다

`go.work`가 `go1.26.4`를 고정한다. `govulncheck ./...`는 표준 라이브러리 `crypto/tls`의 `GO-2026-5856`을 탐지했고 수정 버전으로 `go1.26.5`를 제시했다. 이 이슈는 Encrypted Client Hello 사용 조건과 관련되어 현재 코드의 기본 HTTP 구성에서 실제 노출 가능성은 제한적이지만, 릴리스 도구 체인을 패치 버전으로 올리는 비용이 낮다.

`go.work` toolchain과 CI Go 버전을 1.26.5 이상으로 맞춘 뒤 `go test -race ./...`, `govulncheck ./...`를 다시 실행한다.

### R-10. CI가 품질 게이트를 강제하지 않는다

현재 GitHub Actions는 태그 릴리스 빌드만 수행하고 Go test/race/vet/lint/vulncheck, TypeScript test, Rust clippy/audit를 실행하지 않는다. 특히 React와 Rust 셸에는 자동화된 동작 테스트가 없어서 R-03~R-05 같은 통합 회귀가 릴리스 시점까지 남을 수 있다.

PR용 `quality.yml`을 추가하고 최소한 다음을 강제한다.

- Go: `go test -race ./...`, `go vet ./...`, `golangci-lint run ./...`, `govulncheck ./...`
- UI: `npm ci`, `npm run build`, 컴포넌트/상태 전이 테스트
- Rust: `cargo fmt --check`, `cargo clippy --all-targets -- -D warnings`, 단위 테스트
- E2E: sidecar 시작→capture→explain→mark unknown→review→backup/restore 핵심 흐름

### R-11. 사용자 안내가 구현 상태와 다르다

- 루트 README는 현재 상태를 2026-07-07의 백로그 #1·#2 완료로 설명하지만 실제 HEAD는 v0.1.0이다: [`README.md`](../../README.md#L5).
- Settings는 알림이 이미 구현됐는데도 “이후 업데이트에서 동작”한다고 표시한다: [`Settings.tsx`](../../apps/desktop-ui/src/settings/Settings.tsx#L106).
- 개발 문서는 `NEULSANG_ADDR`를 통합 앱 설정처럼 안내하지만 실제로는 R-05 제약이 있다.

코드 수정과 함께 README의 현재 상태, 설정 안내, 백로그의 완료 정의를 갱신해야 한다.

## 검증 결과

| 명령 | 결과 |
|---|---|
| `go test -race ./...` | 통과 |
| `go vet ./...` | 통과 |
| `golangci-lint run ./...` | 통과, 0 issues |
| `govulncheck ./...` | 실패, `GO-2026-5856` 1건 |
| `npm run build` | 통과 |
| `npm audit --omit=dev` | 통과, 0 vulnerabilities |
| `cargo clippy --all-targets -- -D warnings` | 통과 |
| `cargo audit` | 미실행, 로컬에 subcommand 미설치 |

GUI 수동 테스트와 실제 macOS/Windows/Linux 패키징은 이번 리뷰에서 수행하지 않았다.

## 권장 수정 순서

1. R-01, R-08, R-09: 로컬 API 신뢰 경계와 입력 제한을 먼저 닫고 toolchain을 패치한다.
2. R-03: graceful shutdown과 stale job 복구를 함께 구현한다.
3. R-02: JSON 스냅샷 계약을 고치고 기능 단위 restore E2E를 추가한다.
4. R-04, R-05, R-06: 누락된 사용자 흐름과 config 단일 소스를 완성한다.
5. R-07, R-10, R-11: 시간대 정확성, CI, 문서를 정리한다.

## 이번 리뷰에서 적용한 변경

- 이 리뷰 문서를 추가했다.
- `docs/reviews/README.md`에 리뷰 인덱스를 추가했다.
- 제품 코드는 변경하지 않았다.
