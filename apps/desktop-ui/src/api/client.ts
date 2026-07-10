// desktopd 로컬 HTTP API 클라이언트 골격.
//
// UI ↔ desktopd는 로컬 HTTP로만 통신한다(PRD §15). 화면(#14~#17)에서 쓸 요청을
// 이 얇은 래퍼로 통일한다. 기본 주소는 desktopd 기본값과 동일하며 필요 시 재정의한다.
//
// 요청은 브라우저 fetch가 아니라 @tauri-apps/plugin-http의 fetch를 쓴다 — webview
// origin(tauri://localhost)에서 127.0.0.1로의 요청은 CORS/mixed-content로 막히므로,
// Rust를 경유하는 플러그인 fetch로 우회한다. 그 대신 desktopd에 CORS를 열지 않는다.

import { fetch } from "@tauri-apps/plugin-http";

const DEFAULT_BASE_URL = "http://127.0.0.1:48989";

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

  /** 사이드카 헬스체크(GET /healthz). */
  async health(): Promise<boolean> {
    try {
      const res = await fetch(`${this.baseUrl}/healthz`);
      return res.ok;
    } catch {
      return false;
    }
  }

  private async request<T>(path: string, init?: RequestInit): Promise<T> {
    const res = await fetch(`${this.baseUrl}${path}`, {
      ...init,
      headers: { "Content-Type": "application/json", ...init?.headers },
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

  /** 해석 스냅샷 조회(GET /v1/captures/{id}/explanation, PRD §15.2). */
  getExplanation(captureId: string): Promise<ExplanationSnapshot> {
    return this.get<ExplanationSnapshot>(
      `/v1/captures/${encodeURIComponent(captureId)}/explanation`,
    );
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

export const api = new DesktopdClient();
