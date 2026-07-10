import React from "react";
import ReactDOM from "react-dom/client";
import { getCurrentWindow } from "@tauri-apps/api/window";
import App from "./App";
import QuickSearch from "./quicksearch/QuickSearch";

// 두 윈도우(main / quicksearch)가 같은 번들을 로드하므로 라벨로 렌더 대상을 고른다.
const isQuickSearch = getCurrentWindow().label === "quicksearch";

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>{isQuickSearch ? <QuickSearch /> : <App />}</React.StrictMode>,
);
