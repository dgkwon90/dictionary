# 문서 지도

| 문서 | 용도 |
|---|---|
| [`prd.md`](prd.md) | 제품 정의 원본 — 문제 정의, MVP 범위, DB 스키마, API, 화면 설계, 복습 알고리즘. **가장 상세하고 기준이 되는 문서**, 충돌 시 우선 |
| [`development.md`](development.md) | 개발·빌드·실행 가이드 — 로컬 실행(백엔드 curl / `tauri dev` / 번들 `.app`), 환경변수, config 주입, 검증 게이트, 함정 |
| [`glossary.md`](glossary.md) | 용어집. 새 도메인 용어는 여기 등재 |
| [`adr/`](adr/README.md) | 되돌리기 어려운 결정 기록 (구조, 스택, provider 등) |
| [`rules/`](rules/) | 개발 사이클, AI 협업, GitHub 워크플로우 규칙 |
| [`planning/backlog.md`](planning/backlog.md) | 작업 대장 (이슈 단위, 의존 그래프, 마일스톤) |
| [`planning/remaining-work.md`](planning/remaining-work.md) | 현재 남은 작업 — 우선순위, 의존 관계, 완료 조건, 릴리스 게이트 |
| [`rw-11-platform-verification.md`](rw-11-platform-verification.md) | RW-11 지원 플랫폼 검증 — 자동화 확인 결과 + 사람이 해야 할 GUI 체크리스트 |
| [`reviews/`](reviews/) | `/tri-review` 실행 결과 요약 |

## 읽는 순서 (신규 세션/에이전트용)
1. `CLAUDE.md` (루트) — 얇은 진입점
2. `docs/prd.md` — 제품이 무엇인지
3. `docs/adr/` — 왜 이렇게 만들기로 했는지 (특히 "제안" 상태 ADR은 아직 미확정이니 주의)
4. `docs/planning/remaining-work.md` — 지금 뭘 해야 하는지
5. `docs/planning/backlog.md` — 완료 이력과 상세 구현 기록
