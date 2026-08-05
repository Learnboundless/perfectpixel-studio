import { afterEach, describe, expect, it, vi } from "vitest";
import { normalizeJob, toolchainApi } from "./api";

afterEach(() => {
  delete (window as any).go;
});

describe("normalizeJob", () => {
  it("normalizes persisted progress and exposes provider results", () => {
    const job = normalizeJob({
      id: "job-1",
      kind: "prompt_optimization",
      status: "completed",
      progress: 0.75,
      providerResponseSummary: JSON.stringify({ optimizedPrompt: "clear prompt" }),
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:01Z",
    });

    expect(job.progress).toBe(75);
    expect(job.result).toEqual({ optimizedPrompt: "clear prompt" });
  });
});

describe("toolchainApi.importAssets", () => {
  it("always passes the Wails request object even when the wrapper reports zero parameters", async () => {
    const importAssets = vi.fn((..._args: unknown[]) => Promise.resolve([
      { id: "imported-image", name: "Imported image", mediaType: "image", createdAt: "2026-08-01T00:00:00Z" },
    ]));
    (window as any).go = { main: { App: { ImportAssets: importAssets } } };

    const assets = await toolchainApi.importAssets("ui", "image");

    expect(importAssets).toHaveBeenCalledWith({ module: "ui", mediaType: "image" });
    expect(assets[0]?.id).toBe("imported-image");
  });
});
