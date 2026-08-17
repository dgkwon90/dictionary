# desktopd

Go 백엔드 sidecar. Tauri UI 옆에서 함께 실행되며 SQLite·AI 호출·복습 스케줄·알림 트리거를 담당한다.

구현 완료(재설계 v2 기준). 실제 구조는 아래 목표와 대체로 일치하되 도메인 구성이 다르다 —
`internal/domain/`의 capture·explain·search·learning·review·knowledge·notification·settings·
stats·suggest·backup·outbox가 현재 목록이고, `internal/infra/`는 llm·phonetic·syncpush다.
결정 기록은 `../../docs/adr/ADR-0010-product-redesign-v2.md`.

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
