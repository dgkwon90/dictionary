# 개발 사이클 규칙

모든 작업은 이 사이클을 반복한다. 큰 것은 반드시 작게 쪼개서 사이클에 태운다.

```
이슈(작은 단위) → 설계 → (필요시 codex/agy 리뷰) → 테스트 작성(TDD) → 구현
→ lint·취약점 검사 → 테스트 통과 → 셀프 리뷰 → 확정(커밋)
```

**작업 종류별 적용** (1인 개발, 사람 리뷰어 없음):
- 제품 코드: 사이클 전체 적용
- 문서·ADR·설정 조사: TDD 단계 면제 (검증 가능한 완료 기준은 이슈에 명시)
- 되돌리기 어려운 결정(ADR급)만 `/tri-review`로 codex/agy 교차 검토, 일상 코드는 셀프 리뷰로 충분

## 작게 쪼개기
- 이슈 1개 = 1~2일 크기. 그보다 크면 쪼갠다
- 커밋 1개 = 리뷰 시 5분 안에 읽을 크기 (생성물·lockfile은 계산 제외)
- "일단 크게 만들고 나중에 정리" 금지 — 작은 단위로 완결시키며 쌓는다

## TDD (테스트 먼저)
- 제품 코드는 실패하는 테스트부터. "이 기능이 됐다"를 증명하는 테스트 없이 확정 금지
- Go: 단위 테스트는 코드 옆(`_test.go`), 교차 컴포넌트는 결과적으로 `apps/desktopd` 통합 테스트로
- TS/React: 컴포넌트 단위 테스트는 코드 옆, E2E는 후순위(Tauri 앱 안정화 이후)
- 버그 수정은 재현 테스트와 함께

## 품질 게이트 (확정 전 필수)
| 게이트 | Go (`apps/desktopd`) | TS/React (`apps/desktop-ui`) |
|---|---|---|
| 포맷 | gofumpt v0.10.0 | prettier |
| lint | golangci-lint v2.11.0 (설정: 루트 `.golangci.yml`) | eslint |
| 빌드·테스트 | `go build` + `go test -race` | 빌드 + vitest |
| 취약점 | govulncheck v1.5.0 | `npm audit` — production 의존성의 high 이상만 차단 |

Go 도구 버전은 #1에서 고정(2026-07-07): Go 1.26 (`go.work`에 `toolchain go1.26.4` 고정 — stdlib 취약점 GO-2026-5039 등 해소 버전). 실행은 `apps/desktopd/Makefile`의 `make all`(fmt→lint→test→vuln→build). TS/React 도구 버전은 #13에서 고정하고 이 표를 갱신한다 (최신 안정 버전 + 공식 문서 기준).

## 디렉토리 경계 강제
- `apps/desktopd/internal/domain/`은 infra(AI provider, clipboard, notifier, scheduler 등)를 직접 import하지 않는다 — interface로 주입하고 `internal/infra/`가 구현한다 (PRD §18.1, §14.3)
- `apps/desktopd` ↔ `apps/desktop-ui`: 직접 import 금지, **로컬 HTTP API(`docs/prd.md` §15)로만 통신**. UI가 새 데이터를 필요로 하면 API 엔드포인트를 먼저 추가한다
- `apps/desktopd/internal/`은 Go의 `internal/` 가시성으로 외부 모듈에서 컴파일 수준 차단됨 (다른 apps가 생기면 유효해짐)
- 경계를 넘고 싶다 = API 계약 누락 신호 — 직접 참조 대신 API 엔드포인트를 추가한다

## 데드코드·코드 냄새 방지
- 사용하지 않는 코드 커밋 금지 ("나중에 쓸" 코드 포함 — 필요하면 git 이력에서 되살린다)
- `TODO`는 백로그 이슈 번호 필수 (`TODO(#12):`)
- 주석 처리된 코드 블록 커밋 금지
- 중복 3회부터 추상화 (성급한 추상화도 냄새다)

## 용어 규칙
- 도메인 용어(테이블명, 상태값, JSON 필드명)는 등장한 커밋에서 `docs/glossary.md` 등재 필수
- 같은 개념에 두 단어 금지 — glossary가 표준어 결정

## 기술 선택 규칙 (OSS·프레임워크·모듈)
1. 라이선스: Apache-2.0 / MIT / BSD / MPL-2.0 우선. AGPL·BSL·SSPL은 상용화 옵션을 막으므로 법적 검토 없이 도입 금지
2. 최신 안정 버전 + 공식 최신 문서 기준 (구버전 API 답습 금지)
3. **되돌리기 어려운 선택만 ADR** (UI 프레임워크, DB, AI provider 1차 선택, 동기화 프로토콜). 포맷터·테스트 러너 등은 위 품질 게이트 표 갱신으로 충분
4. 의존성 최소 — 표준 라이브러리로 충분하면 추가하지 않는다
