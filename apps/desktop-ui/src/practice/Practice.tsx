// 연습(Practice) 화면.
//
// 복습 스케줄(due)과 무관하게 사용자가 고른 단어를 반복 연습한다. 채점은 하되 일정은
// 건드리지 않는다 — 연습에서 맞히고 틀린 것은 정답률에 그대로 반영되지만(연습도 실력이다),
// 아무리 연습해도 내일 복습할 목록은 그대로다. 그래서 "복습 카드를 미리 소진해 버릴까 봐
// 연습을 못 하는" 상황이 생기지 않는다.

import { useCallback, useEffect, useRef, useState } from "react";
import { api, type ReviewCard, type ReviewRating } from "../api/client";
import { cardTypeLabel } from "../labels";
import "./Practice.css";

// Today Review와 같은 4단계. 라벨과 키를 맞춰 두면 두 화면 사이에서 손이 헷갈리지 않는다.
const RATINGS: { rating: ReviewRating; label: string; key: string }[] = [
  { rating: "again", label: "모르겠어요", key: "1" },
  { rating: "hard", label: "어려웠어요", key: "2" },
  { rating: "good", label: "알아요", key: "3" },
  { rating: "easy", label: "쉬워요", key: "4" },
];

const RATING_KEYS: Record<string, ReviewRating | undefined> = {
  "1": "again",
  "2": "hard",
  "3": "good",
  "4": "easy",
};

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
          복습 스케줄과 무관하게 원하는 단어를 골라 반복 연습해요. 맞고 틀린 건 정답률에
          반영되지만, 복습 예정일은 바뀌지 않아요.
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
          <p className="pr-msg">
            연습할 카드가 없어요. 검색한 뒤 "학습할래요"로 담으면 카드가 생겨요.
          </p>
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
  const [accuracy, setAccuracy] = useState<number | null>(null);
  const [gradeError, setGradeError] = useState<string | null>(null);
  // 한 카드에 두 번 채점되지 않게 막는다(키보드와 버튼이 같은 동작을 부른다).
  const grading = useRef(false);

  const reveal = useCallback(() => setMode({ ...mode, revealed: true }), [mode, setMode]);

  // 다음 카드로 넘어가면서 방금 채점의 결과를 함께 싣는다.
  //
  // 결과를 세운 뒤 따로 next()를 부르면 안 된다: 두 setState가 같은 배치에 들어가
  // 나중 것이 이긴다 — 정답률도 실패 메시지도 화면에 한 번도 뜨지 않고, 채점이
  // 실패해도 사용자는 기록된 줄 안다.
  const advance = useCallback(
    (feedback: { accuracy?: number; error?: string }) => {
      setAccuracy(feedback.accuracy ?? null);
      setGradeError(feedback.error ?? null);
      grading.current = false;
      setMode({ ...mode, idx: idx + 1, revealed: false });
    },
    [mode, setMode, idx],
  );
  const next = useCallback(() => advance({}), [advance]);

  // 채점하고 다음 카드로. 실패해도 연습은 계속 굴러간다 — 정답률 한 번 못 적은 것이
  // 세션을 끊을 이유는 아니다. 대신 실패했다는 사실은 남긴다.
  const grade = useCallback(
    async (rating: ReviewRating) => {
      if (grading.current) return;
      grading.current = true;
      try {
        const result = await api.gradePractice(card.card_id, rating);
        advance({ accuracy: result.accuracy });
      } catch (err) {
        advance({ error: err instanceof Error ? err.message : String(err) });
      }
    },
    [card, advance],
  );
  // 채점 없이 현재 카드를 세션 끝에 재삽입 → 그 세트를 한 바퀴 더 돌 때 다시 나온다.
  const againLater = useCallback(() => {
    setAccuracy(null);
    setGradeError(null);
    grading.current = false;
    setMode({ kind: "session", cards: [...cards, card], idx: idx + 1, revealed: false });
  }, [cards, card, idx, setMode]);

  useEffect(() => {
    if (done) return;
    const onKey = (e: KeyboardEvent) => {
      if (!revealed && (e.key === " " || e.key === "Enter")) {
        e.preventDefault();
        reveal();
      } else if (revealed) {
        const rating = RATING_KEYS[e.key];
        if (rating) {
          e.preventDefault();
          void grade(rating);
        } else if (e.key === " " || e.key === "Enter") {
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
  }, [done, revealed, reveal, next, againLater, grade]);

  // 채점 결과는 카드가 아니라 세션에 속한다: 마지막 카드를 채점하면 곧바로 완료 화면이
  // 뜨는데, 그 결과를 카드 레이아웃 안에만 두면 한 장짜리 연습에서는 한 번도 볼 수 없다.
  const feedback = (
    <>
      {accuracy !== null && (
        <p className="pr-accuracy">방금 그 단어 정답률 {Math.round(accuracy * 100)}%</p>
      )}
      {gradeError && <p className="pr-error">⚠ 채점을 기록하지 못했어요: {gradeError}</p>}
    </>
  );

  if (done) {
    return (
      <div className="pr-center">
        <p className="pr-done-title">연습 완료 🎉</p>
        {feedback}
        <div className="pr-center-actions">
          <button
            className="pr-secondary"
            onClick={() => setMode({ kind: "session", cards, idx: 0, revealed: false })}
          >
            같은 세트 다시
          </button>
          <button className="pr-secondary" onClick={onExit}>
            카드 다시 고르기
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
        <>
          <div className="pr-grades">
            {RATINGS.map(({ rating, label, key }) => (
              <button
                key={rating}
                className={`pr-grade pr-grade-${rating}`}
                onClick={() => void grade(rating)}
              >
                {label} <kbd>{key}</kbd>
              </button>
            ))}
          </div>
          <div className="pr-sess-actions">
            <button className="pr-secondary" onClick={next}>
              채점 없이 다음 <kbd>Space</kbd>
            </button>
            <button
              className="pr-secondary"
              onClick={againLater}
              title="이 세트를 한 바퀴 더 돌 때 다시 나와요"
            >
              한 번 더 <kbd>R</kbd>
            </button>
          </div>
        </>
      )}

      {feedback}
    </div>
  );
}
