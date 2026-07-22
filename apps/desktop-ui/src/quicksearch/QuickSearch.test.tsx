// QuickSearch의 한글 발음 → 영어 후보 흐름(backlog #21/RW-08) 컴포넌트 테스트.
//
// Tauri API(window/event/clipboard-manager)는 jsdom에 없으므로 전부 mock한다.
// api 클라이언트도 mock해 네트워크 없이 상태 전이만 검증한다.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("@tauri-apps/api/window", () => ({
  getCurrentWindow: () => ({
    hide: vi.fn().mockResolvedValue(undefined),
    onFocusChanged: vi.fn().mockImplementation(() => Promise.resolve(() => {})),
  }),
}));

vi.mock("@tauri-apps/api/event", () => ({
  listen: vi.fn().mockImplementation(() => Promise.resolve(() => {})),
}));

vi.mock("@tauri-apps/plugin-clipboard-manager", () => ({
  readText: vi.fn().mockResolvedValue(""),
}));

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      suggest: vi.fn(),
      confirmSuggestPick: vi.fn().mockResolvedValue({ status: "ok" }),
      createCapture: vi.fn(),
      getExplanation: vi.fn(),
      ackCaptureNotification: vi.fn().mockResolvedValue({ status: "ok" }),
    },
  };
});

import { api } from "../api/client";
import QuickSearch from "./QuickSearch";

const doneExplanation = {
  brief_ko: "짧은 설명",
  detailed_ko: "자세한 설명",
  pronunciation_ko: "steil",
  domain_category: "general",
  difficulty: 0.3,
  examples: [],
  sub_items: [],
};

function mockSuccessfulCapture(captureId = "cap-1") {
  vi.mocked(api.createCapture).mockResolvedValue({
    capture_id: captureId,
    lookup_job_id: "job-1",
    status: "queued",
  });
  vi.mocked(api.getExplanation).mockResolvedValue({
    capture_id: captureId,
    status: "done",
    explanation: doneExplanation,
  });
}

async function typeAndSubmit(text: string) {
  const user = userEvent.setup();
  const input = screen.getByRole("textbox");
  await user.clear(input);
  await user.type(input, text);
  await user.keyboard("{Enter}");
}

beforeEach(() => {
  vi.mocked(api.suggest).mockReset();
  vi.mocked(api.createCapture).mockReset();
  vi.mocked(api.getExplanation).mockReset();
  vi.mocked(api.confirmSuggestPick).mockClear();
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("QuickSearch 한글 발음 후보", () => {
  it("AI 후보를 보여주고, 선택하면 confirm 후 그 영어 표현으로 검색한다", async () => {
    vi.mocked(api.suggest).mockResolvedValue({
      candidates: [{ english: "stale", confidence: 0.9, gloss_ko: "오래된", source: "ai" }],
    });
    mockSuccessfulCapture();
    render(<QuickSearch />);

    await typeAndSubmit("스테일");

    await waitFor(() => expect(screen.getByText("stale")).toBeInTheDocument());
    expect(screen.getByText(/오래된/)).toBeInTheDocument();
    expect(screen.getByText(/AI/)).toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(screen.getByText("stale"));

    await waitFor(() =>
      expect(api.confirmSuggestPick).toHaveBeenCalledWith("스테일", "stale", "오래된"),
    );
    await waitFor(() =>
      expect(api.createCapture).toHaveBeenCalledWith({ text: "stale", input_mode: "manual" }),
    );
    await waitFor(() => expect(screen.getByText("짧은 설명")).toBeInTheDocument());
  });

  it("캐시 출처 후보는 '이전 선택'으로 표시된다", async () => {
    vi.mocked(api.suggest).mockResolvedValue({
      candidates: [{ english: "mutex", confidence: 1, gloss_ko: "상호 배제", source: "cache" }],
    });
    render(<QuickSearch />);

    await typeAndSubmit("뮤텍스");

    await waitFor(() => expect(screen.getByText(/이전 선택/)).toBeInTheDocument());
  });

  it("로컬 폴백 출처 후보는 '오프라인'으로 표시된다", async () => {
    vi.mocked(api.suggest).mockResolvedValue({
      candidates: [{ english: "idempotent", confidence: 0.8, gloss_ko: "멱등의", source: "local" }],
    });
    render(<QuickSearch />);

    await typeAndSubmit("이디엠포턴트");

    await waitFor(() => expect(screen.getByText(/오프라인/)).toBeInTheDocument());
  });

  it("후보가 비어 있으면 원문을 그대로 검색한다", async () => {
    vi.mocked(api.suggest).mockResolvedValue({ candidates: [] });
    mockSuccessfulCapture();
    render(<QuickSearch />);

    await typeAndSubmit("알수없는말");

    await waitFor(() =>
      expect(api.createCapture).toHaveBeenCalledWith({
        text: "알수없는말",
        input_mode: "manual",
      }),
    );
    await waitFor(() => expect(screen.getByText("짧은 설명")).toBeInTheDocument());
  });

  it("suggest 자체가 실패해도 원문 검색으로 복구한다", async () => {
    vi.mocked(api.suggest).mockRejectedValue(new Error("network down"));
    mockSuccessfulCapture();
    render(<QuickSearch />);

    await typeAndSubmit("스테일");

    await waitFor(() =>
      expect(api.createCapture).toHaveBeenCalledWith({ text: "스테일", input_mode: "manual" }),
    );
  });

  it("취소하면 입력 화면으로 돌아가고 capture를 만들지 않는다", async () => {
    vi.mocked(api.suggest).mockResolvedValue({
      candidates: [{ english: "stale", confidence: 0.9, gloss_ko: "오래된", source: "ai" }],
    });
    render(<QuickSearch />);

    await typeAndSubmit("스테일");
    await waitFor(() => expect(screen.getByText("stale")).toBeInTheDocument());

    const user = userEvent.setup();
    await user.click(screen.getByText("취소"));

    await waitFor(() => expect(screen.getByText(/Enter로 검색/)).toBeInTheDocument());
    expect(api.createCapture).not.toHaveBeenCalled();
  });

  it("영문/문장 입력은 후보 없이 곧바로 검색한다", async () => {
    mockSuccessfulCapture();
    render(<QuickSearch />);

    await typeAndSubmit("hello world");

    await waitFor(() =>
      expect(api.createCapture).toHaveBeenCalledWith({
        text: "hello world",
        input_mode: "manual",
      }),
    );
    expect(api.suggest).not.toHaveBeenCalled();
  });
});
