-- Backlog #9: track which review_card_candidates have already been turned into
-- review_cards, so marking a word unknown more than once does not create duplicate
-- cards, while candidates added by a later capture still become cards on the next
-- mark-unknown.

ALTER TABLE review_card_candidates ADD COLUMN consumed_at DATETIME;

CREATE INDEX idx_review_card_candidates_unconsumed
ON review_card_candidates(knowledge_item_id, consumed_at);
