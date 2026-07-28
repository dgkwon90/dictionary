// 연습 화면 컴포넌트 테스트.
//
// 연습의 계약은 두 가지다: 답한 것은 정답률에 기록되고, 복습 일정은 건드리지 않는다.
// 뒤쪽은 서버가 지키므로(연습 채점은 review_cards를 쓰지 않는다) 여기서는 앞쪽 —
// 사용자가 누른 등급이 실제로 서버에 가고, 그 결과가 화면에 보이는지를 본다.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      practiceCards: vi.fn(),
      gradePractice: vi.fn(),
    },
  };
});

import { api, type ReviewCard } from "../api/client";
import Practice from "./Practice";

const practiceCards = vi.mocked(api.practiceCards);
const gradePractice = vi.mocked(api.gradePractice);

function card(overrides: Partial<ReviewCard> = {}): ReviewCard {
  return {
    card_id: "c1",
    knowledge_item_id: "k1",
    card_type: "meaning",
    question: "idempotent의 뜻은?",
    answer: "여러 번 해도 결과가 같은",
    state: "review",
    due_at: "2026-08-01T00:00:00Z",
    ...overrides,
  };
}

async function startSession() {
  practiceCards.mockResolvedValue({ cards: [card()] });
  render(<Practice />);
  await userEvent.click(await screen.findByRole("checkbox", { name: /전체 선택/ }));
  await userEvent.click(screen.getByRole("button", { name: /연습 시작/ }));
  await userEvent.click(await screen.findByRole("button", { name: /답 보기/ }));
}

beforeEach(() => {
  practiceCards.mockReset();
  gradePractice.mockReset();
});

describe("연습", () => {
  it("누른 등급이 서버에 기록되고 정답률이 보인다", async () => {
    gradePractice.mockResolvedValue({
      card_id: "c1",
      rating: "good",
      accuracy: 0.75,
      attempt_count: 4,
      correct_count: 3,
    });
    await startSession();

    await userEvent.click(screen.getByRole("button", { name: /알아요/ }));

    await waitFor(() => expect(gradePractice).toHaveBeenCalledWith("c1", "good"));
    // 채점 결과가 화면에 남아야 한다. 다음 카드로 넘어가면서 지워지면 사용자는 자기가
    // 방금 무엇을 기록했는지 확인할 방법이 없다.
    expect(await screen.findByText(/정답률 75%/)).toBeInTheDocument();
  });

  // 채점이 실패했는데 조용히 넘어가면 사용자는 기록된 줄 안다. 연습은 계속 굴러가되
  // 실패는 말해 줘야 한다.
  it("채점이 실패하면 알려준다", async () => {
    gradePractice.mockRejectedValue(new Error("연결 실패"));
    await startSession();

    await userEvent.click(screen.getByRole("button", { name: /알아요/ }));

    expect(await screen.findByText(/채점을 기록하지 못했어요/)).toBeInTheDocument();
  });

  it("'채점 없이 다음'은 서버를 부르지 않는다", async () => {
    await startSession();

    await userEvent.click(screen.getByRole("button", { name: /채점 없이 다음/ }));

    expect(gradePractice).not.toHaveBeenCalled();
    expect(await screen.findByText("연습 완료 🎉")).toBeInTheDocument();
  });
});
