// 여러 화면에서 같이 쓰는 한글 표시 라벨. 서버가 주는 값(영문 enum 등)은 그대로 두고
// 사람이 읽는 문구만 여기서 통일한다 — 같은 개념에 서로 다른 번역이 생기지 않게 한다.

// 메인 윈도우 화면 라우트. 트레이 navigate 이벤트(Rust tray.rs)와 앱 내 알림 route가
// 모두 이 문자열로 온다. App.tsx가 아니라 여기 두는 이유: Notifications.tsx도 같은
// 라우트→한글 라벨 매핑이 필요한데, App.tsx는 Notifications를 import하므로 거꾸로
// App.tsx에서 가져오면 순환 참조가 된다.
export const ROUTES = ["Inbox", "Today Review", "Practice", "Notifications", "Dashboard", "Settings"] as const;
export type Route = (typeof ROUTES)[number];

const ROUTE_LABELS: Record<Route, string> = {
  Inbox: "검색함",
  "Today Review": "오늘 복습",
  Practice: "연습",
  Notifications: "알림",
  Dashboard: "내 기록",
  Settings: "설정",
};

export function routeLabel(route: string): string {
  return (ROUTE_LABELS as Record<string, string>)[route] ?? route;
}

const CARD_TYPE_LABELS: Record<string, string> = {
  meaning: "뜻 맞추기",
  reverse: "영어로 떠올리기",
  cloze: "빈칸 채우기",
  context: "쓰임 고르기",
  sentence_translation: "문장 해석하기",
};

export function cardTypeLabel(cardType: string): string {
  return CARD_TYPE_LABELS[cardType] ?? cardType;
}

const CATEGORY_LABELS: Record<string, string> = {
  backend: "백엔드",
  infra: "인프라",
  database: "데이터베이스",
  network: "네트워크",
  general: "일반",
};

export function categoryLabel(category: string): string {
  return CATEGORY_LABELS[category] ?? category;
}
