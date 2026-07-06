# Neulsang (늘상)

업무 중 마주친 영어 단어·용어·문장을 단축키로 즉시 검색·AI 해석하고, 그 기록을 복습 카드로 전환하는 로컬 우선(local-first) 데스크톱 앱.

## 현재 상태 (2026-07-04)
설계 문서·ADR·백로그 준비 완료. 코드 없음. 착수 전 확정 필요 결정 완료(제품명·AI provider·UI 프레임워크).

## 시작하기
1. [`docs/prd.md`](docs/prd.md) — 제품 정의 원본부터 읽는다
2. [`docs/README.md`](docs/README.md) — 문서 지도
3. [`docs/adr/`](docs/adr/README.md) — 확정된 결정 기록 (Neulsang / Gemini / React)
4. [`docs/planning/backlog.md`](docs/planning/backlog.md) — #1부터 착수

## 기술 스택
- Backend sidecar: Go (`apps/desktopd`)
- Desktop UI: Tauri 2 + TypeScript (`apps/desktop-ui`)
- 로컬 DB: SQLite (WAL, FTS5)

## AI 협업
Claude가 오케스트레이터, `codex`/`agy` CLI를 작업자로 위임한다 (`.claude/`, `docs/rules/ai-collaboration.md`).
