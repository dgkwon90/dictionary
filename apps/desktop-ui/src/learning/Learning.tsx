// 학습 목록 화면.
//
// 검색함이 "내가 찾아본 것"이라면 여기는 "내가 배우기로 한 것"이다. 등록은 검색 결과에서
// [학습할래요](단어) 또는 [모르는 단어 고르기 → 완료](문장)로만 일어나므로, 이 목록은
// 사용자가 의도적으로 담은 것만 담고 있다.
//
// 필터는 [단어|문장] × [전체|오늘|이번 주|자주 틀림]. "자주 틀림"은 등록 이후에도 또 몰랐거나
// 복습에서 틀린 적이 있는 항목만 보여준다 — 등록 자체가 "몰랐다"는 기록이라, 그것까지 세면
// 전체 목록과 똑같아진다.

import { useCallback, useEffect, useState } from "react";
import {
  api,
  type LearnKind,
  type LearningItem,
  type LearningScope,
} from "../api/client";
import "./Learning.css";

const SCOPES: { value: LearningScope; label: string }[] = [
  { value: "all", label: "전체" },
  { value: "today", label: "오늘" },
  { value: "week", label: "이번 주" },
  { value: "weak", label: "자주 틀림" },
];

const KINDS: { value: LearnKind | "all"; label: string }[] = [
  { value: "all", label: "전체" },
  { value: "word", label: "단어" },
  { value: "sentence", label: "문장" },
];

const EMPTY_MESSAGES: Record<LearningScope, string> = {
  all: "아직 학습할 항목이 없어요. 검색한 뒤 [학습할래요]를 누르면 여기에 쌓여요.",
  today: "오늘 등록한 항목이 없어요.",
  week: "이번 주에 등록한 항목이 없어요.",
  weak: "자주 틀리는 항목이 아직 없어요. 계속 잘하고 있다는 뜻이에요.",
};

export default function Learning() {
  const [scope, setScope] = useState<LearningScope>("all");
  const [kind, setKind] = useState<LearnKind | "all">("all");
  const [query, setQuery] = useState("");
  // 입력 중인 검색어와 실제로 서버에 보낸 검색어를 나눈다. 타이핑마다 요청하면 한 글자씩
  // 목록이 흔들린다.
  const [submittedQuery, setSubmittedQuery] = useState("");
  const [items, setItems] = useState<LearningItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.listLearning({
        scope,
        kind: kind === "all" ? undefined : kind,
        q: submittedQuery || undefined,
        limit: 200,
      });
      setItems(res.items);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [scope, kind, submittedQuery]);

  useEffect(() => {
    void load();
  }, [load]);

  // 알겠어요/빼기는 그 항목을 목록에서 내보낸다. 서버가 성공을 돌려준 뒤에 지워야
  // 실패했는데 사라진 것처럼 보이지 않는다.
  const applyAction = async (
    item: LearningItem,
    action: (id: string) => Promise<LearningItem>,
  ) => {
    setBusyId(item.knowledge_item_id);
    setError(null);
    try {
      await action(item.knowledge_item_id);
      setItems((prev) => prev.filter((i) => i.knowledge_item_id !== item.knowledge_item_id));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusyId(null);
    }
  };

  return (
    <div className="lrn">
      <div className="lrn-head">
        <h1>학습 목록</h1>
        <p className="lrn-note">학습하겠다고 담은 단어와 문장이에요. 검색만 한 것은 검색함에 있어요.</p>
      </div>

      <div className="lrn-filters">
        <div className="lrn-chips" role="group" aria-label="종류">
          {KINDS.map((option) => (
            <button
              key={option.value}
              className={option.value === kind ? "lrn-chip active" : "lrn-chip"}
              onClick={() => setKind(option.value)}
            >
              {option.label}
            </button>
          ))}
        </div>
        <div className="lrn-chips" role="group" aria-label="범위">
          {SCOPES.map((option) => (
            <button
              key={option.value}
              className={option.value === scope ? "lrn-chip active" : "lrn-chip"}
              onClick={() => setScope(option.value)}
            >
              {option.label}
            </button>
          ))}
        </div>
        <form
          className="lrn-search"
          onSubmit={(event) => {
            event.preventDefault();
            setSubmittedQuery(query.trim());
          }}
        >
          <input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="단어나 뜻으로 찾기"
            aria-label="학습 목록 검색"
          />
          <button type="submit">찾기</button>
          {submittedQuery && (
            <button
              type="button"
              className="lrn-clear"
              onClick={() => {
                setQuery("");
                setSubmittedQuery("");
              }}
            >
              초기화
            </button>
          )}
        </form>
      </div>

      {error && <p className="lrn-error">{error}</p>}

      {loading ? (
        <p className="lrn-empty">불러오는 중이에요…</p>
      ) : items.length === 0 ? (
        <p className="lrn-empty">{submittedQuery ? "찾는 항목이 없어요." : EMPTY_MESSAGES[scope]}</p>
      ) : (
        <ul className="lrn-list">
          {items.map((item) => (
            <li key={item.knowledge_item_id} className="lrn-item">
              <div className="lrn-item-main">
                <div className="lrn-item-title">
                  <span className={`lrn-kind lrn-kind-${item.learn_kind}`}>
                    {item.learn_kind === "sentence" ? "문장" : "단어"}
                  </span>
                  <span className="lrn-text">{item.surface_text}</span>
                </div>
                {item.meaning_ko && <p className="lrn-meaning">{item.meaning_ko}</p>}
              </div>

              <dl className="lrn-stats">
                <div>
                  <dt>모른 횟수</dt>
                  <dd>{item.unknown_count}</dd>
                </div>
                <div>
                  <dt>정답률</dt>
                  <dd>{formatAccuracy(item)}</dd>
                </div>
                <div>
                  <dt>다음 복습</dt>
                  <dd>{formatDue(item.next_due_at)}</dd>
                </div>
              </dl>

              <div className="lrn-actions">
                <button
                  className="lrn-retire"
                  disabled={busyId === item.knowledge_item_id}
                  onClick={() => void applyAction(item, (id) => api.retireLearningItem(id))}
                >
                  알겠어요
                </button>
                <button
                  className="lrn-remove"
                  disabled={busyId === item.knowledge_item_id}
                  onClick={() => void applyAction(item, (id) => api.removeLearningItem(id))}
                >
                  목록에서 빼기
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// 한 번도 안 푼 항목은 0%가 아니라 "—". 0%는 "다 틀렸다"는 뜻이라 아직 안 푼 것과 섞이면
// 안 된다.
function formatAccuracy(item: LearningItem): string {
  if (item.attempt_count === 0) return "—";
  return `${Math.round(item.accuracy * 100)}% (${item.correct_count}/${item.attempt_count})`;
}

function formatDue(dueAt?: string): string {
  if (!dueAt) return "—";
  const due = new Date(dueAt);
  const startOfToday = new Date();
  startOfToday.setHours(0, 0, 0, 0);
  const days = Math.round((due.getTime() - startOfToday.getTime()) / 86_400_000);
  if (days <= 0) return "지금";
  if (days === 1) return "내일";
  return `${days}일 뒤`;
}
