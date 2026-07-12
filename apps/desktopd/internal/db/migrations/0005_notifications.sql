-- ADR-0008: 사이드카→UI 이벤트 전달. notifications는 dedup/ack/coalesce/배지 카운트
-- 원장이다. dedup_key는 전역 UNIQUE라 coalesce와 ack 후 재발화 방지를 동시에 충족한다
-- (result_ready=capture_id 1건, review_due="review_due:<date>:<slot>" 슬롯당 1건).
-- expires_at으로 재시작 후 지난 알림 재발을 막는다(result_ready 짧은 TTL).

CREATE TABLE notifications (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK(kind IN ('result_ready', 'review_due')),
  dedup_key TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  route TEXT,
  payload_id TEXT,
  created_at DATETIME NOT NULL,
  expires_at DATETIME,
  acked_at DATETIME
);

CREATE INDEX idx_notifications_unacked ON notifications(created_at) WHERE acked_at IS NULL;
