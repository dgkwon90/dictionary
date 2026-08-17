import { useEffect, useState } from "react";
import { listen } from "@tauri-apps/api/event";
import { api } from "./api/client";
import SearchHistory from "./search/SearchHistory";
import Learning from "./learning/Learning";
import Review from "./review/Review";
import Practice from "./practice/Practice";
import Notifications from "./notifications/Notifications";
import Dashboard from "./dashboard/Dashboard";
import Settings from "./settings/Settings";
import { ROUTES, type Route, resolveRoute, routeLabel } from "./labels";
import "./App.css";

const DESCRIPTIONS: Record<Route, string> = {
  "Search History": "찾아본 것들 — 학습할지 아직 정하지 않은 것부터",
  Learning: "학습하겠다고 담은 단어와 문장",
  "Today Review": "오늘 복습할 카드 세션 (#16)",
  Practice: "스케줄 무시하고 원하는 단어를 골라 연습 (#28)",
  Notifications: "지난 알림 목록 (#24)",
  Dashboard: "학습 기록과 약한 부분 (#17)",
  Settings: "단축키·AI 서비스·API 키·알림·저장 위치 (#17)",
};

function App() {
  const [route, setRoute] = useState<Route>("Search History");
  // 팝업이 "이 문장의 단어를 고르러 가라"며 넘긴 검색. 화면 전환만으로는 어느 문장인지
  // 알 수 없어 payload로 함께 온다.
  const [openCaptureId, setOpenCaptureId] = useState<string | undefined>(undefined);
  const [online, setOnline] = useState<boolean | null>(null);
  const [ready, setReady] = useState(false);

  // 트레이 메뉴·알림 클릭·팝업의 "모르는 단어 고르기" → Rust가 보내는 navigate 이벤트.
  useEffect(() => {
    const unlisten = listen<{ route: string; capture_id?: string }>("navigate", (event) => {
      const target = resolveRoute(event.payload.route);
      if (!target) return;
      setRoute(target);
      setOpenCaptureId(event.payload.capture_id);
    });
    return () => {
      void unlisten.then((off) => off());
    };
  }, []);

  // 사이드카 연결 상태 확인. 셸이 desktopd를 spawn한 직후엔 포트가 아직 안 열려있을 수
  // 있는데(특히 Windows는 첫 실행 시 백신이 새로 풀린 exe를 스캔하느라 몇 초 걸릴 수
  // 있음), 화면(검색 기록 등)이 그 틈에 마운트되면 요청이 그대로 실패해 raw 네트워크 에러가
  // 노출된다(실사용 리포트). 그래서 첫 성공 전엔 화면을 아예 그리지 않고 짧은 간격으로
  // 재시도하다가, 한 번 연결되면 그 뒤론 5초 간격으로 낮춰 계속 지켜본다.
  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    const check = async () => {
      const ok = await api.health();
      if (!active) return;
      setOnline(ok);
      if (ok) setReady(true);
      timer = setTimeout(check, ok ? 5000 : 500);
    };
    void check();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, []);

  if (!ready) {
    return (
      <div className="shell shell-loading">
        <p>시작하는 중이에요…</p>
      </div>
    );
  }

  return (
    <div className="shell">
      <nav className="tabs">
        {ROUTES.map((name) => (
          <button
            key={name}
            className={name === route ? "tab active" : "tab"}
            onClick={() => setRoute(name)}
          >
            {routeLabel(name)}
          </button>
        ))}
      </nav>

      <main className="screen">
        {route === "Search History" ? (
          <SearchHistory openCaptureId={openCaptureId} />
        ) : route === "Learning" ? (
          <Learning />
        ) : route === "Today Review" ? (
          <Review />
        ) : route === "Practice" ? (
          <Practice />
        ) : route === "Notifications" ? (
          <Notifications
            onNavigate={(r) => {
              const target = resolveRoute(r);
              if (target) setRoute(target);
            }}
          />
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
          문제가 생겼어요 — 앱을 다시 시작해 주세요.
        </footer>
      )}
    </div>
  );
}

export default App;
