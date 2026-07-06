# GitHub 워크플로우 규칙

> 상태(2026-07-03): 아직 GitHub 저장소 없음. 로컬 git repo만 초기화됨(`git init` 완료, 커밋 없음).
> 저장소를 만들기로 하면 이 문서의 "연결 시점에 할 일"부터 실행한다.

## 이슈
- **1 이슈 = 1~2일 크기의 완결 작업.** 크면 쪼개서 부모 이슈에 체크리스트로 연결
- 본문 필수 3요소: **목적**(왜) / **수용 기준**(무엇이 되면 완료인가, 테스트 가능하게) / **범위 밖**(이번에 안 하는 것)
- 라벨 체계 (1인 개발이라 `role:` 라벨은 두지 않음):
  - `area:` desktopd | desktop-ui | docs | deploy
  - `kind:` feat | fix | refactor | docs | chore
- 마일스톤 = `docs/planning/backlog.md`의 버전(v0.1 ~ v0.5) 그대로

## 브랜치·커밋
- 저장소 생성 전까지는 `main`에 직접 커밋 (1인 개발, PR 오버헤드 불필요)
- 저장소·원격 연결 후에도 1인 개발이면 기능 브랜치는 선택 사항 — 단, 되돌리기 어려운 변경(스키마 마이그레이션, ADR급)은 브랜치+자기 리뷰(diff 재확인) 권장
- 브랜치명(사용 시): `feat/12-review-scheduler` (`kind/이슈번호-요약`)
- 커밋 메시지: Conventional Commits (`feat:`, `fix:`, `docs:`, `refactor:`, `chore:`)

## push 규칙
- **Claude(AI)**: 원격 push·PR 생성은 사용자가 그 대화에서 지시할 때만. 로컬 커밋은 지시 범위 내 자유
- **사용자**: 자유

## 연결 시점에 할 일 (저장소를 만들기로 확정하면, 1회성)
- [ ] `brew install gh` + `gh auth login`
- [x] GitHub repo 생성 — `dgkwon90/dictionary`로 **public** 생성·연결됨 (2026-07-07). 원래 private 권장이었으나 공개로 결정 — 커밋 전 민감정보 스캔을 거쳤고, API key 등 secret은 앞으로도 커밋 금지 원칙 유지
- [ ] 라벨 생성: `area:*`, `kind:*`
- [ ] 마일스톤 생성: v0.1 ~ v0.5
- [ ] `docs/planning/backlog.md` → 이슈 일괄 등록, backlog.md는 `docs/archive/`로 이동
- [ ] (선택) 1인 개발이면 branch protection은 생략 가능 — 실수 방지가 목적이면 `main` 직접 push 금지만 켜도 충분
