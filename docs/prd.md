# Neulsang 설계 초안 v0.1

## 0. 프로젝트 한 줄 정의

Neulsang은 macOS, Windows, Linux에서 개발자와 실무자가 업무 중 마주치는 영어 단어, 용어, 문장을 빠르게 해석하고, 그 기록을 기반으로 개인화 복습을 제공하는 로컬 우선 영어 학습 데스크톱 앱이다.

핵심은 단순 번역이 아니다.

핵심은 사용자가 언제, 어떤 문맥에서, 무엇을 반복적으로 몰랐는지 기록하고, 그 기록을 다시 학습으로 연결하는 것이다.

---

# 1. 만드는 이유

## 1.1 문제 상황

사용자는 집에서는 macOS, 회사에서는 Windows 또는 Linux 환경에서 일한다.

업무 중 다음과 같은 상황이 자주 발생한다.

* 영어 문서를 읽는다.
* GitHub issue, pull request, commit message를 본다.
* API 문서, 라이브러리 문서, 에러 로그를 본다.
* 회의 중 영어 용어를 접한다.
* 코드 리뷰 중 모르는 표현을 만난다.
* 개발 관련 블로그, 공식 문서, 튜토리얼을 읽는다.

이때 모르는 단어, 문장, 용어를 찾기 위해 웹브라우저를 열고 검색하는 과정이 불편하다.

현재 문제는 다음과 같다.

1. 브라우저를 열고 검색하는 흐름이 느리다.
2. 드래그, 복사, 붙여넣기, 검색 과정이 번거롭다.
3. 해석만 보고 끝나기 때문에 기억에 남지 않는다.
4. 업무 흐름 때문에 모르는 단어를 깊이 학습하기 어렵다.
5. 같은 단어를 반복해서 검색해도 내가 무엇을 자주 모르는지 알기 어렵다.
6. 여러 OS 환경에서 일하기 때문에 학습 기록이 흩어진다.
7. 나중에 복습하려고 해도 무엇을 복습해야 할지 정리되어 있지 않다.

---

## 1.2 해결하려는 방식

Neulsang은 사용자가 웹브라우저를 직접 열지 않고도 빠르게 영어를 검색하고 학습할 수 있게 한다.

주요 방식은 다음과 같다.

1. 사용자가 문장 또는 단어를 드래그한다.
2. 단축키를 누른다.
3. 앱이 클립보드 또는 선택 텍스트를 읽는다.
4. AI가 단어, 용어, 문장을 해석한다.
5. 결과를 즉시 보여주거나 백그라운드 큐에 저장한다.
6. 검색 결과가 준비되면 시스템 트레이 또는 알림으로 알려준다.
7. 검색한 문장과 단어를 로컬 DB에 저장한다.
8. 반복 검색, 복습 결과, 오답 기록을 기반으로 사용자의 취약 단어를 추론한다.
9. 사용자는 오늘 물어본 단어, 최근 물어본 단어, 특정 기간의 단어를 학습할 수 있다.
10. 매일 또는 매주 정해진 시간에 앱이 복습 문제를 낸다.
11. 초기에는 PC 로컬에만 저장한다.
12. 이후 중앙 서버 동기화와 Google 계정 로그인을 붙인다.
13. 모바일에서는 검색보다 복습에 집중한다.

---

# 2. 제품 방향

## 2.1 제품 포지션

Neulsang은 일반 영어 사전 앱이 아니다.

Neulsang은 다음에 가깝다.

* 개발자를 위한 영어 기억 보조 도구
* 업무 중 영어 문맥을 저장하는 개인 지식 로그
* 반복 검색 기반 취약 단어 학습 도구
* 로컬 우선 영어 학습 시스템
* 데스크톱 중심 AI 영어 보조 앱

---

## 2.2 핵심 가치

### 빠름

브라우저를 열지 않고 단축키로 바로 검색한다.

### 단순함

처음 사용자는 단축키와 트레이만 알면 된다.

### 기록

검색한 단어, 문장, 용어를 모두 로컬에 저장한다.

### 학습

저장된 기록을 복습 카드로 바꾼다.

### 개인화

사용자가 자주 묻고 자주 틀리는 단어를 추론한다.

### 로컬 우선

초기 데이터는 사용자의 PC에 저장된다.

### 확장 가능성

나중에 중앙 서버, Google 로그인, 모바일 복습, 동기화를 추가한다.

---

# 3. 주요 사용자

## 3.1 1차 사용자

백엔드 개발자, 프론트엔드 개발자, DevOps 엔지니어, 인프라 엔지니어, 데이터 엔지니어.

이 사용자는 다음 영어를 자주 접한다.

* 공식 문서
* 에러 메시지
* 코드 주석
* GitHub issue
* commit message
* pull request
* API 문서
* 기술 블로그
* SaaS 관리 페이지
* 클라우드 콘솔
* 영어 회의 자료

---

## 3.2 2차 사용자

개발자가 아니더라도 실무에서 영어 문서를 자주 보는 사람.

예시:

* 기획자
* PM
* QA
* 디자이너
* 해외 자료를 보는 직장인
* 논문 또는 문서를 읽는 학생
* 영어가 약하지만 실무 영어를 자주 접하는 사람

---

# 4. 핵심 용어

## Local-first

로컬 퍼스트.

데이터와 핵심 기능을 서버가 아니라 사용자 PC에 먼저 저장하고 실행하는 방식이다.

Neulsang은 초기에는 서버 없이도 동작해야 한다.

---

## Sidecar

사이드카.

메인 앱 옆에서 함께 실행되는 보조 프로세스다.

Neulsang에서는 Tauri UI 옆에 Go sidecar 프로세스를 실행한다.

---

## System Tray

시스템 트레이.

Windows 우측 하단, macOS 메뉴바, Linux 패널에 상주하는 작은 앱 아이콘이다.

Neulsang은 트레이 앱으로 항상 접근 가능해야 한다.

---

## Global Shortcut

글로벌 쇼트컷.

앱이 포커스되어 있지 않아도 동작하는 전역 단축키다.

예시:

* macOS: `Cmd + Shift + E`
* Windows/Linux: `Ctrl + Shift + E`

---

## Clipboard

클립보드.

사용자가 복사한 텍스트가 임시 저장되는 OS 영역이다.

초기 MVP에서는 선택 텍스트를 직접 읽기보다 사용자가 복사한 텍스트를 읽는 방식을 우선한다.

---

## Inbox / Queue

인박스 / 큐.

검색 요청이 들어오고 결과가 쌓이는 공간이다.

사용자는 나중에 결과를 확인할 수 있다.

---

## Review

리뷰.

저장된 단어, 용어, 문장을 다시 학습하는 과정이다.

---

## Reminder

리마인더.

정해진 시간에 복습을 유도하는 알림이다.

---

## Outbox

아웃박스.

로컬에서 생긴 변경 이벤트를 중앙 서버로 나중에 전송하기 위해 쌓아두는 테이블이다.

---

# 5. MVP 범위

## 5.1 MVP 목표

첫 번째 MVP의 목표는 다음이다.

“사용자가 단축키로 영어 단어 또는 문장을 빠르게 검색하고, AI 설명을 저장하며, 나중에 다시 복습할 수 있게 한다.”

---

## 5.2 MVP에 반드시 포함할 기능

### 1. 데스크톱 앱 실행

* macOS, Windows, Linux에서 실행 가능해야 한다.
* 앱은 시스템 트레이에 상주해야 한다.
* 사용자는 트레이 아이콘으로 앱을 열 수 있어야 한다.

### 2. 글로벌 단축키

* 사용자가 어느 앱을 사용 중이든 단축키로 Neulsang을 호출할 수 있어야 한다.
* 초기 단축키:

  * macOS: `Cmd + Shift + E`
  * Windows/Linux: `Ctrl + Shift + E`

### 3. 빠른 검색

세 가지 입력 방식을 지원한다.

1. 드래그 후 복사한 텍스트 검색
2. 단축키로 검색창을 열고 직접 입력
3. 한글 발음 입력 후 유사 영어 단어 추론

예시:

* `스테일` 입력 → `stale` 후보 제안
* `리졸브` 입력 → `resolve` 후보 제안
* `캐노니컬라이제이션` 입력 → `canonicalization` 후보 제안

### 4. AI 해석

AI는 입력 텍스트에 대해 다음을 제공한다.

* 한 줄 해석
* 쉬운 설명
* 개발 또는 실무 문맥 설명
* 한글 발음
* 핵심 단어 분해
* 예문
* 자주 헷갈리는 표현
* 복습용 카드 후보

### 5. 검색 기록 저장

검색한 원문은 로컬 SQLite에 저장한다.

기록할 항목:

* 원문
* 검색 시각
* 입력 방식
* 출처 앱
* 텍스트 타입
* AI 결과
* 추출된 단어/표현
* 검색 횟수

### 6. Inbox 화면

검색 결과는 Inbox에 쌓인다.

Inbox에서 사용자는 다음을 할 수 있다.

* 새 검색 결과 확인
* 설명 상세 보기
* 복습에 추가
* 알고 있음 표시
* 모름 표시
* 삭제 또는 보관

### 7. 학습 카드 생성

모르는 단어, 자주 검색한 단어, 사용자가 직접 추가한 단어를 학습 카드로 만든다.

카드 유형:

* 뜻 맞추기
* 빈칸 채우기
* 한글 뜻 보고 영어 떠올리기
* 문장 해석하기
* 개발 문맥에서 의미 고르기

### 8. 복습 모드

복습 버튼:

* Again: 전혀 모름
* Hard: 어렵게 맞힘
* Good: 적당히 맞힘
* Easy: 쉽게 맞힘

### 9. 알림

알림은 다음 상황에 표시한다.

* 검색 결과 준비 완료
* 오늘 복습할 카드 있음
* 아침 복습 시간
* 저녁 복습 시간

문장 하나를 검색했을 때 단어별 알림을 여러 개 띄우면 안 된다.

알림은 하나로 묶어야 한다.

예시:

“검색 결과 준비됨: stale, implementation 포함. Inbox에서 확인하세요.”

### 10. 내보내기 / 가져오기

초기에는 서버 동기화 없이 로컬 데이터만 사용한다.

그래서 다음 기능이 필요하다.

* SQLite 백업 파일 내보내기
* JSON 내보내기
* JSON 가져오기
* 중복 데이터 병합

---

## 5.3 MVP에서 제외할 기능

초기 MVP에서는 다음을 제외한다.

* 완전 자동 선택 텍스트 감지
* OCR
* 브라우저 확장
* IDE 플러그인
* 실시간 다중 기기 동기화
* 모바일 앱
* 고급 음성 발음
* 팀 공유
* 조직 관리
* 결제 기능

---

# 6. 장기 목표

## 6.1 중앙 서버 동기화

초기에는 로컬만 사용한다.

이후 중앙 서버를 추가한다.

중앙 서버의 역할:

* 계정 관리
* Google 로그인
* 기기 등록
* 로컬 데이터 백업
* 여러 PC의 데이터 취합
* 모바일 학습 데이터 제공
* 통합 통계 생성
* 리마인더 전송

중앙 서버는 처음부터 실시간 동기화 시스템으로 만들지 않는다.

초기 중앙 서버는 백업과 취합 중심이다.

---

## 6.2 모바일 학습

모바일에서는 영어 검색보다 복습에 집중한다.

모바일 기능:

* 오늘의 단어 5개
* 최근 자주 물어본 단어
* 많이 틀린 단어
* 짧은 퀴즈
* 아침/저녁 리마인드
* 주간 학습률 확인

초기에는 네이티브 앱보다 PWA를 고려한다.

PWA는 Progressive Web App이다.

피더블유에이라고 읽는다.

브라우저 기반이지만 홈 화면에 설치해서 앱처럼 사용할 수 있다.

---

# 7. 기술 스택

## 7.1 데스크톱 앱

추천:

* Tauri 2
* React 또는 Svelte
* TypeScript
* Go sidecar
* SQLite

역할:

* Tauri: UI, 시스템 트레이, 글로벌 단축키, 알림
* Go sidecar: 비즈니스 로직, DB, AI 호출, 복습 스케줄, 동기화
* SQLite: 로컬 저장소

---

## 7.2 백엔드 코어

언어:

* Go

주요 책임:

* 캡처 저장
* AI 요청 처리
* 결과 정규화
* 단어/문장 분해
* 학습 카드 생성
* 복습 일정 계산
* 통계 계산
* 로컬 알림 스케줄 관리
* 중앙 동기화 이벤트 생성

---

## 7.3 로컬 DB

추천:

* SQLite
* WAL 모드
* FTS5 검색

SQLite를 사용하는 이유:

* 로컬 앱에 적합하다.
* 배포가 단순하다.
* 백업이 쉽다.
* SQL로 통계를 만들기 쉽다.
* 중복 제거가 쉽다.
* 파일 하나로 관리할 수 있다.

---

## 7.4 중앙 서버

추후 추천:

* Go API server
* PostgreSQL
* Google OAuth
* JWT 또는 session 기반 인증
* 모바일 PWA

중앙 서버는 MVP 1차에는 만들지 않는다.

다만 로컬 DB와 이벤트 구조는 나중에 중앙 서버로 보내기 쉽게 설계한다.

---

## 7.5 AI Provider

초기에는 다음을 추상화한다.

* OpenAI
* Gemini
* Claude
* Local model

코드에서는 특정 AI provider에 강하게 묶이지 않는다.

인터페이스 예시:

```go
type Explainer interface {
    Explain(ctx context.Context, req ExplainRequest) (*ExplainResult, error)
}
```

---

# 8. 전체 아키텍처

## 8.1 초기 MVP 구조

```text
[User]
  |
  | shortcut / tray
  v
[Tauri Desktop UI]
  |
  | local HTTP or IPC
  v
[Go Sidecar: desktopd]
  |
  +--> SQLite
  +--> AI Provider
  +--> Local Scheduler
  +--> Notification
```

---

## 8.2 장기 구조

```text
[Desktop App - macOS]
        |
[Desktop App - Windows]
        |
[Desktop App - Linux]
        |
        | batch sync
        v
[Central Go API]
        |
        +--> PostgreSQL
        +--> Google OAuth
        +--> Reminder Scheduler
        +--> Mobile PWA
        +--> Notification Channel
```

---

# 9. 주요 사용자 흐름

## 9.1 빠른 검색 흐름

```text
사용자 문장 드래그
-> 복사
-> 단축키 입력
-> Neulsang quick popup 열림
-> 클립보드 텍스트 자동 입력
-> 검색 실행
-> AI 해석 요청
-> 결과 DB 저장
-> Inbox에 결과 추가
-> 알림 표시
```

---

## 9.2 직접 입력 흐름

```text
단축키 입력
-> Quick Search 창 열림
-> 단어/문장 입력
-> Enter
-> AI 해석
-> 결과 저장
-> 상세 화면 표시
```

---

## 9.3 한글 발음 입력 흐름

```text
단축키 입력
-> "스테일" 입력
-> 유사 영어 후보 표시
   - stale
   - style
   - steel
-> 사용자가 stale 선택
-> AI 해석
-> 결과 저장
```

---

## 9.4 Inbox 학습 전환 흐름

```text
Inbox 열기
-> 검색 결과 확인
-> "모름" 버튼 클릭
-> knowledge item 생성
-> review card 생성
-> 오늘 복습 목록에 추가
```

---

## 9.5 복습 흐름

```text
Review 화면 진입
-> 카드 표시
-> 사용자 답변
-> 정답/해설 표시
-> Again/Hard/Good/Easy 선택
-> review log 저장
-> 다음 복습 시간 계산
```

---

## 9.6 리마인더 흐름

```text
설정한 시간 도달
-> due card 확인
-> 알림 표시
-> 사용자가 클릭
-> Review 화면 열림
-> 복습 진행
```

---

# 10. 화면 설계

## 10.1 Tray 메뉴

트레이 클릭 시 메뉴:

* Quick Search
* Inbox
* Today Review
* Dashboard
* Settings
* Quit

---

## 10.2 Quick Search 화면

목적:

빠르게 검색하고 흐름을 깨지 않는다.

구성:

* 입력창
* 클립보드 텍스트 자동 삽입 옵션
* 검색 버튼
* 결과 대기 상태
* “백그라운드로 보내기” 버튼

---

## 10.3 Result Detail 화면

구성:

* 원문
* 한 줄 해석
* 쉬운 설명
* 개발 문맥 설명
* 한글 발음
* 핵심 단어
* 예문
* 관련 표현
* 복습 추가 버튼
* 알고 있음 버튼
* 모름 버튼

---

## 10.4 Inbox 화면

탭:

* New
* Saved
* Review Added
* Archived
* Failed

리스트 항목:

* 원문 일부
* 타입
* 생성 시각
* 핵심 단어
* 상태
* 검색 횟수

---

## 10.5 Review 화면

카드 화면:

* 문제
* 답변 입력 또는 선택지
* 정답 보기
* 해설 보기
* Again / Hard / Good / Easy

---

## 10.6 Dashboard 화면

표시 항목:

* 오늘 검색 횟수
* 이번 주 검색 횟수
* 오늘 복습 카드 수
* 완료한 복습 수
* 가장 많이 검색한 단어
* 가장 자주 틀린 단어
* 카테고리별 약점

  * backend
  * infra
  * database
  * network
  * general
* 최근 7일 학습 추세

---

## 10.7 Settings 화면

설정 항목:

* 단축키
* AI provider
* API key
* 알림 허용
* 아침 복습 시간
* 저녁 복습 시간
* 로컬 DB 경로
* 백업 내보내기
* 백업 가져오기
* 중앙 동기화 사용 여부
* 계정 연결

---

# 11. 로컬 DB 설계

## 11.1 app_settings

앱 설정 저장.

```sql
CREATE TABLE app_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at DATETIME NOT NULL
);
```

---

## 11.2 captures

사용자가 검색한 원문 저장.

```sql
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
  created_at DATETIME NOT NULL
);

CREATE INDEX idx_captures_created_at ON captures(created_at DESC);
CREATE INDEX idx_captures_text_hash ON captures(text_hash);
```

설명:

* `source_app`: Chrome, VSCode, Terminal 등
* `source_type`: browser, ide, terminal, document, manual
* `input_mode`: clipboard, manual, pronunciation
* `text_hash`: 중복 감지용

---

## 11.3 lookup_jobs

AI 해석 작업 상태 저장.

```sql
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
```

상태:

* queued
* running
* done
* failed

---

## 11.4 explanations

AI 해석 결과 저장.

```sql
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
```

---

## 11.5 knowledge_items

단어, 용어, 문장, 표현 단위 저장.

```sql
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
```

`item_type` 예시:

* word
* term
* phrase
* sentence
* error_message

---

## 11.6 capture_items

검색 원문과 추출된 단어의 연결.

```sql
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
```

---

## 11.7 learner_items

사용자가 각 단어를 얼마나 아는지 저장.

```sql
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
```

---

## 11.8 review_cards

복습 카드 저장.

```sql
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
```

카드 타입:

* meaning
* reverse
* cloze
* context
* sentence_translation

---

## 11.9 review_logs

복습 결과 저장.

```sql
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
```

rating:

* again
* hard
* good
* easy

---

## 11.10 reminders

리마인더 설정.

```sql
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
```

---

## 11.11 sync_outbox

추후 중앙 서버 동기화용 이벤트 저장.

```sql
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
```

---

# 12. AI 응답 JSON 계약

AI 응답은 반드시 구조화한다.

자유 텍스트로만 저장하면 나중에 검색, 통계, 복습 카드 생성이 어렵다.

## 12.1 ExplainResult JSON

```json
{
  "input_type": "word | term | phrase | sentence | error_message",
  "detected_language": "en",
  "brief_ko": "짧은 한 줄 해석",
  "detailed_ko": "쉬운 한국어 설명",
  "pronunciation_ko": "한글 발음",
  "domain_category": "backend | frontend | infra | database | network | general",
  "difficulty": 0.42,
  "examples": [
    {
      "english": "This cache entry is stale.",
      "korean": "이 캐시 항목은 오래되어 최신 상태가 아니다.",
      "note": "stale은 개발 문맥에서 오래되어 신뢰하기 어려운 상태를 뜻한다."
    }
  ],
  "sub_items": [
    {
      "surface_text": "stale",
      "normalized_key": "stale",
      "item_type": "word",
      "meaning_ko": "오래된, 최신이 아닌",
      "pronunciation_ko": "스테일",
      "importance": 0.9
    }
  ],
  "review_card_candidates": [
    {
      "card_type": "meaning",
      "question": "stale의 개발 문맥상 의미는?",
      "answer": "오래되어 최신이 아니거나 신뢰하기 어려운 상태",
      "explanation": "stale cache, stale data처럼 자주 사용된다."
    }
  ]
}
```

---

## 12.2 AI 프롬프트 기본 방향

AI에게 요구할 것:

* 한국어로 설명한다.
* 영어 철자는 유지한다.
* 한글 발음을 제공한다.
* 개발 문맥이면 개발 문맥을 우선한다.
* 어려운 문법 설명보다 실무 이해를 우선한다.
* JSON schema를 반드시 지킨다.
* 모르면 모른다고 표시한다.
* 과도한 설명을 피하고 짧고 명확하게 답한다.

---

# 13. 복습 알고리즘

초기에는 FSRS-lite 방식으로 구현한다.

FSRS는 Free Spaced Repetition Scheduler이다.

에프에스알에스라고 읽는다.

처음부터 완전한 알고리즘을 구현하지 않아도 된다.

## 13.1 기본 복습 간격

처음 생성된 카드:

* Again: 10분 뒤
* Hard: 1일 뒤
* Good: 3일 뒤
* Easy: 7일 뒤

이후:

* Again: 간격 초기화
* Hard: 기존 간격 × 1.2
* Good: 기존 간격 × 2.5
* Easy: 기존 간격 × 4.0

---

## 13.2 mastery_score 계산

초기 단순 계산:

```text
mastery_score =
  good_count * 0.2
+ easy_count * 0.3
- again_count * 0.4
- hard_count * 0.1
```

0.0 ~ 1.0 사이로 clamp한다.

---

## 13.3 weakness_score 계산

```text
weakness_score =
  ask_count * 0.2
+ wrong_count * 0.5
+ recent_repeat_bonus
- mastery_score * 0.7
```

활용:

* 오늘 복습 카드 정렬
* 취약 단어 TOP 10
* 모바일 오늘의 단어 선정
* 주간 리포트

---

# 14. Go 패키지 구조

## 14.1 모노레포 구조

```text
devrecall/
├─ apps/
│  ├─ desktop-ui/
│  ├─ desktopd/
│  └─ api/
├─ docs/
│  ├─ prd.md
│  ├─ architecture.md
│  ├─ schema-local.md
│  ├─ schema-central.md
│  ├─ sync-protocol.md
│  ├─ prompt-contracts.md
│  ├─ review-algorithm.md
│  └─ tasks.md
├─ deploy/
├─ scripts/
├─ go.work
└─ README.md
```

---

## 14.2 desktopd 구조

```text
apps/desktopd/
├─ cmd/
│  └─ desktopd/
│     └─ main.go
├─ internal/
│  ├─ app/
│  │  ├─ bootstrap/
│  │  └─ lifecycle/
│  ├─ config/
│  ├─ logger/
│  ├─ db/
│  │  ├─ sqlite/
│  │  └─ migrations/
│  ├─ domain/
│  │  ├─ capture/
│  │  ├─ explain/
│  │  ├─ knowledge/
│  │  ├─ review/
│  │  ├─ reminder/
│  │  ├─ stats/
│  │  └─ sync/
│  ├─ infra/
│  │  ├─ llm/
│  │  ├─ clipboard/
│  │  ├─ notifier/
│  │  ├─ scheduler/
│  │  ├─ outbox/
│  │  └─ device/
│  └─ transport/
│     └─ http/
│        ├─ router.go
│        └─ handlers/
└─ go.mod
```

---

## 14.3 domain 책임

### capture

* 원문 저장
* 중복 검색 감지
* text_hash 생성
* capture event 생성

### explain

* AI 요청 생성
* AI 응답 검증
* explanation 저장
* review card candidate 생성

### knowledge

* 단어/용어 정규화
* knowledge item upsert
* learner item 업데이트
* 검색 횟수 증가

### review

* 카드 생성
* due card 조회
* 복습 결과 저장
* 다음 복습 시각 계산
* mastery score 갱신

### reminder

* 아침/저녁 알림 설정
* due card 여부 확인
* 알림 이벤트 생성

### stats

* 오늘 검색 수
* 주간 검색 수
* 많이 물어본 단어
* 많이 틀린 단어
* 카테고리별 약점

### sync

* outbox event 생성
* 추후 중앙 서버 push/pull 담당

---

# 15. 로컬 API 설계

Tauri UI는 Go sidecar에 local API로 요청한다.

## 15.1 Capture API

```http
POST /v1/captures
```

Request:

```json
{
  "text": "This implementation is stale.",
  "input_mode": "clipboard",
  "source_app": "VSCode",
  "source_type": "ide"
}
```

Response:

```json
{
  "capture_id": "cap_123",
  "lookup_job_id": "job_123",
  "status": "queued"
}
```

---

## 15.2 Lookup Result API

```http
GET /v1/captures/{capture_id}/explanation
```

---

## 15.3 Inbox API

```http
GET /v1/inbox?status=new&limit=50
```

---

## 15.4 Mark Unknown API

```http
POST /v1/knowledge/{item_id}/mark-unknown
```

---

## 15.5 Review Start API

```http
POST /v1/reviews/session/start
```

Response:

```json
{
  "cards": [
    {
      "card_id": "card_123",
      "knowledge_item_id": "know_123",
      "card_type": "meaning",
      "question": "stale의 개발 문맥상 의미는?",
      "answer": "신선하지 않은 / 오래된",
      "explanation": "캐시·데이터가 최신이 아님",
      "state": "new",
      "due_at": "2026-07-11T00:00:00Z"
    }
  ]
}
```

> **계약 참고(#16)**: `answer`/`explanation`은 자가 채점 복습을 위해 due 응답에 포함한다.
> Neulsang은 로컬 단일 사용자 앱이라 답 노출로 인한 유출/치팅 개념이 없고, UI가 "답 보기"
> 전까지 표시만 숨긴다(별도 reveal 왕복을 두지 않음). `GET /v1/reviews/due`도 동일 스키마.

---

## 15.6 Review Grade API

```http
POST /v1/reviews/{card_id}/grade
```

Request:

```json
{
  "rating": "good",
  "elapsed_ms": 3200
}
```

---

## 15.7 Dashboard API

```http
GET /v1/dashboard/summary
```

---

## 15.8 Settings API

```http
GET /v1/settings
PUT /v1/settings
```

응답은 두 계층으로 나뉜다(#17, ADR-0004 부록). `preferences`는 편집 가능한 동작
정책으로 `app_settings`에 저장되고, `effective`는 `.env`로 결정되는 부트스트랩 설정의
읽기전용 반영이다. **API key는 값이 아니라 설정 유무(`api_key_configured`)만 노출한다.**

```json
{
  "preferences": {
    "notifications_enabled": true,
    "morning_review_time": "09:00",
    "evening_review_time": "21:00"
  },
  "effective": {
    "addr": "127.0.0.1:48989",
    "db_path": "/path/to/neulsang.db",
    "ai_provider": "gemini",
    "gemini_model": "gemini-flash-lite-latest",
    "api_key_configured": true
  }
}
```

- `PUT`는 전체 교체(full replace)로 `preferences` 세 필드를 모두 받는다. 복습 시간은
  `HH:MM`(24h)이어야 하며 형식이 틀리면 `400`(저장 안 함).
- 복습 시간은 저장만 되고 실제 알림 스케줄링은 #18에서 소비한다.

---

# 16. 중앙 서버 장기 설계

## 16.1 중앙 서버 목적

중앙 서버는 초기 MVP 이후 추가한다.

목적:

* Google 계정 로그인
* device 등록
* 로컬 DB 백업
* 여러 PC 기록 취합
* 모바일 복습 제공
* 리마인더 전송
* 주간 리포트 생성

---

## 16.2 중앙 서버에서 하지 않을 것

초기 중앙 서버는 다음을 하지 않는다.

* 실시간 협업
* 실시간 동기화
* 복잡한 충돌 해결
* 팀 관리
* 조직 단위 관리

---

## 16.3 중앙 서버 주요 테이블

```sql
CREATE TABLE users (
  id UUID PRIMARY KEY,
  email TEXT UNIQUE,
  name TEXT,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE devices (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id),
  device_name TEXT,
  platform TEXT NOT NULL,
  app_version TEXT,
  last_seen_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE sync_events (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id),
  device_id UUID NOT NULL REFERENCES devices(id),
  seq_no BIGINT NOT NULL,
  event_type TEXT NOT NULL,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  payload_json JSONB NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  received_at TIMESTAMPTZ NOT NULL,
  UNIQUE(device_id, seq_no)
);

CREATE TABLE user_items (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id),
  normalized_key TEXT NOT NULL,
  surface_text TEXT NOT NULL,
  item_type TEXT NOT NULL,
  language TEXT NOT NULL,
  domain_category TEXT,
  ask_count INTEGER NOT NULL DEFAULT 0,
  wrong_count INTEGER NOT NULL DEFAULT 0,
  review_count INTEGER NOT NULL DEFAULT 0,
  mastery_score REAL NOT NULL DEFAULT 0,
  weakness_score REAL NOT NULL DEFAULT 0,
  first_seen_at TIMESTAMPTZ NOT NULL,
  last_seen_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE(user_id, normalized_key, item_type)
);
```

---

# 17. 동기화 설계

## 17.1 기본 원칙

* 로컬이 1차 저장소다.
* 중앙은 백업과 취합 저장소다.
* 이벤트는 append-only로 저장한다.
* 상태는 이벤트를 기반으로 재계산 가능해야 한다.
* 실시간 동기화는 나중에 고려한다.

---

## 17.2 로컬 이벤트 예시

* capture_created
* lookup_completed
* knowledge_item_upserted
* learner_item_updated
* review_card_created
* review_completed
* reminder_updated
* item_archived

---

## 17.3 충돌 규칙

초기 규칙:

* capture는 충돌 없음
* review_log는 충돌 없음
* knowledge_item은 normalized_key 기준 병합
* learner_item은 count 누적
* mastery_score는 최신 review_log 기반 재계산
* settings는 updated_at 최신값 우선

---

# 18. AI Agent 개발 규칙

Codex 또는 Gemini agent는 아래 규칙을 따라야 한다.

## 18.1 공통 규칙

* 한 번에 큰 기능을 구현하지 않는다.
* DB migration → repository → service → handler → test 순서로 구현한다.
* domain 패키지는 외부 의존성을 직접 알면 안 된다.
* AI provider는 interface 뒤에 숨긴다.
* SQLite schema 변경은 migration 파일로만 한다.
* 모든 시간은 UTC로 저장한다.
* 화면 표시 시 로컬 timezone으로 변환한다.
* ID는 UUID 또는 ULID를 사용한다.
* 실패 가능한 작업은 로그를 남긴다.
* AI 응답 JSON은 반드시 validation한다.
* raw AI response도 저장한다.
* 사용자의 API key는 평문 저장을 피한다.

---

## 18.2 AI Agent 작업 단위

### Task 01. 프로젝트 초기화

목표:

* monorepo 생성
* go.work 생성
* desktopd 기본 서버 실행
* health check API 제공

완료 조건:

* `GET /healthz` 호출 시 OK 반환
* 로컬에서 desktopd 실행 가능

---

### Task 02. SQLite migration

목표:

* 로컬 DB schema 생성
* migration 도구 적용
* WAL mode 설정

완료 조건:

* 앱 시작 시 DB 자동 생성
* migration 적용 가능
* 기본 테이블 생성 확인

---

### Task 03. Capture 저장 기능

목표:

* `POST /v1/captures` 구현
* capture 저장
* lookup_job queued 생성
* sync_outbox event 생성

완료 조건:

* 같은 텍스트를 여러 번 검색해도 기록이 저장된다.
* text_hash로 중복 여부를 알 수 있다.
* ask_count 증가 준비가 된다.

---

### Task 04. AI Explain pipeline

목표:

* LLM interface 생성
* mock provider 구현
* OpenAI/Gemini provider 확장 가능 구조
* AI 결과를 explanations에 저장

완료 조건:

* mock AI 결과로 explanation 저장 가능
* JSON schema validation 가능
* 실패 시 lookup_jobs.status = failed

---

### Task 05. Knowledge item 추출

목표:

* AI sub_items를 knowledge_items로 upsert
* capture_items 연결
* learner_items 생성 또는 업데이트

완료 조건:

* 같은 단어는 하나의 knowledge_item으로 병합된다.
* ask_count가 증가한다.
* first_seen_at, last_seen_at이 갱신된다.

---

### Task 06. Review card 생성

목표:

* review_card_candidates 기반 카드 생성
* 모름 표시 시 카드 생성
* due_at 설정

완료 조건:

* 특정 단어를 “모름” 처리하면 review_cards가 생성된다.
* due card 조회가 가능하다.

---

### Task 07. Review session

목표:

* due card 목록 조회
* 사용자의 rating 저장
* 다음 due_at 계산
* learner_items mastery_score 갱신

완료 조건:

* Again/Hard/Good/Easy 처리 가능
* review_logs append-only 저장
* review_count 증가

---

### Task 08. Dashboard summary

목표:

* 오늘 검색 수
* 이번 주 검색 수
* 자주 검색한 단어
* 많이 틀린 단어
* due card 수 조회

완료 조건:

* `GET /v1/dashboard/summary`로 요약 반환

---

### Task 09. Tauri UI 기본

목표:

* 트레이 앱 실행
* Quick Search 화면
* Inbox 화면
* Result Detail 화면
* Review 화면
* Settings 화면

완료 조건:

* 사용자가 단축키로 Quick Search를 열 수 있다.
* 검색 결과를 Inbox에서 볼 수 있다.

---

### Task 10. 알림과 리마인더

목표:

* 검색 결과 완료 알림
* due card 알림
* 아침/저녁 알림 설정

완료 조건:

* 설정된 시간에 알림이 표시된다.
* 알림 클릭 시 Review 화면으로 이동한다.

---

### Task 11. 백업 / 복원

목표:

* JSON export
* JSON import
* SQLite 파일 백업
* 중복 병합

완료 조건:

* 내보낸 JSON을 다시 가져올 수 있다.
* 같은 knowledge_item은 중복 생성되지 않는다.

---

### Task 12. 중앙 동기화 준비

목표:

* sync_outbox event 생성
* push API client skeleton
* sync 상태 저장

완료 조건:

* 중앙 서버가 없어도 로컬 기능은 정상 동작한다.
* 나중에 API URL만 붙이면 outbox 전송 가능하다.

---

# 19. 첫 번째 MVP 완료 기준

MVP는 다음이 가능하면 완료로 본다.

1. 앱이 트레이에 상주한다.
2. 단축키로 Quick Search를 열 수 있다.
3. 클립보드의 영어 문장 또는 단어를 검색할 수 있다.
4. AI 설명 결과가 저장된다.
5. Inbox에서 과거 검색 결과를 볼 수 있다.
6. 검색 결과에서 핵심 단어가 추출된다.
7. 사용자가 “모름”을 누르면 복습 카드가 생성된다.
8. Review 화면에서 문제를 풀 수 있다.
9. Again/Hard/Good/Easy 결과가 저장된다.
10. 오늘 복습할 카드 수를 볼 수 있다.
11. 아침/저녁 알림을 설정할 수 있다.
12. 로컬 DB를 내보낼 수 있다.

---

# 20. 제품 이름 후보

## 20.1 Neulsang

의미:

개발 중 만난 영어를 다시 떠올리게 해주는 도구.

장점:

* 개발자 대상이 명확하다.
* Recall이 복습과 기억을 의미한다.
* 이름이 짧다.

## 20.2 WordQueue

의미:

검색한 단어와 문장을 큐에 쌓아두고 나중에 학습한다.

장점:

* 큐 기반 UX와 잘 맞는다.

## 20.3 CodeVoca

의미:

코드와 개발 문맥의 vocabulary.

장점:

* 직관적이다.
* 다만 개발자 외 사용자에게는 범위가 좁아 보일 수 있다.

## 20.4 RecallBox

의미:

모르는 표현을 담아두고 다시 기억하게 하는 박스.

장점:

* 개발자 외 실무자에게도 확장 가능하다.

현재 추천 이름:

Neulsang

---

# 21. 개발 철학

이 앱은 번역 앱이 아니다.

이 앱은 기억 앱이다.

번역 결과는 시간이 지나면 사라지는 정보다.

하지만 사용자가 언제, 어떤 문맥에서, 무엇을 반복해서 몰랐는지는 시간이 지날수록 가치가 커지는 데이터다.

따라서 모든 설계는 다음 질문을 기준으로 판단한다.

“이 기능이 사용자의 영어 기억을 더 잘 쌓고, 다시 떠올리게 하는가?”

그렇다면 넣는다.

아니라면 MVP에서는 뺀다.

---

# 22. 우선 개발 순서 요약

## Phase 1

* desktopd 실행
* SQLite schema
* capture 저장
* AI mock explain
* Inbox API

## Phase 2

* 실제 AI provider 연결
* knowledge item 추출
* learner item 업데이트
* result detail UI

## Phase 3

* review card 생성
* review session
* mastery/weakness score
* dashboard

## Phase 4

* Tauri tray
* global shortcut
* notification
* reminder

## Phase 5

* export/import
* sync_outbox
* 중앙 서버 skeleton

---

# 23. 핵심 리스크

## 23.1 OS별 선택 텍스트 읽기

macOS, Windows, Linux에서 선택 텍스트를 직접 읽는 것은 복잡하다.

초기에는 다음 방식으로 간다.

1. 사용자가 텍스트를 선택한다.
2. 사용자가 복사한다.
3. Neulsang 단축키를 누른다.
4. 앱이 클립보드를 읽는다.

자동 선택 텍스트 감지는 추후 기능으로 둔다.

---

## 23.2 알림 과다

문장 하나에서 단어 여러 개가 추출되어도 알림은 하나만 띄운다.

세부 내용은 Inbox에서 본다.

---

## 23.3 AI 비용

AI 호출 전 캐시를 확인한다.

순서:

1. text_hash로 기존 explanation 확인
2. normalized_key로 기존 knowledge_item 확인
3. 캐시가 없을 때만 AI 호출
4. 배치 요약은 나중에 중앙 서버에서 처리

---

## 23.4 AI 응답 품질

AI 응답은 반드시 JSON schema로 제한한다.

응답 검증 실패 시 재시도하거나 failed 상태로 저장한다.

---

## 23.5 너무 큰 범위

초기 MVP에서 모바일, 중앙 서버, 브라우저 확장, IDE 플러그인을 동시에 만들지 않는다.

먼저 로컬 데스크톱 앱을 완성한다.

---

# 24. AI Agent에게 주는 첫 명령 예시

아래 문장을 Codex 또는 Gemini에게 첫 작업으로 전달한다.

```text
You are building Neulsang, a local-first desktop English learning assistant for developers and professionals.

Read docs/prd.md first.

Implement Task 01 only.

Goal:
- Create a Go desktop sidecar app under apps/desktopd.
- Add a health check HTTP server.
- Add config loading.
- Add structured logging.
- Prepare SQLite connection structure, but do not implement migrations yet.
- Do not implement AI features yet.
- Keep domain, infra, transport boundaries clean.

Completion criteria:
- `go run ./cmd/desktopd` starts the server.
- `GET /healthz` returns 200 OK.
- The project compiles.
- Include basic tests where reasonable.
```

---

# 25. 최종 요약

Neulsang의 초기 목표는 거창한 AI 영어 선생님이 아니다.

초기 목표는 다음이다.

“업무 중 모르는 영어를 빠르게 저장하고, AI가 설명해주고, 그 기록을 다시 복습하게 만드는 로컬 우선 데스크톱 앱.”

이 목표가 완성되면 이후 확장은 자연스럽다.

1. 로컬 검색
2. 기록 저장
3. 복습 카드
4. 알림
5. 백업
6. 중앙 동기화
7. 모바일 복습
8. 개인화 리포트

Neulsang의 진짜 자산은 단어장이 아니다.

Neulsang의 진짜 자산은 사용자가 실제 업무 중 마주친 영어 이해 로그다.
