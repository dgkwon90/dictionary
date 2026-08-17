// 검색 기록 화면 컴포넌트 테스트.
//
// api 클라이언트를 mock해 네트워크 없이 검증한다. 여기서 보는 것은 이 화면이 존재하는
// 이유 그대로다: 검색한 것이 목록에 나오고, 단어는 한 번에 담을 수 있고, 문장은 단어를
// 고르기 전에는 담을 수 없다.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
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
      retrySearch: vi.fn(),
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
const retrySearch = vi.mocked(api.retrySearch);
const discardSearch = vi.mocked(api.discardSearch);

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

// 가짜 시계를 쓰는 테스트가 중간에 실패해도 다음 테스트는 진짜 시계로 시작해야 한다.
afterEach(() => {
  vi.useRealTimers();
});

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

  // 하나를 처리하면 그 검색은 미확인 목록에서 빠진다. 그때 보기가 [전체]로 바뀌어
  // 버리면, 사용자는 방금 한 번 정했을 뿐인데 화면 전체가 예고 없이 달라진다.
  it("학습에 담아도 보고 있던 필터가 그대로다", async () => {
    listSearches.mockResolvedValueOnce({ items: [item()] });
    getSearch.mockResolvedValue(detail());
    learnSearch.mockResolvedValue({
      capture_id: "cap-1",
      triage_state: "learning",
      learning_item_ids: ["k1"],
      cards_created: 1,
    });
    // 담은 뒤의 미확인 목록은 비어 있다.
    listSearches.mockResolvedValue({ items: [] });
    render(<SearchHistory />);

    await userEvent.click(await screen.findByText("idempotent"));
    await userEvent.click(await screen.findByRole("button", { name: "학습할래요" }));
    await waitFor(() => expect(learnSearch).toHaveBeenCalled());

    await waitFor(() =>
      expect(listSearches.mock.calls.every(([arg]) => arg?.view === "unresolved")).toBe(true),
    );
  });

  // 팝업에서 넘어온 검색은 예외다: 이미 학습 중인 문장에 단어를 더 고르러 올 수 있고,
  // 그 검색은 미확인 목록에 없다. 이때만 전체 보기로 넓혀 찾아 준다.
  it("팝업이 넘긴 검색이 미확인에 없으면 전체로 넓힌다", async () => {
    listSearches.mockResolvedValue({ items: [] });
    getSearch.mockResolvedValue(detail({ triage_state: "learning" }));
    render(<SearchHistory openCaptureId="cap-1" />);

    await waitFor(() =>
      expect(listSearches).toHaveBeenCalledWith({ view: "all", kind: undefined }),
    );
  });

  // 해석이 실패한 검색은 단어/문장 판정이 없어 결정 버튼이 하나도 안 그려진다. 그대로
  // 두면 다시 해석할 수도, 지울 수도 없는 검색이 목록에 영원히 남는다.
  it("해석에 실패한 검색은 다시 해석하거나 지울 수 있다", async () => {
    const failed = { job_status: "failed" as const, learn_kind: undefined, brief_ko: undefined };
    listSearches.mockResolvedValue({ items: [item(failed)] });
    getSearch.mockResolvedValue(detail(failed));
    // 다시 걸고 나면 서버는 이 검색을 queued로 돌려준다 — 실패 상태가 아니게 되므로
    // 방금 누른 버튼들은 사라진다. 그래서 "다시 해석 중"이라는 사실은 버튼이 아니라
    // 이 패널이 말해야 한다(버튼 안에 두면 누르는 순간 같이 사라져서 아무 반응이 없다).
    getSearch.mockResolvedValueOnce(detail(failed));
    getSearch.mockResolvedValue(detail({ job_status: "queued", learn_kind: undefined }));
    retrySearch.mockResolvedValue({
      capture_id: "cap-1",
      lookup_job_id: "job-2",
      status: "queued",
    });
    render(<SearchHistory />);

    await userEvent.click(await screen.findByText("idempotent"));

    expect(await screen.findByRole("button", { name: "삭제" })).toBeInTheDocument();
    await userEvent.click(await screen.findByRole("button", { name: "다시 해석" }));

    await waitFor(() => expect(retrySearch).toHaveBeenCalledWith("cap-1"));
    expect(await screen.findByText("해석 중…")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "다시 해석" })).not.toBeInTheDocument();
  });

  // 다시 해석을 걸면 결과가 올 때까지 이 패널이 스스로 갱신돼야 한다. 목록 폴링은
  // 목록만 고치므로, 여기서 멈추면 사용자는 다 끝난 해석을 계속 "해석 중"으로 본다.
  it("해석이 진행 중이면 상세가 스스로 갱신된다", async () => {
    // shouldAdvanceTime: RTL의 waitFor·findBy가 기다리는 동안 시계도 같이 흐르게 한다.
    // 안 그러면 이 테스트가 5초 타임아웃까지 매달리고, 그때는 finally도 못 돌아 가짜
    // 시계가 다음 테스트로 새어 나간다(파일 전체가 무너진다). 복구는 afterEach가 맡는다.
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const queued = { job_status: "queued" as const, learn_kind: undefined, brief_ko: undefined };
    listSearches.mockResolvedValue({ items: [item(queued)] });
    getSearch.mockResolvedValue(detail(queued));
    render(<SearchHistory openCaptureId="cap-1" />);

    await waitFor(() => expect(getSearch).toHaveBeenCalled());
    const first = getSearch.mock.calls.length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });

    expect(getSearch.mock.calls.length).toBeGreaterThan(first);
  });

  it("해석에 실패한 검색도 삭제된다", async () => {
    const failed = { job_status: "failed" as const, learn_kind: undefined, brief_ko: undefined };
    listSearches.mockResolvedValue({ items: [item(failed)] });
    getSearch.mockResolvedValue(detail(failed));
    discardSearch.mockResolvedValue({
      capture_id: "cap-1",
      triage_state: "discarded",
      learning_item_ids: [],
      cards_created: 0,
    });
    render(<SearchHistory />);

    await userEvent.click(await screen.findByText("idempotent"));
    await userEvent.click(await screen.findByRole("button", { name: "삭제" }));

    await waitFor(() => expect(discardSearch).toHaveBeenCalledWith("cap-1"));
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
