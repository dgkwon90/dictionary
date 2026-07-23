// Inbox 화면(#15, PRD §10.4/§9.4).
//
// 검색 기록을 New/Saved/Review Added/Archived/Failed 탭으로 보고, 행을 펼치면 그 캡처의
// 추출 단어를 단어별 "모름/알아요"로 복습 대상에 넣거나 뺀다("모름"이 review card 생성).

import { useCallback, useEffect, useState } from "react";
import {
  api,
  type CaptureKnowledgeItem,
  type InboxItem,
  type InboxStatus,
} from "../api/client";
import "./Inbox.css";

const TABS: { label: string; status: InboxStatus }[] = [
  { label: "신규", status: "new" },
  { label: "저장됨", status: "saved" },
  { label: "복습 추가됨", status: "review_added" },
  { label: "보관됨", status: "archived" },
  { label: "실패", status: "failed" },
];

export default function Inbox() {
  const [tab, setTab] = useState<InboxStatus>("new");
  const [items, setItems] = useState<InboxItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);

  const load = useCallback(async (status: InboxStatus, opts?: { silent?: boolean }) => {
    if (!opts?.silent) {
      setLoading(true);
      setError(null);
    }
    try {
      const res = await api.listInbox(status);
      setItems(res.items);
      // silent 성공도 이전 에러를 지운다(정상 복구 신호) — 다만 실패를 미리 지우지는
      // 않는다: 위에서 silent일 땐 setError(null)을 안 부르므로, 조용한 폴링이 계속
      // 실패해도 화면에 떠 있던 에러 메시지가 조용히 사라지지 않는다.
      if (opts?.silent) setError(null);
    } catch (err) {
      // 조용한 백그라운드 재조회 실패는 화면을 비우지 않는다 — 다음 폴에서 다시 시도.
      if (!opts?.silent) {
        setError(err instanceof Error ? err.message : String(err));
        setItems([]);
      }
    } finally {
      if (!opts?.silent) setLoading(false);
    }
  }, []);

  useEffect(() => {
    setExpanded(null);
    void load(tab);
  }, [tab, load]);

  // New 탭은 AI 해석이 백그라운드에서 비동기로 끝나므로(초 단위), 완료된 결과가
  // 늦게 반영되는 것을 막기 위해 짧은 간격으로 조용히 재조회한다(로딩 표시 없음).
  // 단, 사용자가 행을 펼쳐서 보고 있는 동안(expanded !== null)은 멈춘다 — 그 행에서
  // "모름"을 눌러 카드가 생성되면 서버 조회 시 즉시 review_added로 파생되어 New
  // 목록에서 빠지는데, 폴링이 계속 돌면 사용자가 방금 확인한 "복습 카드 N개 생성"
  // 메시지가 몇 초 안에 예고 없이 사라져버린다(실사용 테스트로 발견한 경합).
  useEffect(() => {
    if (tab !== "new" || expanded !== null) return;
    const timer = setInterval(() => {
      void load(tab, { silent: true });
    }, 3000);
    return () => clearInterval(timer);
  }, [tab, load, expanded]);

  const setStatus = async (captureId: string, action: "save" | "archive") => {
    try {
      if (action === "save") await api.saveInbox(captureId);
      else await api.archiveInbox(captureId);
      // 상태가 바뀌면 현재 탭에서 빠지므로 목록을 다시 불러온다.
      await load(tab);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  return (
    <div className="inbox">
      <nav className="ib-tabs">
        {TABS.map((t) => (
          <button
            key={t.status}
            className={t.status === tab ? "ib-tab active" : "ib-tab"}
            onClick={() => setTab(t.status)}
          >
            {t.label}
          </button>
        ))}
      </nav>

      <div className="ib-list">
        {loading && <p className="ib-msg">불러오는 중…</p>}
        {error && <p className="ib-error">⚠ {error}</p>}
        {!loading && !error && items.length === 0 && <p className="ib-msg">항목이 없습니다.</p>}
        {items.map((item) => (
          <InboxRow
            key={item.capture_id}
            item={item}
            expanded={expanded === item.capture_id}
            onToggle={() =>
              setExpanded(expanded === item.capture_id ? null : item.capture_id)
            }
            onSave={() => setStatus(item.capture_id, "save")}
            onArchive={() => setStatus(item.capture_id, "archive")}
          />
        ))}
      </div>
    </div>
  );
}

function InboxRow({
  item,
  expanded,
  onToggle,
  onSave,
  onArchive,
}: {
  item: InboxItem;
  expanded: boolean;
  onToggle: () => void;
  onSave: () => void;
  onArchive: () => void;
}) {
  return (
    <div className="ib-row">
      <div className="ib-row-head">
        <button className="ib-row-main" onClick={onToggle}>
          <span className="ib-caret">{expanded ? "▾" : "▸"}</span>
          <span className="ib-text">{item.selected_text}</span>
          {item.brief_ko && <span className="ib-brief">{item.brief_ko}</span>}
        </button>
        <div className="ib-actions">
          {item.status === "new" && <button onClick={onSave}>저장</button>}
          {item.status !== "archived" && <button onClick={onArchive}>보관</button>}
        </div>
      </div>
      {expanded && <KnowledgeList captureId={item.capture_id} />}
    </div>
  );
}

function KnowledgeList({ captureId }: { captureId: string }) {
  const [items, setItems] = useState<CaptureKnowledgeItem[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notes, setNotes] = useState<Record<string, string>>({});

  useEffect(() => {
    let active = true;
    api
      .listCaptureKnowledge(captureId)
      .then((res) => active && setItems(res.items))
      .catch((err) => active && setError(err instanceof Error ? err.message : String(err)));
    return () => {
      active = false;
    };
  }, [captureId]);

  const mark = async (id: string, kind: "unknown" | "known") => {
    try {
      const result = kind === "unknown" ? await api.markUnknown(id) : await api.markKnown(id);
      setItems((prev) =>
        prev
          ? prev.map((it) =>
              it.knowledge_item_id === id
                ? { ...it, status: result.status, wrong_count: result.wrong_count }
                : it,
            )
          : prev,
      );
      setNotes((prev) => ({
        ...prev,
        [id]:
          kind === "unknown"
            ? result.cards_created > 0
              ? `복습 카드 ${result.cards_created}개 생성`
              : "모름 표시됨(추가 카드 없음)"
            : "알고 있음 표시됨",
      }));
    } catch (err) {
      setNotes((prev) => ({
        ...prev,
        [id]: `⚠ ${err instanceof Error ? err.message : String(err)}`,
      }));
    }
  };

  if (error) return <p className="ib-error ib-sub">⚠ {error}</p>;
  if (items === null) return <p className="ib-msg ib-sub">단어 불러오는 중…</p>;
  if (items.length === 0) return <p className="ib-msg ib-sub">추출된 단어가 없습니다.</p>;

  return (
    <div className="ib-knowledge">
      {items.map((it) => (
        <div key={it.knowledge_item_id} className="ib-word">
          <div className="ib-word-main">
            <b>{it.surface_text}</b>
            {it.pronunciation_ko && <span className="ib-pron"> {it.pronunciation_ko}</span>}
            {it.meaning_ko && <span className="ib-mean"> — {it.meaning_ko}</span>}
            {it.status === "known" && <span className="ib-known">알고 있음</span>}
          </div>
          <div className="ib-word-actions">
            <button onClick={() => mark(it.knowledge_item_id, "unknown")}>모름</button>
            <button onClick={() => mark(it.knowledge_item_id, "known")}>알아요</button>
            {notes[it.knowledge_item_id] && (
              <span className="ib-note">{notes[it.knowledge_item_id]}</span>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}
