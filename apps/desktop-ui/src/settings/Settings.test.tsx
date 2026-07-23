// Settings 백업·복원 UI(RW-09) 컴포넌트 테스트.
//
// Tauri dialog/fs 플러그인은 jsdom에 없으므로 mock한다. api 클라이언트도 mock해
// 네트워크 없이 상태 전이만 검증한다.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const dialogOpen = vi.fn();
const dialogSave = vi.fn();
vi.mock("@tauri-apps/plugin-dialog", () => ({
  open: (...args: unknown[]) => dialogOpen(...args),
  save: (...args: unknown[]) => dialogSave(...args),
}));

const readTextFile = vi.fn();
const writeTextFile = vi.fn();
vi.mock("@tauri-apps/plugin-fs", () => ({
  readTextFile: (...args: unknown[]) => readTextFile(...args),
  writeTextFile: (...args: unknown[]) => writeTextFile(...args),
}));

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      getSettings: vi.fn(),
      updateSettings: vi.fn(),
      exportBackup: vi.fn(),
      importBackup: vi.fn(),
      backupToFile: vi.fn(),
    },
  };
});

import { api, ApiError } from "../api/client";
import Settings from "./Settings";

const PREFS = {
  notifications_enabled: true,
  morning_review_time: "09:00",
  evening_review_time: "21:00",
};
const EFFECTIVE = {
  addr: "127.0.0.1:48989",
  db_path: "/tmp/neulsang.db",
  ai_provider: "mock",
  gemini_model: "",
  api_key_configured: false,
};

async function renderSettings() {
  vi.mocked(api.getSettings).mockResolvedValue({ preferences: PREFS, effective: EFFECTIVE });
  render(<Settings />);
  await waitFor(() => expect(screen.getByText("백업·복원")).toBeInTheDocument());
}

beforeEach(() => {
  vi.mocked(api.getSettings).mockReset();
  vi.mocked(api.exportBackup).mockReset();
  vi.mocked(api.importBackup).mockReset();
  vi.mocked(api.backupToFile).mockReset();
  dialogOpen.mockReset();
  dialogSave.mockReset();
  readTextFile.mockReset();
  writeTextFile.mockReset();
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("Settings 백업·복원", () => {
  it("내보내기: 저장 경로를 골라 JSON을 쓴다", async () => {
    dialogSave.mockResolvedValue("/tmp/backup.json");
    vi.mocked(api.exportBackup).mockResolvedValue({
      version: 2,
      exported_at: "2026-07-01T00:00:00Z",
      captures: [{ id: "c1" }],
    });
    await renderSettings();

    const user = userEvent.setup();
    await user.click(screen.getByText("기록 내보내기"));

    await waitFor(() => expect(writeTextFile).toHaveBeenCalledWith("/tmp/backup.json", expect.any(String)));
    await waitFor(() => expect(screen.getByText(/내보내기 완료/)).toBeInTheDocument());
  });

  it("가져오기 취소: 파일 선택 대화상자에서 취소하면 아무 일도 안 한다", async () => {
    dialogOpen.mockResolvedValue(null);
    await renderSettings();

    const user = userEvent.setup();
    await user.click(screen.getByText("기록 가져오기"));

    await waitFor(() => expect(dialogOpen).toHaveBeenCalled());
    expect(readTextFile).not.toHaveBeenCalled();
    expect(api.importBackup).not.toHaveBeenCalled();
  });

  it("잘못된 파일: JSON으로 파싱이 안 되면 에러를 보여준다", async () => {
    dialogOpen.mockResolvedValue("/tmp/not-json.txt");
    readTextFile.mockResolvedValue("this is not json{{{");
    await renderSettings();

    const user = userEvent.setup();
    await user.click(screen.getByText("기록 가져오기"));

    await waitFor(() =>
      expect(screen.getByText(/이 파일은 읽을 수 없어요/)).toBeInTheDocument(),
    );
    expect(api.importBackup).not.toHaveBeenCalled();
  });

  it("version 필드가 없는 파일은 백업 스냅샷이 아니라고 거부한다", async () => {
    dialogOpen.mockResolvedValue("/tmp/random.json");
    readTextFile.mockResolvedValue(JSON.stringify({ hello: "world" }));
    await renderSettings();

    const user = userEvent.setup();
    await user.click(screen.getByText("기록 가져오기"));

    await waitFor(() => expect(screen.getByText(/백업 파일이 아닌 것 같아요/)).toBeInTheDocument());
    expect(api.importBackup).not.toHaveBeenCalled();
  });

  it("요약을 보여주고 확인 후에만 가져오기를 진행한다", async () => {
    dialogOpen.mockResolvedValue("/tmp/backup.json");
    readTextFile.mockResolvedValue(
      JSON.stringify({ version: 2, exported_at: "2026-07-01T00:00:00Z", captures: [{}, {}] }),
    );
    vi.mocked(api.importBackup).mockResolvedValue({
      knowledge_items: { inserted: 1, merged: 0, updated: 0, skipped: 0 },
      captures: { inserted: 2, merged: 0, updated: 0, skipped: 0 },
    });
    await renderSettings();

    const user = userEvent.setup();
    await user.click(screen.getByText("기록 가져오기"));

    await waitFor(() => expect(screen.getByText(/version 2/)).toBeInTheDocument());
    expect(screen.getByText(/검색 기록: 2건/)).toBeInTheDocument();
    expect(api.importBackup).not.toHaveBeenCalled(); // 아직 확인 전

    await user.click(screen.getByText("가져오기 진행"));

    await waitFor(() => expect(api.importBackup).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByText(/가져오기 완료/)).toBeInTheDocument());
    expect(screen.getByText("2")).toBeInTheDocument(); // captures.inserted
  });

  it("확인 화면에서 취소하면 가져오기를 호출하지 않는다", async () => {
    dialogOpen.mockResolvedValue("/tmp/backup.json");
    readTextFile.mockResolvedValue(JSON.stringify({ version: 1, exported_at: "2026-07-01T00:00:00Z" }));
    await renderSettings();

    const user = userEvent.setup();
    await user.click(screen.getByText("기록 가져오기"));
    await waitFor(() => expect(screen.getByText(/version 1/)).toBeInTheDocument());

    await user.click(screen.getByText("취소"));

    await waitFor(() => expect(screen.queryByText(/version 1/)).not.toBeInTheDocument());
    expect(api.importBackup).not.toHaveBeenCalled();
  });

  it("지원하지 않는 version은 서버 메시지를 그대로 보여준다", async () => {
    dialogOpen.mockResolvedValue("/tmp/future.json");
    readTextFile.mockResolvedValue(JSON.stringify({ version: 99, exported_at: "2026-07-01T00:00:00Z" }));
    vi.mocked(api.importBackup).mockRejectedValue(
      new ApiError(400, "unsupported snapshot version: 99 (supported: 1-2)"),
    );
    await renderSettings();

    const user = userEvent.setup();
    await user.click(screen.getByText("기록 가져오기"));
    await waitFor(() => expect(screen.getByText(/version 99/)).toBeInTheDocument());
    await user.click(screen.getByText("가져오기 진행"));

    await waitFor(() =>
      expect(screen.getByText(/unsupported snapshot version: 99/)).toBeInTheDocument(),
    );
  });

  it("대용량 파일은 413을 사람이 읽을 수 있는 메시지로 보여준다", async () => {
    dialogOpen.mockResolvedValue("/tmp/huge.json");
    readTextFile.mockResolvedValue(JSON.stringify({ version: 2, exported_at: "2026-07-01T00:00:00Z" }));
    vi.mocked(api.importBackup).mockRejectedValue(new ApiError(413, "request entity too large"));
    await renderSettings();

    const user = userEvent.setup();
    await user.click(screen.getByText("기록 가져오기"));
    await waitFor(() => expect(screen.getByText(/version 2/)).toBeInTheDocument());
    await user.click(screen.getByText("가져오기 진행"));

    await waitFor(() => expect(screen.getByText(/너무 커요/)).toBeInTheDocument());
  });

  it("전체 복사본 저장: 저장 경로를 골라 backupToFile을 호출한다", async () => {
    dialogSave.mockResolvedValue("/tmp/backup.db");
    vi.mocked(api.backupToFile).mockResolvedValue({ path: "/tmp/backup.db", size_bytes: 2048 });
    await renderSettings();

    const user = userEvent.setup();
    await user.click(screen.getByText("전체 복사본 저장"));

    await waitFor(() => expect(api.backupToFile).toHaveBeenCalledWith("/tmp/backup.db"));
    await waitFor(() => expect(screen.getByText(/백업 완료/)).toBeInTheDocument());
  });
});
