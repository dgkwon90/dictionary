# Neulsang(늘상) — 로컬 우선 개발자 영어 학습 앱

## 프로젝트 개요
업무 중 마주친 영어 단어·용어·문장을 단축키로 즉시 검색·AI 해석하고, 그 기록을 복습 카드로 전환하는 로컬 우선(local-first) 데스크톱 앱.
**제품명은 Neulsang(한글 "늘상")으로 확정** (`docs/adr/ADR-0006-product-name.md`). 영문은 `Neulsang`, 한글 병기는 "늘상". 컴포넌트·바이너리명(`desktopd` 등)은 제품명과 별개다.

**기준 문서: `docs/prd.md`** (제품 정의, DB 스키마, API, 화면 설계, MVP 범위 — 이미 상세). 충돌 시 그 문서가 우선.
문서 지도: `docs/README.md` / 용어: `docs/glossary.md` / 결정 기록: `docs/adr/`

## 개발 전략 (핵심 원칙)
"작게 설계 → 구현 → 검증 → 확장" 반복. `docs/planning/backlog.md`의 마일스톤(v0.1 ~ v0.5) 순서를 따른다.
모든 기능 판단 기준(PRD §21): **"이 기능이 사용자의 영어 기억을 더 잘 쌓고, 다시 떠올리게 하는가?"** — 아니면 MVP에서 뺀다.

## 팀
1인 개발(dgkwon90). 사람 리뷰어가 없으므로 codex/agy 교차 검토가 실질적인 2차 검증 수단이다.

## 일하는 방식 (상세는 docs/rules/)
- **사이클**: 이슈(1~2일 크기) → 설계 → TDD → 구현 → lint·취약점 검사 → (중요 결정만) codex/agy 교차검토 → 확정 (`rules/development-cycle.md`)
- **작업 관리**: `docs/planning/backlog.md`가 이슈 대장
- **AI·토큰 효율**: tri-review는 ADR급 결정에만, 범위는 파일 단위로 명시, 재리뷰 금지 (`rules/ai-collaboration.md`)
- **용어**: 새 용어는 `docs/glossary.md`에 등재, 같은 개념에 두 단어 금지

## 기술 스택
- Backend sidecar: **Go** (`apps/desktopd`) — HTTP API + SQLite + AI provider 연동 + 복습 스케줄
- Desktop UI: **Tauri 2 + TypeScript + React** (`apps/desktop-ui`) — `docs/adr/ADR-0005-frontend-framework.md`
- 로컬 DB: **SQLite** (WAL 모드, FTS5)
- AI Provider: `Explainer` 인터페이스로 추상화, 1차 실연동 provider는 **Gemini** (`docs/adr/ADR-0004`)

## Repo 구조
```
apps/{desktopd, desktop-ui, api(추후 중앙서버)}   deploy/   scripts/   docs/
```
1. `apps/desktopd/internal/domain/`은 infra(AI provider, DB 드라이버 등)를 직접 알면 안 된다 — interface로 주입 (PRD §18.1)
2. `apps/desktopd` ↔ `apps/desktop-ui`는 직접 import 금지 — 로컬 HTTP API로만 통신 (PRD §15)
3. SQLite 스키마 변경은 migration 파일로만
4. API key 등 secret은 평문 저장·커밋 금지
5. **원격 push는 사용자가 명시 지시할 때만**

## AI 오케스트레이션
Claude가 중심, `.claude/agents/`의 codex-worker(구현 위임)·agy-worker(대량 분석)를 작업자로. 위임 결과는 diff/파일 직접 검증 후 채택. `--dangerously-*` 플래그는 사용자가 해당 대화에서 명시 허락 시만. 되돌리기 어려운 결정만 `/tri-review`.

## 현재 상태 (2026-07-07)
설계 문서·ADR·백로그 준비 완료. 원격 저장소 `github.com/dgkwon90/dictionary`(public) 연결·push 완료.
착수 전 확정 필요 항목 모두 결정됨: 제품명=**Neulsang**(ADR-0006), AI provider=**Gemini**(ADR-0004), UI=**React**(ADR-0005).
백로그 #1(desktopd bootstrap) 완료: `go.work` + `apps/desktopd`(module `neulsang/desktopd`), `GET /healthz`, config/slog, internal 경계 골격. 로컬 API 기본 주소 `127.0.0.1:48989`(`NEULSANG_ADDR`).
다음 작업: `docs/planning/backlog.md` #2 (SQLite schema & migration).
