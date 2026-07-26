-- Neulsang 스키마 v2 (제품 재설계, docs/설계재검토.md).
--
-- 재설계 이전 스키마(0001~0005)를 이 한 파일로 대체한다. 실배포·실사용자가 없고
-- 보존할 학습 데이터(review_cards/review_logs)가 0건이었으므로, 델타 마이그레이션
-- 대신 새로 쓴다 — 바꿔야 할 테이블이 사실상 전부였고, SQLite에서 CHECK 변경·컬럼
-- 삭제는 테이블 재생성을 요구하는데 그 재생성 SQL은 실행자가 아무도 없는 채로
-- 레포에 영구히 남기 때문이다.
--
-- 핵심 모델: 검색(capture) 1건은 **단어 또는 문장**이고, 사용자가 결과를 확인해
-- "학습할래요"(단어) 또는 "모르는 단어 선택 완료"(문장)를 눌러야 학습 대상이 된다.
-- 학습 대상은 단어든 문장이든 모두 knowledge_items 한 테이블에 산다.

CREATE TABLE app_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at DATETIME NOT NULL
);

-- captures: 검색 1건. AI 진행 상태(lookup_jobs.status)와 사용자 판단(triage_state)은
-- 직교하므로 컬럼을 나눈다 — job 상태는 조회 시 파생하고 여기에 저장하지 않는다.
CREATE TABLE captures (
  id TEXT PRIMARY KEY,
  -- 문장 안에서 AI가 뽑지 않은 임의 구간을 사용자가 골랐을 때, 그 구간을 다시
  -- 설명하기 위해 만드는 자식 capture가 부모를 가리킨다.
  parent_capture_id TEXT,
  source_app TEXT,
  source_type TEXT,
  selected_text TEXT NOT NULL,
  detected_lang TEXT,
  input_mode TEXT NOT NULL,
  text_hash TEXT NOT NULL,
  -- input_type은 AI의 원본 분류(word/term/phrase/sentence/error_message)를 그대로
  -- 보존한다. 해석 전에는 NULL. 제품 로직은 이걸 직접 쓰지 않고 learn_kind를 쓴다.
  input_type TEXT,
  -- learn_kind는 서버가 정한 2치 분류다. AI 분류를 참고하되 어긋나면 서버가 이긴다
  -- (explain.LearnKind) — 문장을 phrase로 오분류하면 단어 선택 흐름 전체를 건너뛰어
  -- 제품이 조용히 망가지는데, 그 비용이 반대 방향보다 훨씬 크다.
  learn_kind TEXT CHECK(learn_kind IS NULL OR learn_kind IN ('word', 'sentence')),
  triage_state TEXT NOT NULL DEFAULT 'unseen'
    CHECK(triage_state IN ('unseen', 'needs_selection', 'learning', 'discarded')),
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  -- 단어는 needs_selection을 거치지 않는다(고를 단어가 자기 자신뿐이므로). learn_kind가
  -- 아직 NULL인 상태(해석 전)도 마찬가지로 막는다 — `=`가 아니라 `IS`를 쓰는 이유는
  -- NULL과의 `=` 비교가 NULL이 되고 CHECK는 FALSE일 때만 거부하기 때문이다.
  CHECK(triage_state <> 'needs_selection' OR learn_kind IS 'sentence'),
  FOREIGN KEY (parent_capture_id) REFERENCES captures(id)
);

CREATE INDEX idx_captures_created_at ON captures(created_at DESC);
CREATE INDEX idx_captures_text_hash ON captures(text_hash);
-- 기본 목록이 "미확인만"(triage_state IN ('unseen','needs_selection'))이므로
-- 상태를 선두 컬럼으로 둔다.
CREATE INDEX idx_captures_triage_created ON captures(triage_state, created_at DESC);
CREATE INDEX idx_captures_parent ON captures(parent_capture_id);

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

CREATE INDEX idx_lookup_jobs_capture_created_at
ON lookup_jobs(capture_id, created_at DESC);

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

-- knowledge_items: 학습 대상. 단어와 문장이 **같은 테이블**에 산다 — 그래야
-- review_cards / learner_items / 연습 / 통계 쿼리를 종류별로 갈라 쓰지 않아도 된다.
--
-- normalized_key는 서버가 계산한다(AI가 준 값을 키로 쓰면, 긴 문장의 키를 AI가 잘라
-- 반환했을 때 서로 다른 두 문장이 조용히 병합된다). 규칙은 domain/knowledge.NormalizeKey.
--
-- UNIQUE 키에 item_type이 아니라 learn_kind가 들어가는 이유: item_type은 AI가 5지선다로
-- 고르므로 같은 단어가 한 번은 word, 다음엔 term으로 분류되면 행이 갈라져 "중복 추가
-- 없이 횟수만 누적"이 확률적으로 깨진다. learn_kind는 서버가 정하므로 흔들리지 않는다.
CREATE TABLE knowledge_items (
  id TEXT PRIMARY KEY,
  normalized_key TEXT NOT NULL,
  surface_text TEXT NOT NULL,
  learn_kind TEXT NOT NULL CHECK(learn_kind IN ('word', 'sentence')),
  item_type TEXT,
  language TEXT NOT NULL,
  pronunciation TEXT,
  meaning_ko TEXT,
  -- 문장 안에서 고른 단어를 "다시 한번 설명"할 때 보여주는 본문. meaning_ko(짧은 뜻)와
  -- 구분된다.
  description_ko TEXT,
  domain_category TEXT,
  first_seen_at DATETIME NOT NULL,
  last_seen_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE UNIQUE INDEX idx_knowledge_items_key_kind
ON knowledge_items(normalized_key, learn_kind);

CREATE INDEX idx_knowledge_items_kind_surface
ON knowledge_items(learn_kind, surface_text);

-- capture_items: 캡처와 학습 대상의 연결. 두 종류의 사실을 한 행에 담는다.
--   role/confidence  = AI가 파생시킨 사실 (이 항목이 이 캡처에서 어떻게 나왔나)
--   selected_at      = 사용자가 결정한 사실 (문장에서 모르는 단어로 골랐나)
-- char_start/char_end는 문장 원문에서의 위치다. 같은 단어가 문장에 두 번 나오는 경우를
-- 구분하고, cloze 카드의 빈칸 위치를 만드는 데 쓴다. AI 추출 항목은 NULL일 수 있다.
CREATE TABLE capture_items (
  id TEXT PRIMARY KEY,
  capture_id TEXT NOT NULL,
  knowledge_item_id TEXT NOT NULL,
  role TEXT NOT NULL CHECK(role IN ('sub_item', 'sentence_self')),
  confidence REAL NOT NULL DEFAULT 0,
  char_start INTEGER,
  char_end INTEGER,
  selected_at DATETIME,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE(capture_id, knowledge_item_id, role),
  FOREIGN KEY (capture_id) REFERENCES captures(id),
  FOREIGN KEY (knowledge_item_id) REFERENCES knowledge_items(id)
);

CREATE INDEX idx_capture_items_capture_id
ON capture_items(capture_id);

CREATE INDEX idx_capture_items_knowledge_item_id
ON capture_items(knowledge_item_id);

-- learner_items: 학습 대상 1개당 1행. 사용자가 학습을 확정한 시점에 생긴다
-- (AI가 단어를 추출하는 시점이 아니다 — 그게 재설계의 핵심 경계다).
--
-- 정답률은 컬럼이 아니라 correct_count/attempt_count 계산값이다. 원장과 파생값이
-- 어긋날 여지를 없앤다. attempt_count는 복습과 연습 채점을 합산한다.
--
-- unknown_count는 "사용자가 모른다고 선언한 횟수"다. 이전 스키마의 wrong_count가 같은
-- 값을 담고 있었는데 이름이 "복습 오답 수"로 읽혀 지표를 오염시켰다. 복습 오답 수는
-- attempt_count - correct_count로 나온다.
CREATE TABLE learner_items (
  id TEXT PRIMARY KEY,
  knowledge_item_id TEXT NOT NULL,
  ask_count INTEGER NOT NULL DEFAULT 0,
  unknown_count INTEGER NOT NULL DEFAULT 0,
  attempt_count INTEGER NOT NULL DEFAULT 0,
  correct_count INTEGER NOT NULL DEFAULT 0,
  -- "오늘 등록 / 이번 주 등록" 연습 필터의 기준 시각.
  registered_at DATETIME NOT NULL,
  last_asked_at DATETIME,
  last_unknown_at DATETIME,
  last_graded_at DATETIME,
  status TEXT NOT NULL DEFAULT 'active'
    CHECK(status IN ('active', 'known', 'removed')),
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (knowledge_item_id) REFERENCES knowledge_items(id)
);

CREATE UNIQUE INDEX idx_learner_items_knowledge_item_id
ON learner_items(knowledge_item_id);

CREATE INDEX idx_learner_items_registered_at
ON learner_items(registered_at DESC);

-- review_cards: 복습 카드.
--   knowledge_item_id         = 소유자. 이 카드의 채점 결과가 귀속될 학습 대상.
--   context_knowledge_item_id = 문맥. cloze 카드가 나온 문장.
--
-- cloze(문장 S 안의 단어 W)는 소유자가 W, 문맥이 S다. 소유자를 W로 두는 이유는
-- "고른 단어들도 각각 학습 대상"이고 정답률·약점 랭킹이 학습 대상 단위 지표이기
-- 때문이다. "문장 복습 = 뜻 맞추기 + cloze"는 스키마가 아니라 세션 쿼리로 만든다
-- (knowledge_item_id = S 인 카드 ∪ context_knowledge_item_id = S 인 카드).
--
-- interval_days는 이전 스키마에서 stability라는 이름으로 같은 값을 담고 있었다.
CREATE TABLE review_cards (
  id TEXT PRIMARY KEY,
  knowledge_item_id TEXT NOT NULL,
  context_knowledge_item_id TEXT,
  card_type TEXT NOT NULL,
  question TEXT NOT NULL,
  answer TEXT NOT NULL,
  explanation TEXT,
  state TEXT NOT NULL,
  due_at DATETIME,
  interval_days REAL NOT NULL DEFAULT 0,
  reps INTEGER NOT NULL DEFAULT 0,
  lapses INTEGER NOT NULL DEFAULT 0,
  last_review_at DATETIME,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY (knowledge_item_id) REFERENCES knowledge_items(id),
  FOREIGN KEY (context_knowledge_item_id) REFERENCES knowledge_items(id)
);

-- 카드 하나의 정체성 = (소유자, 종류, 문맥). 같은 단어를 여러 번 검색해도 같은 카드가
-- 두 장 생기지 않는다는 것을 구조로 보장한다(이전 스키마에서는 재검색이 후보를 새로
-- 쌓아 중복 카드가 생기는 버그가 있었다). 삽입은 ON CONFLICT DO NOTHING으로 한다.
CREATE UNIQUE INDEX ux_review_cards_identity
ON review_cards(knowledge_item_id, card_type, COALESCE(context_knowledge_item_id, ''));

CREATE INDEX idx_review_cards_due_at
ON review_cards(due_at);

CREATE INDEX idx_review_cards_state_due_at
ON review_cards(state, due_at);

CREATE INDEX idx_review_cards_context
ON review_cards(context_knowledge_item_id);

-- review_logs: append-only 채점 원장. 정답률의 진실은 여기 있고 learner_items의
-- 카운터는 정렬 가능한 캐시다.
--
-- is_correct: again만 오답이다. hard("떠올렸으나 힘들었다")를 오답으로 치면 정직한
-- 자가보고를 처벌해 사용자가 등급을 부풀리게 된다.
CREATE TABLE review_logs (
  id TEXT PRIMARY KEY,
  review_card_id TEXT NOT NULL,
  source TEXT NOT NULL CHECK(source IN ('review', 'practice')),
  rating TEXT NOT NULL,
  is_correct INTEGER NOT NULL CHECK(is_correct IN (0, 1)),
  elapsed_ms INTEGER,
  reviewed_at DATETIME NOT NULL,
  FOREIGN KEY (review_card_id) REFERENCES review_cards(id)
);

CREATE INDEX idx_review_logs_card_reviewed_at
ON review_logs(review_card_id, reviewed_at DESC);

CREATE INDEX idx_review_logs_reviewed_at
ON review_logs(reviewed_at DESC);

-- review_card_candidates: AI가 제안한 카드 재료. 사용자가 학습을 확정할 때 카드로
-- 승격되고 consumed_at이 찍힌다. 승격 자체의 정확성은 ux_review_cards_identity가
-- 보장하므로, consumed_at은 재스캔을 줄이는 최적화에 가깝다.
CREATE TABLE review_card_candidates (
  id TEXT PRIMARY KEY,
  capture_id TEXT NOT NULL,
  knowledge_item_id TEXT,
  context_knowledge_item_id TEXT,
  card_type TEXT NOT NULL,
  question TEXT NOT NULL,
  answer TEXT NOT NULL,
  explanation TEXT,
  created_at DATETIME NOT NULL,
  consumed_at DATETIME,
  FOREIGN KEY (capture_id) REFERENCES captures(id),
  FOREIGN KEY (knowledge_item_id) REFERENCES knowledge_items(id),
  FOREIGN KEY (context_knowledge_item_id) REFERENCES knowledge_items(id)
);

CREATE INDEX idx_review_card_candidates_knowledge_item_id
ON review_card_candidates(knowledge_item_id);

CREATE INDEX idx_review_card_candidates_capture_id
ON review_card_candidates(capture_id);

CREATE INDEX idx_review_card_candidates_unconsumed
ON review_card_candidates(knowledge_item_id, consumed_at);

-- suggest_cache: 한글 발음 → 영어 확정 픽 캐시. 재검색을 AI 호출 없이 즉시·오프라인
-- 으로 해결한다.
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

-- notifications (ADR-0008): 사이드카→UI 이벤트 원장. dedup_key가 전역 UNIQUE라
-- coalesce와 "ack 후 재발화 방지"를 동시에 충족한다. expires_at으로 재시작 후 지난
-- 알림이 다시 뜨는 것을 막는다.
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

-- sync_outbox: 아직 아무 데도 보내지 않지만(NEULSANG_SYNC_URL 미설정 시 no-op),
-- 학습 데이터를 중앙 서버로 올려 모바일에서 이어 학습하는 후속 계획이 있어 원장을
-- 유지한다. 동기화 대상 테이블이 전부 updated_at을 갖는 것도 같은 이유다 — 나중에
-- "T 이후 바뀐 것만" 질의를 못 하면 전체 동기화로 몰리거나, 정답 없는 백필을 하게 된다.
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
