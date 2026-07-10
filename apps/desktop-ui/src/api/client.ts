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
}

export const api = new DesktopdClient();
