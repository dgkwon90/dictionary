// Dashboard 화면(#17, PRD §10.6/§15.7).
//
// desktopd의 GET /v1/dashboard/summary를 읽어 학습 지표를 표시하는 읽기전용 화면.
// 오늘/이번 주 검색·복습 수, due 카드, 가장 많이 검색/자주 틀린 단어, 카테고리별 약점.
// (PRD §10.6의 "최근 7일 추세"는 요약 API에 시계열이 없어 후속으로 미룬다.)

import { useCallback, useEffect, useState } from "react";
import { api, type DashboardSummary } from "../api/client";
import "./Dashboard.css";

const CATEGORY_LABELS: Record<string, string> = {
  backend: "백엔드",
  infra: "인프라",
  database: "데이터베이스",
  network: "네트워크",
  general: "일반",
};

export default function Dashboard() {
  const [summary, setSummary] = useState<DashboardSummary | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setSummary(await api.dashboardSummary());
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setSummary(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  if (loading && !summary) {
    return <div className="db-msg">불러오는 중…</div>;
  }
  if (error) {
    return (
      <div className="db-center">
        <p className="db-error">불러오지 못했습니다: {error}</p>
        <button className="db-secondary" onClick={() => void load()}>
          다시 시도
        </button>
      </div>
    );
  }
  if (!summary) {
    return <div className="db-msg">데이터 없음</div>;
  }

  return (
    <div className="db">
      <div className="db-head">
        <h1>Dashboard</h1>
        <button className="db-secondary" onClick={() => void load()}>
          새로고침
        </button>
      </div>

      <section className="db-stats">
        <Stat label="오늘 검색" value={summary.today_search_count} />
        <Stat label="이번 주 검색" value={summary.week_search_count} />
        <Stat label="오늘 복습 완료" value={summary.today_completed_reviews} />
        <Stat label="복습 대기(due)" value={summary.due_card_count} />
      </section>

      <div className="db-cols">
        <WordList title="가장 많이 검색한 단어" words={summary.most_searched} unit="회" />
        <WordList title="가장 자주 틀린 단어" words={summary.most_wrong} unit="회" />
      </div>

      <section className="db-panel">
        <h2>카테고리별 약점</h2>
        {summary.category_weakness.length === 0 ? (
          <p className="db-empty">아직 데이터가 없습니다.</p>
        ) : (
          <ul className="db-weakness">
            {summary.category_weakness.map((c) => (
              <li key={c.category}>
                <span className="db-cat">{CATEGORY_LABELS[c.category] ?? c.category}</span>
                <span className="db-bar">
                  <span
                    className="db-bar-fill"
                    style={{ width: `${Math.min(100, c.weakness_score * 100)}%` }}
                  />
                </span>
                <span className="db-score">
                  {c.weakness_score.toFixed(2)} <em>({c.item_count})</em>
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="db-stat">
      <div className="db-stat-value">{value}</div>
      <div className="db-stat-label">{label}</div>
    </div>
  );
}

function WordList({
  title,
  words,
  unit,
}: {
  title: string;
  words: { knowledge_item_id: string; surface_text: string; count: number }[];
  unit: string;
}) {
  return (
    <section className="db-panel">
      <h2>{title}</h2>
      {words.length === 0 ? (
        <p className="db-empty">아직 데이터가 없습니다.</p>
      ) : (
        <ol className="db-words">
          {words.map((w) => (
            <li key={w.knowledge_item_id}>
              <span className="db-word">{w.surface_text}</span>
              <span className="db-count">
                {w.count}
                {unit}
              </span>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
