# ADR-0004: AI Provider 추상화 및 1차 연동

- 날짜: 2026-07-03 (2026-07-04 확정) / 상태: 승인

## 맥락
PRD §7.5는 OpenAI/Gemini/Claude/Local model을 모두 지원 대상으로 열어두되 특정 provider에 강하게 묶이지 않도록 `Explainer` 인터페이스 뒤에 숨기라고 명시한다. 하지만 mock 다음으로 **처음 실제 연동할 provider 1개**는 정해야 백로그 #6(Task04 확장)을 시작할 수 있다.

## 결정
1차 실연동 provider: **Gemini** (Google).

이유:
- 무료 티어가 관대해 개발 초기(1인, MVP) API 비용 부담이 작다
- 구조화 출력(response schema)을 지원해 PRD §12.1의 `ExplainResult` 스키마를 강제할 수 있다
- 한국어 처리 품질이 양호 — 사용자가 한국어 개발자이고 한글 발음 추론(§5.2-3) 등 한국어 맥락이 있어 유리

## 미채택 대안
- **OpenAI**: 구조화 출력이 가장 성숙하나 무료 티어가 없어 초기 비용 발생. 2순위 후보로 유지.
- **Claude**: 자연스러운 해석이 강점이나 마찬가지로 유료. 오케스트레이션은 Claude를 쓰지만 제품 런타임 provider와는 별개 결정.
- **Local model(Ollama 등)**: API 비용 0이지만 품질·속도 편차와 사용자 설치 부담이 커 MVP 단계에서는 보류.

`Explainer` 인터페이스 뒤에 두므로 위 대안은 나중에 추가·교체 가능하다. provider를 바꾸는 게 아니라 **추가**하는 것이므로 이 ADR을 대체할 필요는 없다.

## 결과·트레이드오프
- `internal/infra/llm/`에 `Explainer` 인터페이스 + `mock`(백로그 #4) + `gemini`(백로그 #6) 두 구현을 둔다
- Gemini의 구조화 출력 방식(response schema)이 `ExplainResult`를 얼마나 엄격히 강제하는지는 #6 착수 시 검증 — 부족하면 파싱·검증 레이어를 `Explainer` 구현 내부에 둔다
- API key는 평문 저장 금지 — OS 키체인 또는 최소한 `.env`(git 미추적) + 설정 화면에서 암호화 저장 방식은 백로그 #6에서 별도 결정
