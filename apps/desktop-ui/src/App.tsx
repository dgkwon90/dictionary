import { useEffect, useState } from "react";
import { listen } from "@tauri-apps/api/event";
import { api } from "./api/client";
import Inbox from "./inbox/Inbox";
import Review from "./review/Review";
import Practice from "./practice/Practice";
import Notifications from "./notifications/Notifications";
import Dashboard from "./dashboard/Dashboard";
import Settings from "./settings/Settings";
import "./App.css";

// 메인 윈도우 화면 라우트. 트레이 navigate 이벤트는 이 중 일부만 emit(Practice/Notifications는 앱 내 탭).
// 식별자(Route)는 Rust tray.rs가 emit하는 라벨과 그대로 맞춰야 하므로 영문 그대로 두고,
// 화면에 표시하는 문구만 LABELS로 분리한다(한글 번역이 tray 배선에 영향 없게).
const ROUTES = ["Inbox", "Today Review", "Practice", "Notifications", "Dashboard", "Settings"] as const;
type Route = (typeof ROUTES)[number];

const LABELS: Record<Route, string> = {
  Inbox: "인박스",
  "Today Review": "오늘의 복습",
  Practice: "연습",
  Notifications: "알림",
  Dashboard: "대시보드",
  Settings: "설정",
};

const DESCRIPTIONS: Record<Route, string> = {
  Inbox: "검색 기록을 신규/저장됨/복습 추가됨/보관됨/실패로 정리 (#15)",
  "Today Review": "오늘 복습할 카드 세션 (#16)",
  Practice: "스케줄 무시하고 원하는 단어를 골라 연습 (#28)",
  Notifications: "지난 알림 목록 (#24)",
  Dashboard: "학습 지표와 약점 카테고리 (#17)",
  Settings: "단축키·AI provider·API key·알림·DB 경로 (#17)",
};

function App() {
  const [route, setRoute] = useState<Route>("Inbox");
  const [online, setOnline] = useState<boolean | null>(null);

  // 트레이 메뉴 클릭 → Rust가 보내는 navigate 이벤트로 화면 전환.
  useEffect(() => {
    const unlisten = listen<string>("navigate", (event) => {
      if ((ROUTES as readonly string[]).includes(event.payload)) {
        setRoute(event.payload as Route);
      }
    });
    return () => {
      void unlisten.then((off) => off());
    };
  }, []);

  // 사이드카 연결 상태 표시(스켈레톤: 주기 폴링).
  useEffect(() => {
    let active = true;
    const check = async () => {
      const ok = await api.health();
      if (active) setOnline(ok);
    };
    void check();
    const timer = setInterval(check, 5000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, []);

  return (
    <div className="shell">
      <nav className="tabs">
        {ROUTES.map((name) => (
          <button
            key={name}
            className={name === route ? "tab active" : "tab"}
            onClick={() => setRoute(name)}
          >
            {LABELS[name]}
          </button>
        ))}
      </nav>

      <main className="screen">
        {route === "Inbox" ? (
          <Inbox />
        ) : route === "Today Review" ? (
          <Review />
        ) : route === "Practice" ? (
          <Practice />
        ) : route === "Notifications" ? (
          <Notifications onNavigate={(r) => setRoute(r as Route)} />
        ) : route === "Dashboard" ? (
          <Dashboard />
        ) : route === "Settings" ? (
          <Settings />
        ) : (
          <>
            <h1>{route}</h1>
            <p>{DESCRIPTIONS[route]}</p>
            <p className="hint">화면 구현은 백로그 프론트엔드 트랙에서 채워집니다.</p>
          </>
        )}
      </main>

      {/* 사이드카는 앱이 켜지면 항상 같이 뜨는 필수 구성요소라, 정상일 때 매 화면마다
          "연결됨"을 광고하는 건 순소음이다(다른 상태 표시 관례 — 알림 배지·트레이 아이콘 —
          도 전부 "정상=조용함, 이상만 신호"). 그래서 연결 실패로 확인됐을 때만 보여준다. */}
      {online === false && (
        <footer className="status status-warn">
          <span className="dot off" />
          desktopd에 연결할 수 없습니다 — 앱을 재시작해 주세요.
        </footer>
      )}
    </div>
  );
}

export default App;
