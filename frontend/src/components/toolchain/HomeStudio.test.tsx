import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../../i18n";
import { GenerationJob, ToolAsset } from "../../types";
import HomeStudio from "./HomeStudio";

const galleryApi = vi.hoisted(() => ({
  listGalleryImages: vi.fn(),
  loadImageFull: vi.fn(),
}));

vi.mock("../../../wailsjs/go/main/App", () => ({
  ListGalleryImages: galleryApi.listGalleryImages,
  LoadImageFull: galleryApi.loadImageFull,
}));

const assets: ToolAsset[] = [
  { id: "animation-result", name: "Animation", kind: "image", originModule: "animation", createdAt: "2026-07-01T00:00:00Z" },
  { id: "image-old", name: "Old image", kind: "image", originModule: "image", createdAt: "2026-07-02T00:00:00Z" },
  { id: "image-new", name: "New image", kind: "image", originModule: "image", createdAt: "2026-07-03T00:00:00Z" },
  { id: "video-result", name: "Video", kind: "video", originModule: "video", createdAt: "2026-07-04T00:00:00Z" },
];

const jobs: GenerationJob[] = [
  { id: "video-job", kind: "video", status: "polling", progress: 50, sourceModule: "video", createdAt: "2026-07-04T00:00:00Z", updatedAt: "2026-07-04T00:00:00Z" },
];

vi.mock("./ToolchainContext", () => ({
  useToolchain: () => ({ assets, jobs }),
}));

describe("HomeStudio", () => {
  beforeEach(() => {
    galleryApi.listGalleryImages.mockReset().mockResolvedValue([]);
    galleryApi.loadImageFull.mockReset().mockResolvedValue("");
  });

  it("renders four operational module bands and navigates from each action", () => {
    const onNavigate = vi.fn();
    render(<I18nProvider><HomeStudio onNavigate={onNavigate} /></I18nProvider>);

    expect(screen.getAllByRole("article")).toHaveLength(4);
    const actions = screen.getAllByRole("button");
    expect(actions).toHaveLength(4);
    expect(screen.queryByText("打开模块")).not.toBeInTheDocument();
    expect(actions.every((action) => action.classList.contains("home-module-media"))).toBe(true);

    actions.forEach((action) => fireEvent.click(action));
    expect(onNavigate.mock.calls.map(([module]) => module)).toEqual(["animation", "image", "video", "ui"]);
  });

  it("uses the newest compatible module asset and preserves packaged fallbacks", () => {
    const { container } = render(<I18nProvider><HomeStudio onNavigate={() => {}} /></I18nProvider>);

    expect(container.querySelector('img[src="/toolchain/assets/image-new/content"]')).toBeInTheDocument();
    expect(container.querySelector('img[src="/toolchain/assets/image-old/content"]')).not.toBeInTheDocument();
    expect(container.querySelector('video[src="/toolchain/assets/video-result/content"]')).toBeInTheDocument();
    expect(container.querySelector('img[src="/home/animation.gif"]')).toBeInTheDocument();
    expect(container.querySelector('img[src="/home/ui.webp"]')).toBeInTheDocument();
  });

  it("uses the latest gallery GIF for animation instead of a static strip", async () => {
    galleryApi.listGalleryImages.mockResolvedValue([
      { name: "newer-strip.png", path: "C:\\gallery\\newer-strip.png", size: 400, modTime: 300 },
      { name: "walk-final.gif", path: "C:\\gallery\\walk-final.gif", size: 300, modTime: 200 },
      { name: "idle-old.gif", path: "C:\\gallery\\idle-old.gif", size: 200, modTime: 100 },
    ]);
    galleryApi.loadImageFull.mockResolvedValue("data:image/gif;base64,latest-animation");

    const { container } = render(<I18nProvider><HomeStudio onNavigate={() => {}} /></I18nProvider>);

    await waitFor(() => {
      expect(container.querySelector('img[src="data:image/gif;base64,latest-animation"]')).toBeInTheDocument();
    });
    expect(galleryApi.loadImageFull).toHaveBeenCalledWith("C:\\gallery\\walk-final.gif");
    expect(container.querySelector('img[src="/toolchain/assets/animation-result/content"]')).not.toBeInTheDocument();
  });

  it("refreshes the latest GIF when the user returns to the home module", async () => {
    galleryApi.listGalleryImages
      .mockResolvedValueOnce([{ name: "old.gif", path: "C:\\gallery\\old.gif", size: 100, modTime: 100 }])
      .mockResolvedValueOnce([{ name: "new.gif", path: "C:\\gallery\\new.gif", size: 200, modTime: 200 }]);
    galleryApi.loadImageFull
      .mockResolvedValueOnce("data:image/gif;base64,old")
      .mockResolvedValueOnce("data:image/gif;base64,new");

    const { container, rerender } = render(<I18nProvider><HomeStudio active onNavigate={() => {}} /></I18nProvider>);
    await waitFor(() => expect(container.querySelector('img[src="data:image/gif;base64,old"]')).toBeInTheDocument());

    rerender(<I18nProvider><HomeStudio active={false} onNavigate={() => {}} /></I18nProvider>);
    rerender(<I18nProvider><HomeStudio active onNavigate={() => {}} /></I18nProvider>);

    await waitFor(() => expect(container.querySelector('img[src="data:image/gif;base64,new"]')).toBeInTheDocument());
    expect(galleryApi.listGalleryImages).toHaveBeenCalledTimes(2);
  });
});
