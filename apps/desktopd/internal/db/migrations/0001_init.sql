-- PRD §11 기준, ADR-0007

CREATE TABLE app_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE captures (
  id TEXT PRIMARY KEY,
  source_app TEXT,
  source_type TEXT,
  source_title TEXT,
  source_url TEXT,
  selected_text TEXT NOT NULL,
  detected_lang TEXT,
  input_mode TEXT NOT NULL,
  text_hash TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  inbox_status TEXT NOT NULL DEFAULT 'new',
  CHECK(inbox_status IN ('new','saved','archived'))
);

CREATE INDEX idx_captures_created_at ON captures(created_at DESC);
CREATE INDEX idx_captures_text_hash ON captures(text_hash);

CREATE TABLE lookup_jobs (
  id TEXT PRIMARY KEY,
  capture_id TEXT NOT NULL,
  status TEXT NOT NULL,
  provider TEXT,
  model TEXT,
  prompt_version TEXT,
  error_message TEXT,
  started_at DATETIME,
  finished_at DATETIME,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (capture_id) REFERENCES captures(id)
);

CREATE INDEX idx_lookup_jobs_status_created_at
ON lookup_jobs(status, created_at DESC);

CREATE TABLE explanations (
  id TEXT PRIMARY KEY,
  capture_id TEXT NOT NULL,
  brief_ko TEXT NOT NULL,
  detailed_ko TEXT NOT NULL,
  pronunciation TEXT,
  examples_json TEXT,
  terms_json TEXT,
  difficulty_estimate REAL,
  category TEXT,
  raw_response_json TEXT,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (capture_id) REFERENCES captures(id)
);

CREATE UNIQUE INDEX idx_explanations_capture_id
ON explanations(capture_id);

CREATE TABLE knowledge_items (
  id TEXT PRIMARY KEY,
  normalized_key TEXT NOT NULL,
  surface_text TEXT NOT NULL,
  item_type TEXT NOT NULL,
  language TEXT NOT NULL,
  pos TEXT,
  pronunciation TEXT,
  meaning_ko TEXT,
  description_ko TEXT,
  domain_category TEXT,
  first_seen_at DATETIME NOT NULL,
  last_seen_at DATETIME NOT NULL
);

CREATE UNIQUE INDEX idx_knowledge_items_key_type
ON knowledge_items(normalized_key, item_type);

CREATE TABLE capture_items (
  id TEXT PRIMARY KEY,
  capture_id TEXT NOT NULL,
  knowledge_item_id TEXT NOT NULL,
  role TEXT NOT NULL,
  confidence REAL NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (capture_id) REFERENCES captures(id),
  FOREIGN KEY (knowledge_item_id) REFERENCES knowledge_items(id)
);

CREATE INDEX idx_capture_items_capture_id
ON capture_items(capture_id);

CREATE INDEX idx_capture_items_knowledge_item_id
ON capture_items(knowledge_item_id);

CREATE TABLE learner_items (
  id TEXT PRIMARY KEY,
  knowledge_item_id TEXT NOT NULL,
  familiarity_score REAL NOT NULL DEFAULT 0,
  mastery_score REAL NOT NULL DEFAULT 0,
  ask_count INTEGER NOT NULL DEFAULT 0,
  wrong_count INTEGER NOT NULL DEFAULT 0,
  review_count INTEGER NOT NULL DEFAULT 0,
  last_asked_at DATETIME,
  last_wrong_at DATETIME,
  last_reviewed_at DATETIME,
  status TEXT NOT NULL DEFAULT 'active',
  FOREIGN KEY (knowledge_item_id) REFERENCES knowledge_items(id)
);

CREATE UNIQUE INDEX idx_learner_items_knowledge_item_id
ON learner_items(knowledge_item_id);

CREATE TABLE review_cards (
  id TEXT PRIMARY KEY,
  knowledge_item_id TEXT NOT NULL,
  card_type TEXT NOT NULL,
  question TEXT NOT NULL,
  answer TEXT NOT NULL,
  explanation TEXT,
  state TEXT NOT NULL,
  due_at DATETIME,
  stability REAL NOT NULL DEFAULT 0,
  difficulty REAL NOT NULL DEFAULT 0,
  retrievability REAL,
  reps INTEGER NOT NULL DEFAULT 0,
  lapses INTEGER NOT NULL DEFAULT 0,
  last_review_at DATETIME,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (knowledge_item_id) REFERENCES knowledge_items(id)
);

CREATE INDEX idx_review_cards_due_at
ON review_cards(due_at);

CREATE INDEX idx_review_cards_state_due_at
ON review_cards(state, due_at);

CREATE TABLE review_logs (
  id TEXT PRIMARY KEY,
  review_card_id TEXT NOT NULL,
  source TEXT NOT NULL,
  rating TEXT NOT NULL,
  elapsed_ms INTEGER,
  reviewed_at DATETIME NOT NULL,
  FOREIGN KEY (review_card_id) REFERENCES review_cards(id)
);

CREATE INDEX idx_review_logs_card_reviewed_at
ON review_logs(review_card_id, reviewed_at DESC);

CREATE TABLE reminders (
  id TEXT PRIMARY KEY,
  channel TEXT NOT NULL,
  reminder_type TEXT NOT NULL,
  cron_expr TEXT,
  timezone TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  last_sent_at DATETIME,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE sync_outbox (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  event_id TEXT NOT NULL,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  sent_at DATETIME,
  acked_at DATETIME
);

CREATE UNIQUE INDEX idx_sync_outbox_event_id
ON sync_outbox(event_id);

CREATE INDEX idx_sync_outbox_unsent
ON sync_outbox(acked_at, created_at);
