import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import { I18nProvider } from "../../i18n";
import { StudioModeToggle, useStudioMode } from "./StudioControls";

function Harness() {
  const [mode, setMode] = useStudioMode("test");
  const [, rerender] = useState(0);
  return <><StudioModeToggle value={mode} onChange={setMode} /><button onClick={() => rerender((value) => value + 1)}>rerender</button></>;
}

describe("StudioModeToggle", () => {
  it("defaults to simple and persists expert mode", () => {
    localStorage.clear();
    render(<I18nProvider><Harness /></I18nProvider>);
    expect(screen.getByRole("button", { name: "简洁" })).toHaveClass("active");
    fireEvent.click(screen.getByRole("button", { name: "专家" }));
    expect(localStorage.getItem("pp.toolchain.mode.test")).toBe("expert");
    expect(screen.getByRole("button", { name: "专家" })).toHaveClass("active");
  });
});
