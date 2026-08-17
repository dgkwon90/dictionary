-- 알림 삭제는 행을 지우지 않고 표시만 한다.
--
-- dedup_key UNIQUE + Enqueue의 ON CONFLICT DO NOTHING이 "같은 알림을 두 번 띄우지
-- 않는다"를 지탱한다. 복습 리마인더는 슬롯 창(2시간) 동안 스케줄러가 매 틱마다 다시
-- 넣으려 하고, 그때 이미 있는 행이 그것을 막는다. 행을 진짜로 지우면 그 방어가 사라져
-- 다음 틱에 같은 알림이 되살아난다 — 사용자가 방금 지운 것이 몇 초 뒤 배너로 다시 뜬다.
--
-- 그래서 지운 표시(deleted_at)만 남기고 행은 둔다. 목록·배너 조회는 이 컬럼으로 거르고,
-- 중복 방어는 그대로 유효하다.
ALTER TABLE notifications ADD COLUMN deleted_at DATETIME;
