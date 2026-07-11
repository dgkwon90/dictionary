// Today Review 화면(#16, PRD §10.5/§9.5).
//
// due 카드를 하나씩 앞면(질문)만 보여주고, 사용자가 스스로 떠올린 뒤 "답 보기"로 뒷면
// (답/설명)을 공개한다. Again/Hard/Good/Easy로 채점하면 다음 카드로 넘어간다.
// 키보드: Space/Enter=답 보기, 1~4=Again/Hard/Good/Easy.

import { useCallback, useEffect, useRef, useState } from "react";
import { api, type ReviewCard, type ReviewRating } from "../api/client";
import "./Review.css";

type Phase = "loading" | "error" | "empty" | "active" | "done";

const GRADES: { rating: ReviewRating; label: string; key: string }[] = [
  { rating: "again", label: "Again", key: "1" },
  { rating: "hard", label: "Hard", key: "2" },
  { rating: "good", label: "Good", key: "3" },
  { rating: "easy", label: "Easy", key: "4" },
];

export default function Review() {
  const [cards, setCards] = useState<ReviewCard[]>([]);
  const [idx, setIdx] = useState(0);
  const [revealed, setRevealed] = useState(false);
  const [phase, setPhase] = useState<Phase>("loading");
  const [error, setError] = useState<string | null>(null);
  const [reviewed, setReviewed] = useState(0);
  const grading = useRef(false);
  const shownAt = useRef(Date.now());

  const load = useCallback(async () => {
    setPhase("loading");
    setError(null);
    try {
      const res = await api.dueReviews();
      setCards(res.cards);
      setIdx(0);
      setRevealed(false);
      setReviewed(0);
      shownAt.current = Date.now();
      setPhase(res.cards.length === 0 ? "empty" : "active");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setPhase("error");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const card = cards[idx];

  const reveal = useCallback(() => setRevealed(true), []);

  const grade = useCallback(
    async (rating: ReviewRating) => {
      if (!card || grading.current) return;
      grading.current = true;
      try {
        await api.gradeReview(card.card_id, rating, Date.now() - shownAt.current);
        const next = idx + 1;
        setReviewed((n) => n + 1);
        if (next >= cards.length) {
          setPhase("done");
        } else {
          setIdx(next);
          setRevealed(false);
          shownAt.current = Date.now();
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
        setPhase("error");
      } finally {
        grading.current = false;
      }
    },
    [card, idx, cards.length],
  );

  // 키보드: 카드가 활성일 때만. Space/Enter=답 보기, 1~4=채점(공개 후).
  useEffect(() => {
    if (phase !== "active") return;
    const onKey = (e: KeyboardEvent) => {
      if (!revealed && (e.key === " " || e.key === "Enter")) {
        e.preventDefault();
        reveal();
        return;
      }
      if (revealed) {
        const g = GRADES.find((x) => x.key === e.key);
        if (g) {
          e.preventDefault();
          void grade(g.rating);
        }
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [phase, revealed, reveal, grade]);

  if (phase === "loading") return <p className="rv-msg">불러오는 중…</p>;
  if (phase === "error")
    return (
      <div className="rv-center">
        <p className="rv-error">⚠ {error}</p>
        <button className="rv-secondary" onClick={() => void load()}>
          다시 시도
        </button>
      </div>
    );
  if (phase === "empty")
    return (
      <div className="rv-center">
        <p className="rv-done-title">복습할 카드가 없어요 🎉</p>
        <p className="rv-msg">Inbox에서 단어를 "모름"으로 표시하면 복습 카드가 생겨요.</p>
        <button className="rv-secondary" onClick={() => void load()}>
          새로고침
        </button>
      </div>
    );
  if (phase === "done")
    return (
      <div className="rv-center">
        <p className="rv-done-title">복습 완료 🎉</p>
        <p className="rv-msg">이번 세션에서 {reviewed}개 카드를 복습했어요.</p>
        <button className="rv-secondary" onClick={() => void load()}>
          더 복습하기
        </button>
      </div>
    );

  return (
    <div className="rv">
      <div className="rv-progress">
        {idx + 1} / {cards.length}
      </div>

      <div className="rv-card">
        <div className="rv-type">{card.card_type}</div>
        <div className="rv-question">{card.question}</div>

        {revealed ? (
          <>
            <hr className="rv-divider" />
            <div className="rv-answer">{card.answer}</div>
            {card.explanation && <div className="rv-explanation">{card.explanation}</div>}
          </>
        ) : (
          <button className="rv-reveal" onClick={reveal}>
            답 보기 <kbd>Space</kbd>
          </button>
        )}
      </div>

      {revealed && (
        <div className="rv-grades">
          {GRADES.map((g) => (
            <button
              key={g.rating}
              className={`rv-grade rv-${g.rating}`}
              onClick={() => void grade(g.rating)}
            >
              {g.label}
              <kbd>{g.key}</kbd>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
