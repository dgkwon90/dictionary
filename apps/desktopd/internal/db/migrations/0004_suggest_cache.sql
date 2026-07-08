-- Backlog #21 Phase 2: cache confirmed Korean-phonetic → English picks so repeat
-- queries resolve instantly, offline, and without an AI call. Rows are written when
-- the user confirms a suggestion pick (the highest-trust signal), fixing the
-- AI-only cold-start over time.

CREATE TABLE suggest_cache (
  id TEXT PRIMARY KEY,
  normalized_query TEXT NOT NULL,
  english TEXT NOT NULL,
  gloss_ko TEXT,
  hit_count INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL,
  last_used_at DATETIME NOT NULL
);

CREATE UNIQUE INDEX idx_suggest_cache_query_english
ON suggest_cache(normalized_query, english);

CREATE INDEX idx_suggest_cache_query
ON suggest_cache(normalized_query);
