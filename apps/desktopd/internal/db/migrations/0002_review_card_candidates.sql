-- Backlog #8: persist AI review_card_candidates (PRD §12.1) in a queryable,
-- linkable form so #9 can build review_cards from them. Until now candidates
-- survived only inside explanations.raw_response_json.

CREATE TABLE review_card_candidates (
  id TEXT PRIMARY KEY,
  capture_id TEXT NOT NULL,
  knowledge_item_id TEXT,
  card_type TEXT NOT NULL,
  question TEXT NOT NULL,
  answer TEXT NOT NULL,
  explanation TEXT,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (capture_id) REFERENCES captures(id),
  FOREIGN KEY (knowledge_item_id) REFERENCES knowledge_items(id)
);

CREATE INDEX idx_review_card_candidates_knowledge_item_id
ON review_card_candidates(knowledge_item_id);

CREATE INDEX idx_review_card_candidates_capture_id
ON review_card_candidates(capture_id);
