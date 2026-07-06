# desktopd

Go 백엔드 sidecar. Tauri UI 옆에서 함께 실행되며 SQLite·AI 호출·복습 스케줄·알림 트리거를 담당한다.

아직 코드 없음. 백로그 `../../docs/planning/backlog.md` #1(bootstrap)부터 시작한다.

패키지 구조 목표(PRD §14.2):
```
cmd/desktopd/
internal/
  app/{bootstrap,lifecycle}
  config/
  logger/
  db/{sqlite,migrations}
  domain/{capture,explain,knowledge,review,reminder,stats,sync}
  infra/{llm,clipboard,notifier,scheduler,outbox,device}
  transport/http/{router.go,handlers/}
```
`domain/`은 `infra/`를 직접 import하지 않는다 (`../../docs/rules/development-cycle.md` 디렉토리 경계 규칙).
