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

  // 지우기는 서버가 성공을 돌려준 뒤에 목록에서 뺀다. 먼저 지우면 실패했을 때 사라진
  // 것처럼 보이고, 다음 새로고침에 되살아나서 더 헷갈린다.
  const remove = async (n: NotificationItem) => {
    setError(null);
    try {
      await api.deleteNotification(n.id);
      setItems((prev) => prev.filter((x) => x.id !== n.id));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  // 모두 지우기는 되돌릴 수 없어서 한 번 더 묻는다 — 브라우저 confirm 대신 버튼이
  // 스스로 바뀌게 해서, 실수로 누른 사람이 아무 데나 클릭하면 취소되게 한다.
  const [confirmingClear, setConfirmingClear] = useState(false);
  const clearAll = async () => {
    setError(null);
    try {
      await api.deleteAllNotifications();
      setItems([]);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setConfirmingClear(false);
    }
  };

  return (
    <div className="nt">
      <div className="nt-head">
        <h1>알림</h1>
        <div className="nt-head-actions">
          {items.length > 0 &&
            (confirmingClear ? (
              <>
                <button className="nt-danger" onClick={() => void clearAll()}>
                  정말 모두 지울까요?
                </button>
                <button className="nt-secondary" onClick={() => setConfirmingClear(false)}>
                  취소
                </button>
              </>
            ) : (
              <button className="nt-secondary" onClick={() => setConfirmingClear(true)}>
                모두 지우기
              </button>
            ))}
          <button className="nt-secondary" onClick={() => void load()} disabled={loading}>
            {loading ? "새로고침 중…" : "새로고침"}
          </button>
        </div>
      </div>

      {error && <p className="nt-error">⚠ {error}</p>}

      {!loading && items.length === 0 && !error && (
        <p className="nt-msg">아직 알림이 없어요. 검색 결과가 준비되거나 복습 시간이 되면 여기 쌓여요.</p>
      )}

      <ul className="nt-list">
        {items.map((n) => (
          <li key={n.id} className="nt-row">
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
            <button
              className="nt-remove"
              onClick={() => void remove(n)}
              aria-label={`${n.title} 지우기`}
              title="이 알림 지우기"
            >
              ×
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
