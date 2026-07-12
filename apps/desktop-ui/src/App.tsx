import { useEffect, useState } from "react";
import { listen } from "@tauri-apps/api/event";
import { api } from "./api/client";
import Inbox from "./inbox/Inbox";
import Review from "./review/Review";
import Dashboard from "./dashboard/Dashboard";
import Settings from "./settings/Settings";
import "./App.css";

// 메인 윈도우 화면 라우트(트레이 Rust 쪽 ITEMS와 일치). Quick Search는 별도 팝업 윈도우.
const ROUTES = ["Inbox", "Today Review", "Dashboard", "Settings"] as const;
type Route = (typeof ROUTES)[number];

const DESCRIPTIONS: Record<Route, string> = {
  Inbox: "검색 기록을 New/Saved/Review Added/Archived/Failed로 정리 (#15)",
  "Today Review": "오늘 복습할 카드 세션 (#16)",
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
            {name}
          </button>
        ))}
      </nav>

      <main className="screen">
        {route === "Inbox" ? (
          <Inbox />
        ) : route === "Today Review" ? (
          <Review />
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

      <footer className="status">
        <span className={online ? "dot on" : "dot off"} />
        {online === null ? "desktopd 확인 중…" : online ? "desktopd 연결됨" : "desktopd 미연결"}
      </footer>
    </div>
  );
}

export default App;
