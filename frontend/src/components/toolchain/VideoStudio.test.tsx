import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../../i18n";
import VideoStudio from "./VideoStudio";

Object.defineProperty(HTMLElement.prototype, "scrollIntoView", { configurable: true, value: vi.fn() });

const toolchain = vi.hoisted(() => ({
  createJob: vi.fn(),
  notify: vi.fn(),
  assets: [
    { id: "reference-image", name: "Hero start frame", kind: "image", createdAt: "2026-07-31T00:00:00Z" },
  ],
  jobs: [],
  capabilities: {
    video: [{
      provider: "test-provider",
      model: "test-video-model",
      supportsText: true,
      supportsImageReference: true,
      supportsFirstFrame: true,
      supportsLastFrame: true,
      supportsIdentityReference: true,
      supportsMultipleReferences: true,
      maxReferenceImages: 4,
      durations: [4],
      aspectRatios: ["16:9"],
      resolutions: ["720p"],
    }],
  },
  backendAvailable: true,
  importAssets: vi.fn(),
  retryJob: vi.fn(),
}));

vi.mock("./ToolchainContext", () => ({
  useToolchain: () => toolchain,
}));

describe("VideoStudio image references", () => {
  beforeEach(() => {
    localStorage.clear();
    toolchain.createJob.mockReset();
    toolchain.notify.mockReset();
    toolchain.importAssets.mockReset();
    toolchain.createJob.mockResolvedValue({
      id: "video-job",
      kind: "video",
      status: "queued",
      progress: 0,
      createdAt: "2026-07-31T00:00:00Z",
      updatedAt: "2026-07-31T00:00:00Z",
    });
  });

  it("shows a large preview immediately after choosing an existing asset", async () => {
    const { container } = render(
      <I18nProvider>
        <VideoStudio
          onOpenAssets={() => {}}
          onOpenTasks={() => {}}
          onNavigate={() => {}}
          defaultProvider="test-provider"
          defaultModel="test-video-model"
        />
      </I18nProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "图生视频" }));
    fireEvent.click(screen.getByRole("combobox", { name: "参考图" }));
    fireEvent.click(await screen.findByRole("option", { name: "Hero start frame" }));

    await waitFor(() => expect(container.querySelector(".asset-picker-preview-item")).toBeInTheDocument());
    expect(screen.getByText("1/1")).toBeInTheDocument();
  });

  it("previews an imported asset immediately and lets it replace the selected single image", async () => {
    toolchain.importAssets
      .mockResolvedValueOnce([
        { id: "imported-one", name: "Imported frame one", kind: "image", createdAt: "2026-07-31T00:00:00Z" },
      ])
      .mockResolvedValueOnce([
        { id: "imported-two", name: "Imported frame two", kind: "image", createdAt: "2026-07-31T00:01:00Z" },
      ]);
    const { container } = render(
      <I18nProvider>
        <VideoStudio
          onOpenAssets={() => {}}
          onOpenTasks={() => {}}
          onNavigate={() => {}}
          defaultProvider="test-provider"
          defaultModel="test-video-model"
        />
      </I18nProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "图生视频" }));
    fireEvent.click(container.querySelector(".asset-picker-upload") as HTMLElement);

    expect(await screen.findByText("Imported frame one")).toBeInTheDocument();
    expect(container.querySelector('img[src="/toolchain/assets/imported-one/content"]')).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "替换图片: Imported frame one" }));
    await waitFor(() => expect(container.querySelector(".asset-picker-preview-name")).toHaveTextContent("Imported frame two"));
    expect(container.querySelector(".asset-picker-preview-name")).not.toHaveTextContent("Imported frame one");

    fireEvent.change(screen.getByPlaceholderText(/4 秒游戏动画参考/), { target: { value: "Imported hero moves" } });
    fireEvent.click(screen.getByRole("button", { name: "生成视频" }));
    await waitFor(() => expect(toolchain.createJob).toHaveBeenCalledWith("video", expect.objectContaining({
      referenceAssetIds: ["imported-two"],
    })));

    fireEvent.click(screen.getByRole("button", { name: /移除资产: Imported frame two/ }));
    expect(container.querySelector(".asset-picker-preview-item")).not.toBeInTheDocument();
    expect(screen.getByText("0/1")).toBeInTheDocument();
  });

  it("previews a handed-off image and submits the same asset ID", async () => {
    const { container } = render(
      <I18nProvider>
        <VideoStudio
          onOpenAssets={() => {}}
          onOpenTasks={() => {}}
          onNavigate={() => {}}
          defaultProvider="test-provider"
          defaultModel="test-video-model"
        />
      </I18nProvider>,
    );

    act(() => {
      window.dispatchEvent(new CustomEvent("toolchain-asset-sent", {
        detail: { targetModule: "video", prefill: { referenceAssetIds: ["reference-image"] } },
      }));
    });

    expect(await screen.findByText("Hero start frame")).toBeInTheDocument();
    expect(container.querySelector(".asset-picker-preview-item")).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText(/4 秒游戏动画参考/), { target: { value: "Hero runs forward" } });
    fireEvent.click(screen.getByRole("button", { name: "生成视频" }));

    await waitFor(() => expect(toolchain.createJob).toHaveBeenCalledWith("video", expect.objectContaining({
      mode: "image",
      referenceMode: "first_frame",
      referenceAssetIds: ["reference-image"],
    })));
  });
});
