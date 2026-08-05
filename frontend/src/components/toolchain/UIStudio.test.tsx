import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../../i18n";
import UIStudio from "./UIStudio";

vi.mock("./ToolchainContext", () => ({
  useToolchain: () => ({
    assets: [{ id: "ui-source", name: "UI source", kind: "image", createdAt: "2026-08-01T00:00:00Z" }],
    jobs: [],
    capabilities: { image: [], vision: [] },
    createJob: vi.fn(),
    retryJob: vi.fn(),
    importAssets: vi.fn(),
    notify: vi.fn(),
    backendAvailable: true,
  }),
}));

describe("UIStudio", () => {
  it("keeps split controls and results in sibling columns", () => {
    const { container } = render(
      <I18nProvider>
        <UIStudio onOpenAssets={() => {}} onOpenTasks={() => {}} onNavigate={() => {}} />
      </I18nProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "拆图" }));

    const panel = container.querySelector(".ui-split-panel:not([hidden])");
    expect(panel).toBeInTheDocument();
    expect(panel?.children[0]).toHaveClass("ui-split-source-column");
    expect(panel?.children[1]).toHaveClass("studio-results");
    expect(panel?.querySelector(".asset-picker--preview")).toBeInTheDocument();
    expect(panel?.querySelector(".asset-picker-upload")).toBeInTheDocument();
  });
});
