// 학습 목록 화면 컴포넌트 테스트.
//
// api 클라이언트를 mock해 네트워크 없이 필터 전달과 액션 후 상태만 검증한다.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      listLearning: vi.fn(),
      retireLearningItem: vi.fn(),
      removeLearningItem: vi.fn(),
    },
  };
});

import { api, type LearningItem } from "../api/client";
import Learning from "./Learning";

const listLearning = vi.mocked(api.listLearning);
const retireLearningItem = vi.mocked(api.retireLearningItem);

function item(overrides: Partial<LearningItem> = {}): LearningItem {
  return {
    knowledge_item_id: "k1",
    surface_text: "stale",
    learn_kind: "word",
    meaning_ko: "오래된",
    status: "active",
    ask_count: 1,
    unknown_count: 1,
    attempt_count: 0,
    correct_count: 0,
    accuracy: 0,
    weakness_score: 0.7,
    registered_at: "2026-07-27T00:00:00Z",
    // 등록 직후 카드는 바로 due라, "다음 복습"은 "지금"으로 확정된다 — 정답률의 "—"와
    // 같은 문자열이 두 칸에 뜨지 않게 해 단언이 무엇을 보는지 분명해진다.
    next_due_at: "2026-07-27T00:00:00Z",
    card_count: 1,
    ...overrides,
  };
}

describe("Learning", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listLearning.mockResolvedValue({ items: [item()] });
  });

  it("칩을 고르면 그 필터로 다시 조회한다", async () => {
    const user = userEvent.setup();
    render(<Learning />);
    await screen.findByText("stale");

    await user.click(screen.getByRole("button", { name: "자주 틀림" }));

    await waitFor(() => {
      expect(listLearning).toHaveBeenLastCalledWith(expect.objectContaining({ scope: "weak" }));
    });
  });

  it("검색어는 제출할 때만 보낸다", async () => {
    const user = userEvent.setup();
    render(<Learning />);
    await screen.findByText("stale");
    listLearning.mockClear();

    await user.type(screen.getByLabelText("학습 목록 검색"), "cache");
    expect(listLearning).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "찾기" }));
    await waitFor(() => {
      expect(listLearning).toHaveBeenLastCalledWith(expect.objectContaining({ q: "cache" }));
    });
  });

  // 0%는 "다 틀렸다"는 뜻이라 아직 한 번도 안 푼 항목과 같은 자리에 놓이면 안 된다.
  it("한 번도 안 푼 항목의 정답률은 0%가 아니라 —", async () => {
    render(<Learning />);
    expect(await screen.findByText("—")).toBeInTheDocument();
  });

  it("푼 적이 있으면 정답률과 횟수를 함께 보여준다", async () => {
    listLearning.mockResolvedValue({
      items: [item({ attempt_count: 4, correct_count: 3, accuracy: 0.75 })],
    });
    render(<Learning />);
    expect(await screen.findByText("75% (3/4)")).toBeInTheDocument();
  });

  it("알겠어요를 누르면 목록에서 빠진다", async () => {
    const user = userEvent.setup();
    retireLearningItem.mockResolvedValue(item({ status: "known" }));
    render(<Learning />);
    await screen.findByText("stale");

    await user.click(screen.getByRole("button", { name: "알겠어요" }));

    await waitFor(() => expect(screen.queryByText("stale")).not.toBeInTheDocument());
    expect(retireLearningItem).toHaveBeenCalledWith("k1");
  });

  // 실패했는데 사라지면 사용자는 처리된 줄 안다. 서버가 성공을 돌려준 뒤에만 지운다.
  it("액션이 실패하면 항목을 지우지 않고 이유를 보여준다", async () => {
    const user = userEvent.setup();
    retireLearningItem.mockRejectedValue(new Error("learning item not found"));
    render(<Learning />);
    await screen.findByText("stale");

    await user.click(screen.getByRole("button", { name: "알겠어요" }));

    expect(await screen.findByText("learning item not found")).toBeInTheDocument();
    expect(screen.getByText("stale")).toBeInTheDocument();
  });

  it("비어 있으면 범위에 맞는 안내를 보여준다", async () => {
    const user = userEvent.setup();
    listLearning.mockResolvedValue({ items: [] });
    render(<Learning />);
    expect(await screen.findByText(/검색한 뒤 \[학습할래요\]/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "자주 틀림" }));
    expect(await screen.findByText(/자주 틀리는 항목이 아직 없어요/)).toBeInTheDocument();
  });
});
