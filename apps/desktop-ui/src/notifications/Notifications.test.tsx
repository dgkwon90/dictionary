// 알림 목록 화면 테스트.
//
// 지우기는 되돌릴 수 없어서, 여기서 보는 것은 "정말 지워졌는가"보다 "지우려 하지 않은
// 것이 지워지지 않는가"에 가깝다.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      notificationHistory: vi.fn(),
      ackNotification: vi.fn(),
      deleteNotification: vi.fn(),
      deleteAllNotifications: vi.fn(),
    },
  };
});

import { api, type NotificationItem } from "../api/client";
import Notifications from "./Notifications";

const notificationHistory = vi.mocked(api.notificationHistory);
const deleteNotification = vi.mocked(api.deleteNotification);
const deleteAllNotifications = vi.mocked(api.deleteAllNotifications);

function notice(overrides: Partial<NotificationItem> = {}): NotificationItem {
  return {
    id: "n1",
    kind: "result_ready",
    title: "검색 결과 준비 완료",
    body: "여러 번 해도 결과가 같은",
    route: "Search History",
    payload_id: "cap-1",
    created_at: "2026-08-05T11:17:14Z",
    acked: false,
    ...overrides,
  };
}

beforeEach(() => {
  for (const fn of Object.values(api)) vi.mocked(fn).mockReset();
  notificationHistory.mockResolvedValue({ notifications: [notice()], unacked_count: 1 });
  vi.mocked(api.ackNotification).mockResolvedValue({ status: "ok" });
  deleteNotification.mockResolvedValue({ status: "ok" });
  deleteAllNotifications.mockResolvedValue({ deleted: 1 });
});

describe("알림 목록", () => {
  it("하나를 지우면 목록에서 빠진다", async () => {
    render(<Notifications onNavigate={() => {}} />);
    await screen.findByText("검색 결과 준비 완료");

    await userEvent.click(screen.getByRole("button", { name: "검색 결과 준비 완료 지우기" }));

    await waitFor(() => expect(deleteNotification).toHaveBeenCalledWith("n1"));
    await waitFor(() =>
      expect(screen.queryByText("검색 결과 준비 완료")).not.toBeInTheDocument(),
    );
  });

  // 서버가 거절했는데 사라지면 사용자는 지워진 줄 알고, 다음 새로고침에 되살아난 것을
  // 보게 된다. 성공했을 때만 지운다.
  it("지우기가 실패하면 항목이 남고 이유를 보여준다", async () => {
    deleteNotification.mockRejectedValue(new Error("notification not found"));
    render(<Notifications onNavigate={() => {}} />);
    await screen.findByText("검색 결과 준비 완료");

    await userEvent.click(screen.getByRole("button", { name: "검색 결과 준비 완료 지우기" }));

    expect(await screen.findByText(/notification not found/)).toBeInTheDocument();
    expect(screen.getByText("검색 결과 준비 완료")).toBeInTheDocument();
  });

  // 모두 지우기는 한 번에 전부 없앤다. 한 번 더 묻지 않으면 새로고침 옆의 버튼을 잘못
  // 눌러 이력이 통째로 날아간다.
  it("모두 지우기는 확인을 한 번 받는다", async () => {
    render(<Notifications onNavigate={() => {}} />);
    await screen.findByText("검색 결과 준비 완료");

    await userEvent.click(screen.getByRole("button", { name: "모두 지우기" }));
    expect(deleteAllNotifications).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("button", { name: "취소" }));
    expect(deleteAllNotifications).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("button", { name: "모두 지우기" }));
    await userEvent.click(screen.getByRole("button", { name: "정말 모두 지울까요?" }));

    await waitFor(() => expect(deleteAllNotifications).toHaveBeenCalled());
    await waitFor(() => expect(screen.queryByText("검색 결과 준비 완료")).not.toBeInTheDocument());
  });

  it("항목을 누르면 확인 처리하고 그 화면으로 간다", async () => {
    const onNavigate = vi.fn();
    render(<Notifications onNavigate={onNavigate} />);

    await userEvent.click(await screen.findByText("검색 결과 준비 완료"));

    expect(api.ackNotification).toHaveBeenCalledWith("n1");
    expect(onNavigate).toHaveBeenCalledWith("Search History");
  });
});
