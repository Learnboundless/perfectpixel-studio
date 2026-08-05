import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../../i18n";
import { ToolAsset } from "../../types";
import AssetPicker from "./AssetPicker";

const toolchain = vi.hoisted(() => ({
  assets: [
    { id: "reference-image", name: "Hero start frame", kind: "image", width: 128, height: 256, createdAt: "2026-07-31T00:00:00Z" },
    { id: "second-image", name: "Second frame", kind: "image", width: 512, height: 512, createdAt: "2026-07-31T00:01:00Z" },
  ] as ToolAsset[],
  importAssets: vi.fn(),
  notify: vi.fn(),
}));

vi.mock("./ToolchainContext", () => ({
  useToolchain: () => toolchain,
}));

describe("AssetPicker", () => {
  beforeEach(() => {
    toolchain.importAssets.mockReset();
    toolchain.notify.mockReset();
  });

  it("renders selected assets as a large preview without changing their IDs", () => {
    const onChange = vi.fn();
    const { container } = render(
      <I18nProvider>
        <AssetPicker kind="image" value={["reference-image"]} onChange={onChange} label="Reference image" />
      </I18nProvider>,
    );

    expect(container.querySelector(".asset-picker--preview")).toBeInTheDocument();
    expect(container.querySelector(".asset-picker-preview-item")).toBeInTheDocument();
    expect(container.querySelector(".asset-picker-chip")).not.toBeInTheDocument();
    expect(container.querySelector('img[src="/toolchain/assets/reference-image/content"]')).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "移除资产: Hero start frame" }));
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it("only uses compact chips when preview is explicitly disabled", () => {
    const { container } = render(
      <I18nProvider>
        <AssetPicker kind="image" value={["reference-image"]} onChange={() => {}} label="Reference image" preview={false} />
      </I18nProvider>,
    );

    expect(container.querySelector(".asset-picker-chip")).toBeInTheDocument();
    expect(container.querySelector(".asset-picker-preview-item")).not.toBeInTheDocument();
  });

  it("shows an upload preview surface and imports directly into it", async () => {
    toolchain.importAssets.mockResolvedValue([
      { id: "imported-image", name: "Imported image", kind: "image", createdAt: "2026-08-01T00:00:00Z" },
    ]);
    const onChange = vi.fn();
    const { container } = render(
      <I18nProvider>
        <AssetPicker kind="image" module="ui" value={[]} onChange={onChange} label="Source image" />
      </I18nProvider>,
    );

    fireEvent.click(container.querySelector(".asset-picker-upload") as HTMLElement);

    await waitFor(() => expect(toolchain.importAssets).toHaveBeenCalledWith("ui", "image"));
    expect(onChange).toHaveBeenCalledWith(["imported-image"]);
  });

  it("renders image thumbnails and dimensions inside the asset dropdown", async () => {
    render(
      <I18nProvider>
        <AssetPicker kind="image" value={[]} onChange={() => {}} label="Reference image" />
      </I18nProvider>,
    );

    fireEvent.click(screen.getByRole("combobox", { name: "Reference image" }));
    const option = await screen.findByRole("option", { name: /Hero start frame/ });

    expect(option.querySelector('img[src="/toolchain/assets/reference-image/thumbnail"]')).toBeInTheDocument();
    expect(option).toHaveTextContent("128 x 256");
  });
});
