// 검색 기록 화면.
//
// 이전 Inbox의 5탭(새 것/저장/복습할 것/보관/실패)을 대체한다. 저장·보관은 사라졌고,
// 남은 질문은 하나다 — "이 검색을 학습할 것인가". 그래서 화면은 아직 정하지 않은 것
// (미확인)을 기본으로 보여주고, 전체 이력은 눌러야 나온다.
//
// 행을 펼치는 대신 오른쪽 상세 패널을 쓴다: 문장의 단어 선택 UI는 인라인 확장에 들어가지
// 않는다(단어 목록 + 원문 + 완료 버튼).

import { useCallback, useEffect, useRef, useState } from "react";
import {
  api,
  type LearnKind,
  type SearchDetail,
  type SearchItem,
  type SearchView,
  type TriageResult,
} from "../api/client";
import TriageActions from "../triage/TriageActions";
import "./SearchHistory.css";

const KIND_CHIPS: { label: string; kind?: LearnKind }[] = [
  { label: "전체" },
  { label: "단어", kind: "word" },
  { label: "문장", kind: "sentence" },
];

const STATE_LABELS: Record<string, string> = {
  unseen: "아직 안 봤어요",
  needs_selection: "단어를 고르는 중",
  learning: "학습 중",
  discarded: "삭제함",
};

export interface SearchHistoryProps {
  /** 팝업에서 "모르는 단어 고르기"로 넘어온 경우 그 검색을 바로 연다. */
  openCaptureId?: string;
}

export default function SearchHistory({ openCaptureId }: SearchHistoryProps) {
  const [view, setView] = useState<SearchView>("unresolved");
  const [kind, setKind] = useState<LearnKind | undefined>(undefined);
  const [items, setItems] = useState<SearchItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const load = useCallback(
    async (opts?: { silent?: boolean }) => {
      if (!opts?.silent) {
        setLoading(true);
        setError(null);
      }
      try {
        const res = await api.listSearches({ view, kind });
        setItems(res.items);
        if (opts?.silent) setError(null);
      } catch (err) {
        // 조용한 재조회 실패는 화면을 비우지 않는다 — 다음 폴에서 다시 시도.
        if (!opts?.silent) {
          setError(err instanceof Error ? err.message : String(err));
          setItems([]);
        }
      } finally {
        if (!opts?.silent) setLoading(false);
      }
    },
    [view, kind],
  );

  useEffect(() => {
    void load();
  }, [load]);

  // 해석이 끝나기 전 검색은 아직 단어/문장 판정도, 버튼도 없다. 끝나는 즉시 반영되도록
  // 진행 중인 것이 남아 있는 동안만 조용히 재조회한다(다 끝나면 폴링도 멈춘다).
  const hasPending = items.some((it) => it.job_status === "queued" || it.job_status === "running");
  useEffect(() => {
    if (!hasPending) return;
    const timer = setInterval(() => void load({ silent: true }), 2000);
    return () => clearInterval(timer);
  }, [hasPending, load]);

  // 팝업이 넘긴 검색을 연다. 그 검색이 미확인이 아닐 수도 있으므로(이미 학습 중인 문장에
  // 단어를 더 고르러 오는 경우) 목록에 없으면 전체 보기로 바꿔 찾아준다.
  const handledOpenId = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (!openCaptureId || handledOpenId.current === openCaptureId) return;
    handledOpenId.current = openCaptureId;
    setSelectedId(openCaptureId);
  }, [openCaptureId]);
  useEffect(() => {
    if (!selectedId || loading) return;
    if (view === "all" || items.some((it) => it.capture_id === selectedId)) return;
    setView("all");
  }, [selectedId, items, loading, view]);

  const unresolvedCount = view === "unresolved" ? items.length : null;

  return (
    <div className="sh">
      <div className="sh-head">
        <h1>검색 기록</h1>
        <p className="sh-note">
          찾아본 것들이에요. 학습할지 정하지 않은 것부터 보여줍니다.
        </p>
      </div>

      <div className="sh-filters">
        <div className="sh-chips">
          <button
            className={view === "unresolved" ? "sh-chip active" : "sh-chip"}
            onClick={() => setView("unresolved")}
          >
            미확인
            {unresolvedCount !== null && unresolvedCount > 0 && (
              <span className="sh-badge">{unresolvedCount}</span>
            )}
          </button>
          <button
            className={view === "all" ? "sh-chip active" : "sh-chip"}
            onClick={() => setView("all")}
          >
            전체
          </button>
        </div>
        <div className="sh-chips">
          {KIND_CHIPS.map((chip) => (
            <button
              key={chip.label}
              className={kind === chip.kind ? "sh-chip active" : "sh-chip"}
              onClick={() => setKind(chip.kind)}
            >
              {chip.label}
            </button>
          ))}
        </div>
      </div>

      {error && <p className="sh-error">⚠ {error}</p>}

      <div className="sh-split">
        <div className="sh-list">
          {loading && <p className="sh-msg">불러오는 중…</p>}
          {!loading && items.length === 0 && (
            <p className="sh-msg">
              {view === "unresolved"
                ? "정할 것이 없어요. 다 처리했네요."
                : "아직 검색한 것이 없어요."}
            </p>
          )}
          {items.map((item) => (
            <button
              key={item.capture_id}
              className={item.capture_id === selectedId ? "sh-row active" : "sh-row"}
              onClick={() => setSelectedId(item.capture_id)}
            >
              <span className="sh-row-top">
                <span className="sh-text">{item.selected_text}</span>
                <KindBadge kind={item.learn_kind} jobStatus={item.job_status} />
              </span>
              <span className="sh-row-bottom">
                {item.job_status === "failed" ? (
                  <span className="sh-failed">해석하지 못했어요</span>
                ) : (
                  <span className="sh-brief">{item.brief_ko ?? "해석 중…"}</span>
                )}
                <span className="sh-state">{STATE_LABELS[item.triage_state] ?? item.triage_state}</span>
              </span>
            </button>
          ))}
        </div>

        <div className="sh-detail">
          {selectedId ? (
            <Detail
              key={selectedId}
              captureId={selectedId}
              onChanged={() => void load()}
              onClose={() => setSelectedId(null)}
            />
          ) : (
            <p className="sh-msg">왼쪽에서 검색을 골라 보세요.</p>
          )}
        </div>
      </div>
    </div>
  );
}

function KindBadge({ kind, jobStatus }: { kind?: LearnKind; jobStatus: string }) {
  if (jobStatus === "queued" || jobStatus === "running") {
    return <span className="sh-kind pending">해석 중</span>;
  }
  if (!kind) return null;
  return <span className="sh-kind">{kind === "sentence" ? "문장" : "단어"}</span>;
}

function Detail({
  captureId,
  onChanged,
  onClose,
}: {
  captureId: string;
  onChanged: () => void;
  onClose: () => void;
}) {
  const [detail, setDetail] = useState<SearchDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [picking, setPicking] = useState(false);
  // 결과 안내는 버튼이 아니라 이 패널이 들고 있는다. 학습에 담는 순간 버튼들은 사라지는데
  // (담은 검색에는 담을 버튼이 없다), 방금 카드가 몇 장 생겼는지는 그때 알려줘야 한다.
  const [outcome, setOutcome] = useState<string | null>(null);

  const reload = useCallback(async () => {
    try {
      setDetail(await api.getSearch(captureId));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, [captureId]);

  useEffect(() => {
    void reload();
  }, [reload]);

  // 문장 결과를 열어 본 것 자체가 상태 전이다(unseen → needs_selection): 열어는 봤지만
  // 모르는 단어를 아직 안 골랐다면 여전히 미해결이라는 뜻을 상태로 남긴다. 멱등이라
  // 다시 열어도 안전하다.
  const opened = useRef(false);
  useEffect(() => {
    if (!detail || opened.current) return;
    if (detail.learn_kind !== "sentence" || detail.triage_state !== "unseen") return;
    opened.current = true;
    void api
      .openSearch(captureId)
      .then(() => {
        setPicking(true);
        void reload();
        onChanged();
      })
      .catch((err) => setError(err instanceof Error ? err.message : String(err)));
  }, [detail, captureId, reload, onChanged]);

  // 단어를 고르는 중인 문장으로 돌아오면 선택 패널을 바로 편다.
  useEffect(() => {
    if (detail?.triage_state === "needs_selection") setPicking(true);
  }, [detail?.triage_state]);

  if (error) return <p className="sh-error">⚠ {error}</p>;
  if (!detail) return <p className="sh-msg">불러오는 중…</p>;

  const done = (result: TriageResult) => {
    setDetail({ ...detail, triage_state: result.triage_state });
    onChanged();
    if (result.triage_state === "discarded") {
      onClose();
      return;
    }
    if (result.triage_state === "learning") {
      setPicking(false);
      setOutcome(
        result.cards_created > 0
          ? `학습 목록에 담고 복습 카드 ${result.cards_created}장을 만들었어요`
          : "학습 목록에 담았어요",
      );
    }
  };

  return (
    <div className="sh-detail-body">
      <p className="sh-detail-text">{detail.selected_text}</p>
      {detail.job_status === "failed" && (
        <p className="sh-error">⚠ 해석하지 못했어요. 다시 검색해 주세요.</p>
      )}
      {detail.brief_ko && <p className="sh-detail-brief">{detail.brief_ko}</p>}
      {detail.detailed_ko && <p className="sh-detail-detailed">{detail.detailed_ko}</p>}

      {detail.triage_state === "learning" ? (
        <p className="sh-detail-state">{outcome ?? "학습 목록에 담겨 있어요."}</p>
      ) : detail.triage_state === "discarded" ? (
        <p className="sh-detail-state">삭제한 검색이에요.</p>
      ) : (
        <TriageActions
          captureId={captureId}
          learnKind={detail.learn_kind}
          onPickWords={() => setPicking(true)}
          onDone={done}
          onKindChanged={(kind) => {
            // 단어로 바꿨으면 선택 패널은 의미가 없다 — 고를 것이 없는 화면이 남는다.
            if (kind === "word") setPicking(false);
            opened.current = false;
            void reload();
            onChanged();
          }}
        />
      )}

      {detail.learn_kind === "sentence" && picking && detail.triage_state !== "discarded" && (
        <WordSelection detail={detail} onReload={reload} onCompleted={done} />
      )}
    </div>
  );
}

// 문장에서 모르는 단어를 고른다. 고른 단어와 문장이 함께 학습 목록에 들어가고, 고른
// 단어마다 그 문장을 문맥으로 하는 빈칸 카드가 생긴다.
function WordSelection({
  detail,
  onReload,
  onCompleted,
}: {
  detail: SearchDetail;
  onReload: () => Promise<void>;
  onCompleted: (result: TriageResult) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const selectedCount = detail.items.filter((it) => it.selected).length;

  const toggle = async (knowledgeItemId: string, selected: boolean) => {
    setBusy(true);
    setError(null);
    try {
      if (selected) await api.selectWord(detail.capture_id, knowledgeItemId);
      else await api.deselectWord(detail.capture_id, knowledgeItemId);
      await onReload();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const complete = async (noUnknownWords: boolean) => {
    setBusy(true);
    setError(null);
    try {
      onCompleted(await api.completeSelection(detail.capture_id, noUnknownWords));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="sh-select">
      <p className="sh-select-title">모르는 단어를 골라 주세요</p>
      {detail.items.length === 0 ? (
        <p className="sh-msg">이 문장에서 찾은 단어가 없어요.</p>
      ) : (
        <ul className="sh-words">
          {detail.items.map((item) => (
            <li key={item.knowledge_item_id}>
              <label className={item.selected ? "sh-word selected" : "sh-word"}>
                <input
                  type="checkbox"
                  checked={item.selected}
                  disabled={busy}
                  onChange={() => void toggle(item.knowledge_item_id, !item.selected)}
                />
                <span className="sh-word-text">
                  <b>{item.surface_text}</b>
                  {item.pronunciation_ko && (
                    <span className="sh-word-pron"> {item.pronunciation_ko}</span>
                  )}
                  {item.meaning_ko && <span className="sh-word-mean"> — {item.meaning_ko}</span>}
                </span>
              </label>
              {item.selected && item.description_ko && (
                <p className="sh-word-desc">{item.description_ko}</p>
              )}
            </li>
          ))}
        </ul>
      )}
      {error && <p className="sh-error">⚠ {error}</p>}
      <div className="sh-select-actions">
        <button
          className="tri-primary"
          disabled={busy || selectedCount === 0}
          onClick={() => void complete(false)}
        >
          {selectedCount > 0 ? `${selectedCount}개 담고 완료` : "완료"}
        </button>
        {/* 문장은 이해 못 했는데 딱 짚을 단어가 없는 경우. 문장만 학습 대상이 된다. */}
        <button className="tri-ghost" disabled={busy} onClick={() => void complete(true)}>
          짚을 단어는 없어요
        </button>
      </div>
    </div>
  );
}
