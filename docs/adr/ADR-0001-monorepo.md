# ADR-0001: 모노레포 구조

- 날짜: 2026-07-03 / 상태: 승인

## 맥락
PRD §14.1이 `apps/{desktop-ui,desktopd,api}` 형태의 단일 저장소 구조를 이미 전제하고 있다. 1인 개발이고, UI(Tauri/TS)와 백엔드(Go sidecar)가 매 기능마다 함께 바뀌는 빈도가 높아 별도 저장소로 쪼갤 이유가 약하다.

## 결정
단일 monorepo. `apps/desktopd`(Go), `apps/desktop-ui`(Tauri+TS), `apps/api`(추후 중앙 서버), `deploy/`, `scripts/`, `docs/`를 한 저장소에 둔다.

## 근거
- 1인 개발 + 두 컴포넌트가 로컬 HTTP API 계약으로 강하게 결합 — 별도 repo는 버전 동기화 오버헤드만 늘림
- 중앙 서버(`apps/api`)는 v0.5 이후에나 착수 (PRD §16) — 그 전까지는 폴더만 예약

## 결과·트레이드오프
- 장점: 계약(API) 변경이 단일 커밋으로 가능, 브랜치 전환 없이 전체 컨텍스트 확인 가능
- 트레이드오프: `apps/desktopd` ↔ `apps/desktop-ui` 간 직접 import를 막는 규율이 필요 (`docs/rules/development-cycle.md` 디렉토리 경계 절 참조) — 강제 도구(go의 `internal/`)는 desktopd 내부에만 유효하고, UI 쪽은 리뷰로 지켜야 함
