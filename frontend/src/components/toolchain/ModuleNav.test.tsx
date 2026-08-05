import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../../i18n";
import ModuleNav from "./ModuleNav";

describe("ModuleNav", () => {
  it("exposes the homepage and all four modules", () => {
    const onChange = vi.fn();
    render(<I18nProvider><ModuleNav value="home" onChange={onChange} /></I18nProvider>);

    expect(screen.getAllByRole("button")).toHaveLength(5);
    expect(screen.getByRole("button", { name: "首页" })).toHaveAttribute("aria-current", "page");
    fireEvent.click(screen.getByRole("button", { name: "视频生成" }));
    expect(onChange).toHaveBeenCalledWith("video");
  });
});
