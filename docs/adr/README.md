# ADR (Architecture Decision Records)

되돌리기 어려운 결정 1건 = 파일 1개. `ADR-NNNN-제목.md`, 번호는 순차 증가, 기존 ADR은 수정하지 않고 새 ADR로 대체(대체됨 표기).

## 템플릿
```markdown
# ADR-NNNN: 제목
- 날짜: YYYY-MM-DD / 상태: 제안 | 승인 | 대체됨(→ADR-MMMM)
## 맥락 (무엇을 결정해야 했나)
## 결정
## 근거 (tri-review 결과가 있으면 링크)
## 결과·트레이드오프
```

## 목록
- [ADR-0001](ADR-0001-monorepo.md) — 모노레포 구조 (승인)
- [ADR-0002](ADR-0002-tauri-go-sidecar.md) — Tauri 2 + Go sidecar 아키텍처 (승인)
- [ADR-0003](ADR-0003-sqlite-local-first.md) — SQLite 로컬 우선 저장소 (승인)
- [ADR-0004](ADR-0004-ai-provider.md) — AI Provider 추상화 및 1차 연동: Gemini (승인)
- [ADR-0005](ADR-0005-frontend-framework.md) — Tauri UI 프레임워크: React (승인)
- [ADR-0006](ADR-0006-product-name.md) — 제품명 확정: Neulsang (승인)
- (예정) 동기화 프로토콜(중앙 서버 연동 시점), 복습 스케줄러 알고리즘 고도화(FSRS-lite → FSRS 정식)
