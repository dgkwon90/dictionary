// desktopd 로컬 HTTP API 클라이언트 골격.
//
// UI ↔ desktopd는 로컬 HTTP로만 통신한다(PRD §15). 화면(#14~#17)에서 쓸 요청을
// 이 얇은 래퍼로 통일한다. 기본 주소는 desktopd 기본값과 동일하며 필요 시 재정의한다.
//
// 요청은 브라우저 fetch가 아니라 @tauri-apps/plugin-http의 fetch를 쓴다 — webview
// origin(tauri://localhost)에서 127.0.0.1로의 요청은 CORS/mixed-content로 막히므로,
// Rust를 경유하는 플러그인 fetch로 우회한다. 그 대신 desktopd에 CORS를 열지 않는다.
//
// desktopd의 모든 /v1/*는 이제 bearer 토큰을 요구한다(review R-01). 셸이 spawn마다
// 새로 생성하는 세션 토큰이라 webview는 빌드타임 상수로 가질 수 없고, `get_api_token`
// Tauri command로 런타임에 가져와 메모리에 캐시한다(main·quicksearch 두 창 모두 각자
// invoke). /healthz는 서버에서도 인증이 면제라 토큰 없이 호출한다.

import { fetch } from "@tauri-apps/plugin-http";
import { invoke } from "@tauri-apps/api/core";

const DEFAULT_BASE_URL = "http://127.0.0.1:48989";

let tokenPromise: Promise<string> | null = null;

function getToken(): Promise<string> {
  tokenPromise ??= invoke<string | null>("get_api_token").then((token) => token ?? "");
  return tokenPromise;
}

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export class DesktopdClient {
  constructor(private readonly baseUrl: string = DEFAULT_BASE_URL) {}

  /** 사이드카 헬스체크(GET /healthz). 인증 불요(서버에서도 면제 엔드포인트). */
  async health(): Promise<boolean> {
    try {
      const res = await fetch(`${this.baseUrl}/healthz`);
      return res.ok;
    } catch {
      return false;
    }
  }

  private async request<T>(path: string, init?: RequestInit): Promise<T> {
    const token = await getToken();
    const res = await fetch(`${this.baseUrl}${path}`, {
      ...init,
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
        ...init?.headers,
      },
    });
    if (!res.ok) {
      throw new ApiError(res.status, `${init?.method ?? "GET"} ${path} → ${res.status}`);
    }
    return (await res.json()) as T;
  }

  /** GET 헬퍼(화면 트랙에서 확장). */
  get<T>(path: string): Promise<T> {
    return this.request<T>(path);
  }

  /** POST 헬퍼(화면 트랙에서 확장). */
  post<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>(path, {
      method: "POST",
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  }

  /** 캡처 생성(POST /v1/captures, PRD §15.1). 생성 직후 비동기로 해석이 시작된다. */
  createCapture(input: CreateCaptureInput): Promise<CreateCaptureResult> {
    return this.post<CreateCaptureResult>("/v1/captures", input);
  }

  /** 한글 발음 → 영어 후보(GET /v1/suggest?q=, backlog #21). */
  suggest(query: string): Promise<SuggestResponse> {
    return this.get<SuggestResponse>(`/v1/suggest?q=${encodeURIComponent(query)}`);
  }

  /** 사용자가 고른 후보를 캐시에 기록(POST /v1/suggest/confirm, #21 Phase2). best-effort —
   * 실패해도 검색 흐름을 막지 않는다(다음 번 같은 발음 검색 시 캐시 적중 기회를 놓칠 뿐). */
  confirmSuggestPick(query: string, english: string, glossKo: string): Promise<{ status: string }> {
    return this.post<{ status: string }>("/v1/suggest/confirm", {
      query,
      english,
      gloss_ko: glossKo,
    });
  }

  /** 해석 스냅샷 조회(GET /v1/captures/{id}/explanation, PRD §15.2). */
  getExplanation(captureId: string): Promise<ExplanationSnapshot> {
    return this.get<ExplanationSnapshot>(
      `/v1/captures/${encodeURIComponent(captureId)}/explanation`,
    );
  }

  /** 팝업이 결과를 이미 보여줬을 때 그 capture의 result_ready 알림을 소비(#18/ADR-0008).
   * best-effort: 폴 루프가 중복 OS 알림을 띄우지 않게 한다. */
  ackCaptureNotification(captureId: string): Promise<{ status: string }> {
    return this.post<{ status: string }>(
      `/v1/captures/${encodeURIComponent(captureId)}/notification-ack`,
    );
  }

  /** 알림 이력(GET /v1/notifications/history, #24). acked 포함, 최신순. 알림 켜짐 여부와 무관. */
  notificationHistory(limit?: number): Promise<NotificationHistoryResponse> {
    const query = limit ? `?limit=${limit}` : "";
    return this.get<NotificationHistoryResponse>(`/v1/notifications/history${query}`);
  }

  /** 특정 알림 확인 처리(POST /v1/notifications/{id}/ack, #24). */
  ackNotification(id: string): Promise<{ status: string }> {
    return this.post<{ status: string }>(`/v1/notifications/${encodeURIComponent(id)}/ack`);
  }

  /** Inbox 목록(GET /v1/inbox, PRD §15.3). status 미지정 시 전체. */
  listInbox(status?: InboxStatus, limit?: number): Promise<InboxListResponse> {
    const params = new URLSearchParams();
    if (status) params.set("status", status);
    if (limit) params.set("limit", String(limit));
    const query = params.toString();
    return this.get<InboxListResponse>(`/v1/inbox${query ? `?${query}` : ""}`);
  }

  /** Inbox 저장(POST /v1/inbox/{id}/save). */
  saveInbox(captureId: string): Promise<InboxStatusResult> {
    return this.post<InboxStatusResult>(`/v1/inbox/${encodeURIComponent(captureId)}/save`);
  }

  /** Inbox 보관(POST /v1/inbox/{id}/archive). */
  archiveInbox(captureId: string): Promise<InboxStatusResult> {
    return this.post<InboxStatusResult>(`/v1/inbox/${encodeURIComponent(captureId)}/archive`);
  }

  /** capture의 추출 단어 목록(GET /v1/captures/{id}/knowledge, #15). */
  listCaptureKnowledge(captureId: string): Promise<CaptureKnowledgeResponse> {
    return this.get<CaptureKnowledgeResponse>(
      `/v1/captures/${encodeURIComponent(captureId)}/knowledge`,
    );
  }

  /** 단어 "모름"(POST /v1/knowledge/{id}/mark-unknown) — 복습 카드 생성. */
  markUnknown(knowledgeItemId: string): Promise<MarkResult> {
    return this.post<MarkResult>(
      `/v1/knowledge/${encodeURIComponent(knowledgeItemId)}/mark-unknown`,
    );
  }

  /** 단어 "알아요"(POST /v1/knowledge/{id}/mark-known). */
  markKnown(knowledgeItemId: string): Promise<MarkResult> {
    return this.post<MarkResult>(
      `/v1/knowledge/${encodeURIComponent(knowledgeItemId)}/mark-known`,
    );
  }

  /** 지금 복습할 due 카드(GET /v1/reviews/due, PRD §15.5). answer/explanation 포함. */
  dueReviews(limit?: number): Promise<ReviewDueResponse> {
    const query = limit ? `?limit=${limit}` : "";
    return this.get<ReviewDueResponse>(`/v1/reviews/due${query}`);
  }

  /** 카드 채점(POST /v1/reviews/{id}/grade, PRD §15.6). elapsed_ms는 카드 표시→채점 경과. */
  gradeReview(cardId: string, rating: ReviewRating, elapsedMs?: number): Promise<GradeResult> {
    return this.post<GradeResult>(`/v1/reviews/${encodeURIComponent(cardId)}/grade`, {
      rating,
      elapsed_ms: elapsedMs ?? 0,
    });
  }

  /**
   * 연습용 카드 목록(GET /v1/practice/cards, #28). due 무시하고 q로 검색.
   * 조회 전용 — 연습은 서버에 아무것도 쓰지 않아 복습 스케줄·mastery에 영향이 없다.
   */
  practiceCards(query?: string, limit?: number): Promise<ReviewDueResponse> {
    const params = new URLSearchParams();
    if (query) params.set("q", query);
    if (limit) params.set("limit", String(limit));
    const qs = params.toString();
    return this.get<ReviewDueResponse>(`/v1/practice/cards${qs ? `?${qs}` : ""}`);
  }

  /** 대시보드 지표(GET /v1/dashboard/summary, PRD §15.7). */
  dashboardSummary(): Promise<DashboardSummary> {
    return this.get<DashboardSummary>("/v1/dashboard/summary");
  }

  /** 설정 조회(GET /v1/settings, PRD §15.8). preferences(편집 가능) + effective(읽기전용 인프라). */
  getSettings(): Promise<SettingsResponse> {
    return this.get<SettingsResponse>("/v1/settings");
  }

  /** 설정 저장(PUT /v1/settings). PUT = 전체 교체이므로 preferences 전 필드를 보낸다. */
  updateSettings(prefs: SettingsPreferences): Promise<SettingsResponse> {
    return this.request<SettingsResponse>("/v1/settings", {
      method: "PUT",
      body: JSON.stringify(prefs),
    });
  }
}

export type InboxStatus = "new" | "saved" | "review_added" | "archived" | "failed";

export interface InboxItem {
  capture_id: string;
  selected_text: string;
  source_app?: string;
  source_type?: string;
  input_mode: string;
  status: InboxStatus;
  job_status: string;
  brief_ko?: string;
  created_at: string;
}

export interface InboxListResponse {
  items: InboxItem[];
}

export interface InboxStatusResult {
  capture_id: string;
  status: string;
}

// learner status: active | known.
export interface CaptureKnowledgeItem {
  knowledge_item_id: string;
  surface_text: string;
  item_type: string;
  pronunciation_ko?: string;
  meaning_ko?: string;
  role: string;
  confidence: number;
  status: string;
  ask_count: number;
  wrong_count: number;
}

export interface CaptureKnowledgeResponse {
  capture_id: string;
  items: CaptureKnowledgeItem[];
}

export interface MarkResult {
  knowledge_item_id: string;
  status: string;
  ask_count: number;
  wrong_count: number;
  candidate_count: number;
  cards_created: number;
}

export type ReviewRating = "again" | "hard" | "good" | "easy";

export interface ReviewCard {
  card_id: string;
  knowledge_item_id: string;
  card_type: string;
  question: string;
  answer: string;
  explanation?: string;
  state: string;
  due_at: string;
}

export interface ReviewDueResponse {
  cards: ReviewCard[];
}

export interface GradeResult {
  card_id: string;
  rating: ReviewRating;
  state: string;
  reps: number;
  due_at: string;
  mastery_score: number;
}

export type InputMode = "clipboard" | "manual" | "pronunciation";

export interface CreateCaptureInput {
  text: string;
  input_mode: InputMode;
  source_app?: string;
  source_type?: string;
}

export interface CreateCaptureResult {
  capture_id: string;
  lookup_job_id: string;
  status: string;
}

// source: "ai"(Gemini 즉시 호출) | "cache"(이전에 확정한 픽 재사용) | "local"(오프라인
// 음소 매칭 폴백, #21 Phase3).
export type SuggestSource = "ai" | "cache" | "local";

export interface SuggestCandidate {
  english: string;
  confidence: number;
  gloss_ko: string;
  source: SuggestSource;
}

export interface SuggestResponse {
  candidates: SuggestCandidate[];
}

export interface Example {
  english: string;
  korean: string;
  note: string;
}

export interface SubItem {
  surface_text: string;
  normalized_key: string;
  item_type: string;
  meaning_ko: string;
  pronunciation_ko: string;
  importance: number;
}

export interface Explanation {
  brief_ko: string;
  detailed_ko: string;
  pronunciation_ko: string;
  domain_category: string;
  difficulty: number;
  examples: Example[];
  sub_items: SubItem[];
}

// status: queued | running | done | failed (lookup_jobs 상태). done일 때만 explanation 존재.
export interface ExplanationSnapshot {
  capture_id: string;
  status: string;
  error_message?: string;
  explanation?: Explanation;
}

export interface WordStat {
  knowledge_item_id: string;
  surface_text: string;
  count: number;
}

export interface CategoryWeakness {
  category: string;
  item_count: number;
  weakness_score: number;
}

export interface DashboardSummary {
  today_search_count: number;
  week_search_count: number;
  today_completed_reviews: number;
  due_card_count: number;
  most_searched: WordStat[];
  most_wrong: WordStat[];
  category_weakness: CategoryWeakness[];
}

// 알림 이력 항목(#24). route는 클릭 시 이동할 메인 탭 라벨, acked는 확인 여부.
export interface NotificationItem {
  id: string;
  kind: string;
  title: string;
  body: string;
  route?: string;
  payload_id?: string;
  created_at: string;
  acked: boolean;
  acked_at?: string;
}

export interface NotificationHistoryResponse {
  notifications: NotificationItem[];
  unacked_count: number;
}

// 편집 가능한 동작 정책(app_settings에 저장). 복습 시간은 "HH:MM" 24h.
export interface SettingsPreferences {
  notifications_enabled: boolean;
  morning_review_time: string;
  evening_review_time: string;
}

// 읽기전용 인프라 config(.env 기반). api_key는 값이 아니라 설정 유무만 노출.
export interface EffectiveConfig {
  addr: string;
  db_path: string;
  ai_provider: string;
  gemini_model: string;
  api_key_configured: boolean;
}

export interface SettingsResponse {
  preferences: SettingsPreferences;
  effective: EffectiveConfig;
}

export const api = new DesktopdClient();
