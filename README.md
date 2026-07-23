# Neulsang (늘상)

업무 중 마주친 영어 단어·용어·문장을 단축키로 즉시 검색·AI 해석하고, 그 기록을 복습 카드로 전환하는 로컬 우선(local-first) 데스크톱 앱.

## 현재 상태 (2026-07-23)
백로그 #1~#32(backend v0.1~v0.3, Tauri UI v0.4, 백업·동기화 v0.5, 다중 플랫폼 릴리스)가
모두 완료되어 핵심 사용자 흐름(Quick Search·Inbox·Review·Practice·Dashboard·Settings·
알림·백업·복원)이 기능적으로 전부 동작한다. `v0.1.0` 태그로 mac arm64/x86_64 +
Windows 11 배포판을 GitHub Actions로 자동 빌드·릴리스한다.

이후 진행한 프로젝트 리뷰([`docs/reviews/2026-07-22-project-review.md`](docs/reviews/2026-07-22-project-review.md))의
보안·안정성 지적사항(RW-01~RW-10: 로컬 API 인증·요청 제한·graceful shutdown·백업
스냅샷 v2·PR 품질 게이트 등)도 각각 별도 브랜치+codex 교차검토를 거쳐 `main`에
병합 완료했다. **다만 [`docs/planning/remaining-work.md`](docs/planning/remaining-work.md)가 정의하는
"Usable MVP" 마일스톤은 RW-11(macOS/Windows 실기기 GUI 수동 검증)이 끝나야 공식
종료된다** — 아직 미착수다. RW-12(이 문서 동기화)는 이번에 완료했다.

## 시작하기
1. [`docs/prd.md`](docs/prd.md) — 제품 정의 원본부터 읽는다
2. [`docs/README.md`](docs/README.md) — 문서 지도
3. [`docs/adr/`](docs/adr/README.md) — 확정된 결정 기록 (Neulsang / Gemini / React)
4. [`docs/planning/remaining-work.md`](docs/planning/remaining-work.md) — 지금 남은 작업
5. [`docs/planning/backlog.md`](docs/planning/backlog.md) — 완료 이력과 상세 구현 기록

## 빌드 · 실행
로컬에서 직접 띄워 테스트하는 법(백엔드 curl / `tauri dev` / 번들 `.app` / 검증 게이트)은 [`docs/development.md`](docs/development.md) 참고.

```bash
# 백엔드만 (curl 검증용)
cd apps/desktopd && go run ./cmd/desktopd     # 127.0.0.1:48989

# 프론트 + 백엔드 통합 (트레이·단축키·팝업)
npm --prefix apps/desktop-ui run tauri dev
```

## 기술 스택
- Backend sidecar: Go (`apps/desktopd`)
- Desktop UI: Tauri 2 + TypeScript (`apps/desktop-ui`)
- 로컬 DB: SQLite (WAL, FTS5)

## AI 협업
Claude가 오케스트레이터, `codex`/`agy` CLI를 작업자로 위임한다 (`.claude/`, `docs/rules/ai-collaboration.md`).
