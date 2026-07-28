// 검색 기록 화면 컴포넌트 테스트.
//
// api 클라이언트를 mock해 네트워크 없이 검증한다. 여기서 보는 것은 이 화면이 존재하는
// 이유 그대로다: 검색한 것이 목록에 나오고, 단어는 한 번에 담을 수 있고, 문장은 단어를
// 고르기 전에는 담을 수 없다.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      listSearches: vi.fn(),
      getSearch: vi.fn(),
      openSearch: vi.fn(),
      learnSearch: vi.fn(),
      discardSearch: vi.fn(),
      selectWord: vi.fn(),
      deselectWord: vi.fn(),
      completeSelection: vi.fn(),
      setSearchKind: vi.fn(),
    },
  };
});

import { api, type SearchDetail, type SearchItem } from "../api/client";
import SearchHistory from "./SearchHistory";

const listSearches = vi.mocked(api.listSearches);
const getSearch = vi.mocked(api.getSearch);
const openSearch = vi.mocked(api.openSearch);
const learnSearch = vi.mocked(api.learnSearch);
const completeSelection = vi.mocked(api.completeSelection);
const selectWord = vi.mocked(api.selectWord);

function item(overrides: Partial<SearchItem> = {}): SearchItem {
  return {
    capture_id: "cap-1",
    selected_text: "idempotent",
    input_mode: "manual",
    learn_kind: "word",
    triage_state: "unseen",
    job_status: "done",
    brief_ko: "여러 번 해도 결과가 같은",
    created_at: "2026-07-28T00:00:00Z",
    ...overrides,
  };
}

function detail(overrides: Partial<SearchDetail> = {}): SearchDetail {
  return {
    capture_id: "cap-1",
    selected_text: "idempotent",
    learn_kind: "word",
    triage_state: "unseen",
    job_status: "done",
    brief_ko: "여러 번 해도 결과가 같은",
    created_at: "2026-07-28T00:00:00Z",
    items: [],
    ...overrides,
  };
}

beforeEach(() => {
  for (const fn of Object.values(api)) vi.mocked(fn).mockReset();
  listSearches.mockResolvedValue({ items: [] });
  openSearch.mockResolvedValue({
    capture_id: "cap-1",
    triage_state: "needs_selection",
    learning_item_ids: [],
    cards_created: 0,
  });
});

describe("검색 기록", () => {
  it("찾아본 것이 목록에 나온다", async () => {
    listSearches.mockResolvedValue({ items: [item()] });
    render(<SearchHistory />);

    expect(await screen.findByText("idempotent")).toBeInTheDocument();
    // 기본은 미확인 — 화면이 열리자마자 답해야 할 것부터 보여준다.
    expect(listSearches).toHaveBeenCalledWith({ view: "unresolved", kind: undefined });
  });

  it("단어를 고르면 학습에 담을 수 있다", async () => {
    listSearches.mockResolvedValue({ items: [item()] });
    getSearch.mockResolvedValue(detail());
    learnSearch.mockResolvedValue({
      capture_id: "cap-1",
      triage_state: "learning",
      learning_item_ids: ["k1"],
      cards_created: 2,
    });
    render(<SearchHistory />);

    await userEvent.click(await screen.findByText("idempotent"));
    await userEvent.click(await screen.findByRole("button", { name: "학습할래요" }));

    await waitFor(() => expect(learnSearch).toHaveBeenCalledWith("cap-1"));
    expect(await screen.findByText(/복습 카드 2장/)).toBeInTheDocument();
  });

  // 문장은 "학습할래요"로 곧장 담을 수 없다: 무엇을 몰랐는지 고르지 않으면 빈칸 카드를
  // 만들 수 없고, 그러면 학습 목록에 복습할 것 없는 문장만 남는다.
  it("문장은 단어를 고르는 화면으로 간다", async () => {
    listSearches.mockResolvedValue({
      items: [item({ learn_kind: "sentence", selected_text: "the bread went stale" })],
    });
    getSearch.mockResolvedValue(
      detail({
        learn_kind: "sentence",
        selected_text: "the bread went stale",
        triage_state: "needs_selection",
        items: [
          {
            knowledge_item_id: "k-stale",
            surface_text: "stale",
            meaning_ko: "오래된",
            char_start: 15,
            char_end: 20,
            selected: false,
          },
        ],
      }),
    );
    render(<SearchHistory />);

    await userEvent.click(await screen.findByText("the bread went stale"));

    expect(await screen.findByText("모르는 단어를 골라 주세요")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "학습할래요" })).not.toBeInTheDocument();
    expect(await screen.findByText("stale")).toBeInTheDocument();
  });

  it("고른 단어와 함께 문장을 담는다", async () => {
    listSearches.mockResolvedValue({
      items: [item({ learn_kind: "sentence", selected_text: "the bread went stale" })],
    });
    getSearch.mockResolvedValue(
      detail({
        learn_kind: "sentence",
        selected_text: "the bread went stale",
        triage_state: "needs_selection",
        items: [
          {
            knowledge_item_id: "k-stale",
            surface_text: "stale",
            char_start: 15,
            char_end: 20,
            selected: true,
          },
        ],
      }),
    );
    completeSelection.mockResolvedValue({
      capture_id: "cap-1",
      triage_state: "learning",
      learning_item_ids: ["k-sentence", "k-stale"],
      cards_created: 3,
    });
    render(<SearchHistory />);

    await userEvent.click(await screen.findByText("the bread went stale"));
    await userEvent.click(await screen.findByRole("button", { name: /1개 담고 완료/ }));

    await waitFor(() => expect(completeSelection).toHaveBeenCalledWith("cap-1", false));
  });

  // 짚을 단어가 없다는 것도 답이다. 이걸 막으면 사용자는 목록을 비우려고 아무 단어나
  // 고르게 되고, 그 순간 학습 목록은 거짓이 된다.
  it("짚을 단어가 없어도 문장을 끝낼 수 있다", async () => {
    listSearches.mockResolvedValue({
      items: [item({ learn_kind: "sentence", selected_text: "it depends" })],
    });
    getSearch.mockResolvedValue(
      detail({ learn_kind: "sentence", selected_text: "it depends", triage_state: "needs_selection" }),
    );
    completeSelection.mockResolvedValue({
      capture_id: "cap-1",
      triage_state: "learning",
      learning_item_ids: ["k-sentence"],
      cards_created: 1,
    });
    render(<SearchHistory />);

    await userEvent.click(await screen.findByText("it depends"));
    await userEvent.click(await screen.findByRole("button", { name: "짚을 단어는 없어요" }));

    await waitFor(() => expect(completeSelection).toHaveBeenCalledWith("cap-1", true));
  });

  it("문장을 열어 보는 것 자체가 상태를 남긴다", async () => {
    listSearches.mockResolvedValue({
      items: [item({ learn_kind: "sentence", selected_text: "the bread went stale" })],
    });
    getSearch.mockResolvedValue(
      detail({ learn_kind: "sentence", selected_text: "the bread went stale", triage_state: "unseen" }),
    );
    render(<SearchHistory />);

    await userEvent.click(await screen.findByText("the bread went stale"));

    await waitFor(() => expect(openSearch).toHaveBeenCalledWith("cap-1"));
  });

  it("단어를 골랐다 취소할 수 있다", async () => {
    listSearches.mockResolvedValue({
      items: [item({ learn_kind: "sentence", selected_text: "the bread went stale" })],
    });
    getSearch.mockResolvedValue(
      detail({
        learn_kind: "sentence",
        selected_text: "the bread went stale",
        triage_state: "needs_selection",
        items: [
          {
            knowledge_item_id: "k-stale",
            surface_text: "stale",
            char_start: 15,
            char_end: 20,
            selected: false,
          },
        ],
      }),
    );
    selectWord.mockResolvedValue({
      capture_id: "cap-1",
      knowledge_item_id: "k-stale",
      selected: true,
    });
    render(<SearchHistory />);

    await userEvent.click(await screen.findByText("the bread went stale"));
    await userEvent.click(await screen.findByRole("checkbox"));

    await waitFor(() => expect(selectWord).toHaveBeenCalledWith("cap-1", "k-stale"));
  });
});
