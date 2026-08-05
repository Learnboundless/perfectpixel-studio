import { fireEvent, render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ToolAsset } from "../../types";
import AssetThumb from "./AssetThumb";

const asset: ToolAsset = {
  id: "legacy-image",
  name: "Legacy image",
  kind: "image",
  createdAt: "2026-08-01T00:00:00Z",
};

describe("AssetThumb", () => {
  it("uses full content immediately for a large preview", () => {
    const { container } = render(<AssetThumb asset={asset} full />);

    expect(container.querySelector('img[src="/toolchain/assets/legacy-image/content"]')).toBeInTheDocument();
  });

  it("falls back from a missing thumbnail to the controlled content route", () => {
    const { container } = render(<AssetThumb asset={asset} compact />);
    const thumbnail = container.querySelector('img[src="/toolchain/assets/legacy-image/thumbnail"]') as HTMLImageElement;

    expect(thumbnail).toBeInTheDocument();
    fireEvent.error(thumbnail);

    expect(container.querySelector('img[src="/toolchain/assets/legacy-image/content"]')).toBeInTheDocument();
  });
});
