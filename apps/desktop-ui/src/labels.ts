// 여러 화면에서 같이 쓰는 한글 표시 라벨. 서버가 주는 값(영문 enum 등)은 그대로 두고
// 사람이 읽는 문구만 여기서 통일한다 — 같은 개념에 서로 다른 번역이 생기지 않게 한다.

// 메인 윈도우 화면 라우트. 트레이 navigate 이벤트(Rust tray.rs)와 앱 내 알림 route가
// 모두 이 문자열로 온다. App.tsx가 아니라 여기 두는 이유: Notifications.tsx도 같은
// 라우트→한글 라벨 매핑이 필요한데, App.tsx는 Notifications를 import하므로 거꾸로
// App.tsx에서 가져오면 순환 참조가 된다.
export const ROUTES = [
  "Search History",
  "Learning",
  "Today Review",
  "Practice",
  "Notifications",
  "Dashboard",
  "Settings",
] as const;
export type Route = (typeof ROUTES)[number];

const ROUTE_LABELS: Record<Route, string> = {
  "Search History": "검색 기록",
  Learning: "학습 목록",
  "Today Review": "오늘 복습",
  Practice: "연습",
  Notifications: "알림",
  Dashboard: "내 기록",
  Settings: "설정",
};

export function routeLabel(route: string): string {
  // 이미 저장된 알림이 옛 route 이름을 들고 오므로, 라벨도 같은 해석을 거친다 —
  // 안 그러면 알림 목록에만 "Inbox"가 영문 그대로 남는다.
  const resolved = resolveRoute(route);
  return resolved ? ROUTE_LABELS[resolved] : route;
}

// 서버는 알림 route를 DB에 저장한다 — 이름을 바꾸기 전에 쌓인 값이 그대로 남아 있고,
// 그 알림을 누른 사용자는 아무 데도 가지 못한다. 서버·Rust까지 개명이 끝나면(백로그 #10)
// 이 표는 지운다.
const LEGACY_ROUTES: Record<string, Route> = {
  Inbox: "Search History",
};

/** navigate 이벤트나 알림이 들고 온 route 문자열을 실제 화면으로 해석한다. */
export function resolveRoute(route: string): Route | null {
  if ((ROUTES as readonly string[]).includes(route)) return route as Route;
  return LEGACY_ROUTES[route] ?? null;
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
