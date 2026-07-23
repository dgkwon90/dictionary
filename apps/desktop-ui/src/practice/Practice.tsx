// 연습(Practice) 화면(#28).
//
// 복습 스케줄(due)과 무관하게 사용자가 고른 단어를 반복 연습한다. Today Review와 달리
// 채점(grade)이 없어 서버에 아무것도 쓰지 않는다 → due_at·mastery·복습 이력이 전혀 바뀌지
// 않는다(순수 자가확인). GET /v1/practice/cards로 카드를 검색·선택하고, 로컬에서 세션을 돈다.

import { useCallback, useEffect, useRef, useState } from "react";
import { api, type ReviewCard } from "../api/client";
import { cardTypeLabel } from "../labels";
import "./Practice.css";

type Mode =
  | { kind: "picker" }
  | { kind: "session"; cards: ReviewCard[]; idx: number; revealed: boolean };

export default function Practice() {
  const [query, setQuery] = useState("");
  const [cards, setCards] = useState<ReviewCard[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [mode, setMode] = useState<Mode>({ kind: "picker" });
  // 검색을 바꿔도 이전에 선택한 카드 객체를 잃지 않도록, 본 카드를 전부 누적 보관한다.
  const pool = useRef<Map<string, ReviewCard>>(new Map());

  const search = useCallback(async (q: string) => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.practiceCards(q || undefined, 200);
      for (const c of res.cards) pool.current.set(c.card_id, c);
      setCards(res.cards);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void search("");
  }, [search]);

  const toggle = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const allVisibleSelected = cards.length > 0 && cards.every((c) => selectedIds.has(c.card_id));
  const toggleAllVisible = () => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (allVisibleSelected) for (const c of cards) next.delete(c.card_id);
      else for (const c of cards) next.add(c.card_id);
      return next;
    });
  };

  const start = () => {
    const chosen = [...selectedIds]
      .map((id) => pool.current.get(id))
      .filter((c): c is ReviewCard => Boolean(c));
    if (chosen.length === 0) return;
    setMode({ kind: "session", cards: chosen, idx: 0, revealed: false });
  };

  if (mode.kind === "session") {
    return <Session mode={mode} setMode={setMode} onExit={() => setMode({ kind: "picker" })} />;
  }

  return (
    <div className="pr">
      <div className="pr-head">
        <h1>연습</h1>
        <p className="pr-note">
          복습 스케줄과 무관하게 원하는 단어를 골라 반복 연습해요. 연습은 복습 예정일·정답률에
          영향을 주지 않습니다.
        </p>
      </div>

      <form
        className="pr-searchbar"
        onSubmit={(e) => {
          e.preventDefault();
          void search(query);
        }}
      >
        <input
          className="pr-search"
          value={query}
          onChange={(e) => setQuery(e.currentTarget.value)}
          placeholder="단어·뜻으로 검색 (비우면 전체)"
          spellCheck={false}
        />
        <button type="submit" className="pr-secondary">
          검색
        </button>
      </form>

      {error && <p className="pr-error">⚠ {error}</p>}

      <div className="pr-listwrap">
        {loading ? (
          <p className="pr-msg">불러오는 중…</p>
        ) : cards.length === 0 ? (
          <p className="pr-msg">연습할 카드가 없어요. 단어를 검색해 "모름"으로 표시하면 카드가 생겨요.</p>
        ) : (
          <>
            <label className="pr-row pr-selall">
              <input type="checkbox" checked={allVisibleSelected} onChange={toggleAllVisible} />
              <span>보이는 {cards.length}개 전체 선택</span>
            </label>
            <ul className="pr-list">
              {cards.map((c) => (
                <li key={c.card_id}>
                  <label className="pr-row">
                    <input
                      type="checkbox"
                      checked={selectedIds.has(c.card_id)}
                      onChange={() => toggle(c.card_id)}
                    />
                    <span className="pr-word">{c.question}</span>
                    <span className="pr-type">{cardTypeLabel(c.card_type)}</span>
                  </label>
                </li>
              ))}
            </ul>
          </>
        )}
      </div>

      <div className="pr-actions">
        <button className="pr-start" onClick={start} disabled={selectedIds.size === 0}>
          선택한 {selectedIds.size}개 연습 시작
        </button>
        {selectedIds.size > 0 && (
          <button className="pr-secondary" onClick={() => setSelectedIds(new Set())}>
            선택 해제
          </button>
        )}
      </div>
    </div>
  );
}

function Session({
  mode,
  setMode,
  onExit,
}: {
  mode: { kind: "session"; cards: ReviewCard[]; idx: number; revealed: boolean };
  setMode: (m: Mode) => void;
  onExit: () => void;
}) {
  const { cards, idx, revealed } = mode;
  const card = cards[idx];
  const done = idx >= cards.length;

  const reveal = useCallback(() => setMode({ ...mode, revealed: true }), [mode, setMode]);
  const next = useCallback(() => {
    setMode({ ...mode, idx: idx + 1, revealed: false });
  }, [mode, setMode, idx]);
  // 채점 없이 현재 카드를 세션 끝에 재삽입 → 그 세트를 한 바퀴 더 돌 때 다시 나온다.
  const againLater = useCallback(() => {
    setMode({ kind: "session", cards: [...cards, card], idx: idx + 1, revealed: false });
  }, [cards, card, idx, setMode]);

  useEffect(() => {
    if (done) return;
    const onKey = (e: KeyboardEvent) => {
      if (!revealed && (e.key === " " || e.key === "Enter")) {
        e.preventDefault();
        reveal();
      } else if (revealed) {
        if (e.key === " " || e.key === "Enter") {
          e.preventDefault();
          next();
        } else if (e.key === "r" || e.key === "R") {
          e.preventDefault();
          againLater();
        }
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [done, revealed, reveal, next, againLater]);

  if (done) {
    return (
      <div className="pr-center">
        <p className="pr-done-title">연습 완료 🎉</p>
        <div className="pr-center-actions">
          <button
            className="pr-secondary"
            onClick={() => setMode({ kind: "session", cards, idx: 0, revealed: false })}
          >
            같은 세트 다시
          </button>
          <button className="pr-secondary" onClick={onExit}>
            단어 다시 고르기
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="pr-sess">
      <div className="pr-sess-top">
        <span className="pr-progress">
          {idx + 1} / {cards.length}
        </span>
        <button className="pr-exit" onClick={onExit}>
          연습 종료
        </button>
      </div>

      <div className="pr-card">
        <div className="pr-cardtype">{cardTypeLabel(card.card_type)}</div>
        <div className="pr-question">{card.question}</div>
        {revealed ? (
          <>
            <hr className="pr-divider" />
            <div className="pr-answer">{card.answer}</div>
            {card.explanation && <div className="pr-explanation">{card.explanation}</div>}
          </>
        ) : (
          <button className="pr-reveal" onClick={reveal}>
            답 보기 <kbd>Space</kbd>
          </button>
        )}
      </div>

      {revealed && (
        <div className="pr-sess-actions">
          <button className="pr-next" onClick={next}>
            다음 <kbd>Space</kbd>
          </button>
          <button className="pr-secondary" onClick={againLater} title="이 세트를 한 바퀴 더 돌 때 다시 나옵니다">
            한 번 더 <kbd>R</kbd>
          </button>
        </div>
      )}
    </div>
  );
}
