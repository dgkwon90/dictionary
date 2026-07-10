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
  { label: "New", status: "new" },
  { label: "Saved", status: "saved" },
  { label: "Review Added", status: "review_added" },
  { label: "Archived", status: "archived" },
  { label: "Failed", status: "failed" },
];

export default function Inbox() {
  const [tab, setTab] = useState<InboxStatus>("new");
  const [items, setItems] = useState<InboxItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);

  const load = useCallback(async (status: InboxStatus) => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.listInbox(status);
      setItems(res.items);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setItems([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    setExpanded(null);
    void load(tab);
  }, [tab, load]);

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
