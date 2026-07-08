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

## 부록: #6 구현 결정 (2026-07-08)
- **REST 직접 호출, SDK 미도입**: `google.golang.org/genai` 등 공식 SDK 대신 `net/http`로 REST 엔드포인트(`v1beta/models/{model}:generateContent`)를 직접 호출한다. 근거: 의존성 최소 규칙(`rules/development-cycle.md`), Explainer 인터페이스 뒤라 나중에 SDK로 교체해도 도메인 영향 없음(되돌리기 쉬움 → ADR 재작성 불필요)
- **인증**: `x-goog-api-key` 헤더 사용(쿼리 파라미터 금지 — 서버 로그·URL에 키 노출 방지, Google 공식 권장)
- **기본 모델**: `gemini-flash-latest`(자동 갱신 별칭, 이 결정 시점엔 gemini-3.5-flash를 가리킴). 특정 버전을 하드코딩하면 Google의 모델 폐지 주기(예: gemini-2.0-flash가 2026-06-01 폐지)에 매번 코드 변경이 필요해짐. `NEULSANG_GEMINI_MODEL` 환경변수로 재정의 가능
- **API key 저장 방식 결론(위에서 미결로 남긴 것)**: **저장하지 않는다.** `NEULSANG_GEMINI_API_KEY` 환경변수로만 읽고 DB·파일 어디에도 쓰지 않는다 — "평문 저장 금지" 원칙을 저장 자체를 안 함으로써 충족. OS 키체인·암호화 저장은 Settings 화면(#17, `area:desktop-ui`)이 영구 설정 UX를 요구하는 시점에 재검토(`app_settings` 테이블은 이미 스키마에 존재하므로 그때 활용 가능)
- **재시도/타임아웃**: 시도당 20초, 최대 2회 재시도(총 3회 시도), 지수 백오프(300ms/600ms). 429·5xx·전송 오류만 재시도, 4xx(400/401/403 등)는 즉시 실패(재시도해도 해결 안 됨)
- **raw_response_json**: `Explainer.Explain`가 `(ExplainResult, rawResponseJSON string, error)`를 반환하도록 확장 — mock은 자신이 만든 결과의 JSON을, Gemini는 **실제 API 응답 원문**을 반환해 PRD §18.1("raw AI response도 저장한다")을 문자 그대로 충족
- **동기→비동기 전환**: #4에서 mock은 즉시 반환되어 capture 생성 요청 안에서 동기 처리했으나, 실제 Gemini는 초 단위 지연·재시도가 있어 그대로 두면 최악의 경우 API 응답이 1분 가까이 지연된다. 이번 이슈에서 capture→explain 트리거를 비동기(goroutine, 전체 타임아웃 상한)로 전환한다 — PRD §15.1 응답 예시가 애초에 `status:"queued"`인 것과 일치
