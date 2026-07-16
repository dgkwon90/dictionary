// Quick Search 팝업(PRD §10.2, §9.1~9.3).
//
// 글로벌 단축키(Cmd/Ctrl+Shift+E)나 트레이로 열린다. 열릴 때 클립보드를 자동 삽입하고
// (직접 입력도 가능), 제출하면 POST /v1/captures → 해석(GET .../explanation)을 폴링해
// 결과를 보여준다. Escape로 창을 숨긴다.

import { useCallback, useEffect, useRef, useState } from "react";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { listen } from "@tauri-apps/api/event";
import { readText } from "@tauri-apps/plugin-clipboard-manager";
import { api, type Explanation, type InputMode } from "../api/client";
import "./QuickSearch.css";

type Phase =
  | { kind: "input" }
  | { kind: "searching" }
  | { kind: "result"; explanation: Explanation }
  | { kind: "error"; message: string };

const POLL_INTERVAL_MS = 700;
const POLL_TIMEOUT_MS = 90_000;

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export default function QuickSearch() {
  const [text, setText] = useState("");
  const [phase, setPhase] = useState<Phase>({ kind: "input" });
  const inputRef = useRef<HTMLInputElement>(null);
  // 삽입된 클립보드 원문과 같으면 input_mode=clipboard, 손대면 manual.
  const clipboardText = useRef("");
  // 진행 중인 검색을 세대 번호로 취소(재활성화 시 이전 폴링 무효화).
  const searchGen = useRef(0);

  // 팝업 활성화(최초 + 단축키/트레이 재호출): 클립보드 재삽입 + 초기화 + 포커스.
  const activate = useCallback(async () => {
    searchGen.current += 1;
    setPhase({ kind: "input" });
    let clip = "";
    try {
      clip = (await readText())?.trim() ?? "";
    } catch {
      clip = "";
    }
    clipboardText.current = clip;
    setText(clip);
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  useEffect(() => {
    void activate();
    const unlisten = listen("quicksearch:activate", () => void activate());
    return () => void unlisten.then((off) => off());
  }, [activate]);

  const hide = useCallback(() => {
    // 닫는 순간 진행 중인 검색 폴을 취소한다(세대번호 증가) — 그래야 숨겨진 팝업이
    // 결과를 self-ack 하지 않고, 해석이 끝나면 Rust 셸의 폴 루프가 OS 배너로 알린다
    // (#18/ADR-0008: "안 보고 있을 때 결과 준비를 알림"). 이미 결과를 봐서 ack된 뒤라면
    // 폴 루프는 이미 반환했으므로 이 증가는 무해하다.
    searchGen.current += 1;
    getCurrentWindow()
      .hide()
      .catch((err) => console.error("hide quicksearch failed", err));
  }, []);

  // Escape는 포커스 위치와 무관하게 닫혀야 하므로 document 레벨에서 듣는다.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        hide();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [hide]);

  const submit = useCallback(async () => {
    const trimmed = text.trim();
    if (trimmed === "") return;

    const gen = ++searchGen.current;
    setPhase({ kind: "searching" });
    const inputMode: InputMode = trimmed === clipboardText.current ? "clipboard" : "manual";

    try {
      const created = await api.createCapture({ text: trimmed, input_mode: inputMode });
      const deadline = Date.now() + POLL_TIMEOUT_MS;
      for (;;) {
        if (searchGen.current !== gen) return; // 재활성화로 취소됨
        const snap = await api.getExplanation(created.capture_id);
        if (searchGen.current !== gen) return; // 요청 중 재활성화됐으면 결과 폐기
        if (snap.status === "done" && snap.explanation) {
          setPhase({ kind: "result", explanation: snap.explanation });
          // 팝업에서 이미 결과를 봤으므로 이 capture의 result_ready 알림을 소비한다
          // (폴 루프의 중복 OS 알림 방지, #18). best-effort.
          void api.ackCaptureNotification(created.capture_id).catch(() => {});
          return;
        }
        if (snap.status === "failed") {
          setPhase({ kind: "error", message: snap.error_message || "해석에 실패했습니다." });
          return;
        }
        if (Date.now() > deadline) {
          setPhase({ kind: "error", message: "해석이 시간 내에 끝나지 않았습니다." });
          return;
        }
        await sleep(POLL_INTERVAL_MS);
      }
    } catch (err) {
      if (searchGen.current !== gen) return;
      setPhase({ kind: "error", message: err instanceof Error ? err.message : String(err) });
    }
  }, [text]);

  return (
    <div className="qs">
      <form
        className="qs-bar"
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <input
          ref={inputRef}
          className="qs-input"
          value={text}
          onChange={(e) => setText(e.currentTarget.value)}
          placeholder="단어·용어·문장을 입력하고 Enter…"
          autoFocus
          spellCheck={false}
        />
        <button className="qs-go" type="submit" disabled={phase.kind === "searching"}>
          검색
        </button>
      </form>

      <div className="qs-body">
        {phase.kind === "input" && <p className="qs-hint">Enter로 검색, Esc로 닫기</p>}
        {phase.kind === "searching" && <p className="qs-hint">해석 중…</p>}
        {phase.kind === "error" && <p className="qs-error">⚠ {phase.message}</p>}
        {phase.kind === "result" && <Result explanation={phase.explanation} />}
      </div>
    </div>
  );
}

function Result({ explanation }: { explanation: Explanation }) {
  const { brief_ko, pronunciation_ko, detailed_ko, domain_category, examples, sub_items } =
    explanation;
  return (
    <div className="qs-result">
      <p className="qs-brief">{brief_ko}</p>
      {pronunciation_ko && <p className="qs-pron">🔊 {pronunciation_ko}</p>}
      {domain_category && <span className="qs-tag">{domain_category}</span>}
      {detailed_ko && <p className="qs-detail">{detailed_ko}</p>}

      {examples.length > 0 && (
        <ul className="qs-examples">
          {examples.map((ex, i) => (
            <li key={i}>
              <span className="qs-en">{ex.english}</span>
              {ex.korean && <span className="qs-ko"> — {ex.korean}</span>}
            </li>
          ))}
        </ul>
      )}

      {sub_items.length > 0 && (
        <div className="qs-subitems">
          {sub_items.map((s, i) => (
            <div key={i} className="qs-subitem">
              <b>{s.surface_text}</b>
              {s.pronunciation_ko && <span className="qs-pron-sm"> {s.pronunciation_ko}</span>}
              {s.meaning_ko && <span> — {s.meaning_ko}</span>}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
