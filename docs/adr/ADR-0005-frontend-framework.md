# ADR-0005: Tauri UI 프레임워크 선택

- 날짜: 2026-07-03 (2026-07-04 확정) / 상태: 승인

## 맥락
PRD §7.1은 "React 또는 Svelte"를 추천만 하고 확정하지 않았다. 백로그 #13(Tauri 셸 부트스트랩)을 시작하려면 하나를 골라야 한다.

## 결정
**React**.

이유:
- 생태계·자료가 가장 넓어 codex 등 AI 에이전트에게 화면 구현을 위임하기 쉬움 (레퍼런스 코드가 많아 결과 검증도 쉬움)
- Tauri 공식 템플릿·플러그인 예제가 React 기준으로 가장 많음

## 미채택 대안
- **Svelte**: 번들 크기·문법 단순성이 강점이나 AI 위임 시 참고 코드가 React보다 적다.
- **SolidJS**: React 문법 + Svelte급 성능이나 생태계가 가장 좁아 1인 개발에 자료 부족 리스크.

## 결과·트레이드오프
- `apps/desktop-ui`는 Vite + React + TypeScript + Tauri 2 plugin 기준으로 스캐폴딩
- 상태 관리 라이브러리(Zustand/Redux 등)는 화면 수가 적어(트레이, Quick Search, Inbox, Result Detail, Review, Dashboard, Settings) 백로그 #13에서 필요할 때 최소로 추가 — 성급한 도입 금지
