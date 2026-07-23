// 앱 내 알림 목록 화면(#24, ADR-0008).
//
// 지금까지의 알림(result_ready·review_due)을 원장에서 최신순으로 보여준다. OS 알림/트레이 ●는
// "지금 새 알림"만 표면화하지만, 지나간 알림은 여기서 확인한다. 항목을 클릭하면 확인(ack) 처리하고
// route가 있으면 해당 화면으로 이동한다. 이 목록은 알림 켜짐 여부와 무관하게 항상 조회된다.

import { useCallback, useEffect, useState } from "react";
import { api, type NotificationItem } from "../api/client";
import { routeLabel } from "../labels";
import "./Notifications.css";

const KIND_LABEL: Record<string, string> = {
  result_ready: "결과 준비",
  review_due: "복습 시간",
};

function kindLabel(kind: string): string {
  return KIND_LABEL[kind] ?? kind;
}

function formatTime(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
}

export default function Notifications({ onNavigate }: { onNavigate: (route: string) => void }) {
  const [items, setItems] = useState<NotificationItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.notificationHistory(100);
      setItems(res.notifications);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const onItem = async (n: NotificationItem) => {
    // 확인 처리(best-effort) 후 route가 있으면 이동. 로컬 상태를 즉시 갱신해 배지/dim 반영.
    if (!n.acked) {
      setItems((prev) => prev.map((x) => (x.id === n.id ? { ...x, acked: true } : x)));
      void api.ackNotification(n.id).catch(() => {});
    }
    if (n.route) onNavigate(n.route);
  };

  return (
    <div className="nt">
      <div className="nt-head">
        <h1>알림</h1>
        <button className="nt-secondary" onClick={() => void load()} disabled={loading}>
          {loading ? "새로고침 중…" : "새로고침"}
        </button>
      </div>

      {error && <p className="nt-error">⚠ {error}</p>}

      {!loading && items.length === 0 && !error && (
        <p className="nt-msg">아직 알림이 없어요. 검색 결과가 준비되거나 복습 시간이 되면 여기 쌓여요.</p>
      )}

      <ul className="nt-list">
        {items.map((n) => (
          <li key={n.id}>
            <button
              className={n.acked ? "nt-item acked" : "nt-item"}
              onClick={() => void onItem(n)}
              title={n.route ? `${routeLabel(n.route)} 화면으로 이동` : undefined}
            >
              {!n.acked && <span className="nt-dot" aria-label="새 알림" />}
              <span className="nt-kind">{kindLabel(n.kind)}</span>
              <span className="nt-main">
                <span className="nt-title">{n.title}</span>
                {n.body && <span className="nt-body">{n.body}</span>}
              </span>
              <span className="nt-time">{formatTime(n.created_at)}</span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
