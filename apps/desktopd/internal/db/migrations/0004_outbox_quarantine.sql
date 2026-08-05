-- 아웃박스는 오래된 것부터 보낸다. 서버가 영원히 거절할 이벤트(400 같은 것)가 맨 앞에
-- 있으면 그 뒤의 정상 이벤트는 전부 그 뒤에 갇힌다 — 재시도는 15초마다 영원히 계속되고,
-- 큐는 늘어나기만 하며, 아무도 이유를 모른다.
--
-- 그런 이벤트는 지우지 않고 격리한다: `failed_at`이 찍힌 행은 전송 대상에서 빠지지만
-- 원장에는 남아 무엇이 왜 거절됐는지 확인할 수 있다(하드 삭제 금지, ADR-0010 D8).
-- 판단 근거를 남기려고 응답도 함께 적는다.
ALTER TABLE sync_outbox ADD COLUMN failed_at DATETIME;
ALTER TABLE sync_outbox ADD COLUMN failure_reason TEXT;
