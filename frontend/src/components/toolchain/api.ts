import {
  GenerationCapabilities,
  GenerationJob,
  ToolAsset,
  ToolAssetKind,
  ToolchainModule,
  UIRegion,
} from "../../types";

type WailsMethod = (...args: any[]) => Promise<any>;

function backendMethod(name: string): WailsMethod | null {
  if (typeof window === "undefined") return null;
  const method = (window as any)?.go?.main?.App?.[name];
  return typeof method === "function" ? method.bind((window as any).go.main.App) : null;
}

export function hasToolchainBackend(): boolean {
  return backendMethod("ListGenerationJobs") !== null && backendMethod("ListAssets") !== null;
}

async function invoke<T>(name: string, ...args: unknown[]): Promise<T> {
  const method = backendMethod(name);
  if (!method) throw new Error(`Toolchain backend method ${name} is unavailable.`);
  return method(...args) as Promise<T>;
}

function asArray<T>(value: unknown): T[] {
  return Array.isArray(value) ? value as T[] : [];
}

function normalizeResult(value: unknown): Record<string, unknown> | string | undefined {
  if (value === null || value === undefined) return undefined;
  let candidate = value;
  if (Array.isArray(value) && value.every((item) => typeof item === "number")) {
    try {
      candidate = new TextDecoder().decode(new Uint8Array(value));
    } catch {
      return undefined;
    }
  }
  if (typeof candidate === "string") {
    try {
      return JSON.parse(candidate) as Record<string, unknown>;
    } catch {
      return candidate;
    }
  }
  return typeof candidate === "object" ? candidate as Record<string, unknown> : String(candidate);
}

export function normalizeJob(value: any): GenerationJob {
  const rawProgress = Number(value?.progress ?? 0);
  const progress = rawProgress > 0 && rawProgress <= 1 ? rawProgress * 100 : rawProgress;
  const providerResponseSummary = normalizeResult(value?.providerResponseSummary);
  return {
    ...value,
    id: String(value?.id ?? ""),
    kind: value?.kind ?? "image",
    status: value?.status ?? "queued",
    progress: Math.max(0, Math.min(100, progress)),
    inputAssetIds: asArray<string>(value?.inputAssetIds),
    outputAssetIds: asArray<string>(value?.outputAssetIds),
    request: value?.request ?? {},
    providerResponseSummary,
    result: normalizeResult(value?.result) ?? providerResponseSummary,
    createdAt: String(value?.createdAt ?? new Date().toISOString()),
    updatedAt: String(value?.updatedAt ?? value?.createdAt ?? new Date().toISOString()),
  };
}

export function normalizeAsset(value: any): ToolAsset {
  return {
    ...value,
    id: String(value?.id ?? ""),
    name: String(value?.name ?? value?.id ?? "Untitled"),
    kind: value?.kind ?? value?.mediaType,
    mediaType: value?.mediaType ?? value?.kind,
    role: value?.role ?? value?.usage,
    usage: value?.usage ?? value?.role,
    mime: value?.mime ?? value?.mimeType,
    mimeType: value?.mimeType ?? value?.mime,
    originModule: value?.originModule ?? value?.sourceModule,
    sourceModule: value?.sourceModule ?? value?.originModule,
    parentIds: asArray<string>(value?.parentIds).length
      ? asArray<string>(value?.parentIds)
      : value?.parentAssetId ? [String(value.parentAssetId)] : [],
    tags: asArray<string>(value?.tags),
    createdAt: String(value?.createdAt ?? new Date().toISOString()),
  };
}

export const toolchainApi = {
  async createImageJob(request: Record<string, unknown>): Promise<GenerationJob> {
    return normalizeJob(await invoke("CreateImageJob", request));
  },

  async createVideoJob(request: Record<string, unknown>): Promise<GenerationJob> {
    return normalizeJob(await invoke("CreateVideoJob", request));
  },

  async createPromptOptimizationJob(request: Record<string, unknown>): Promise<GenerationJob> {
    return normalizeJob(await invoke("CreatePromptOptimizationJob", request));
  },

  async createUIAnalysisJob(request: Record<string, unknown>): Promise<GenerationJob> {
    return normalizeJob(await invoke("CreateUIAnalysisJob", request));
  },

  async createUISplitJob(request: Record<string, unknown>): Promise<GenerationJob> {
    return normalizeJob(await invoke("CreateUISplitJob", request));
  },

  async listJobs(): Promise<GenerationJob[]> {
    return asArray<any>(await invoke("ListGenerationJobs")).map(normalizeJob);
  },

  async getJob(id: string): Promise<GenerationJob> {
    return normalizeJob(await invoke("GetGenerationJob", id));
  },

  async cancelJob(id: string): Promise<GenerationJob | void> {
    const result = await invoke<any>("CancelGenerationJob", id);
    return result ? normalizeJob(result) : undefined;
  },

  async retryJob(id: string): Promise<GenerationJob> {
    return normalizeJob(await invoke("RetryGenerationJob", id));
  },

  async listAssets(): Promise<ToolAsset[]> {
    return asArray<any>(await invoke("ListAssets")).map(normalizeAsset);
  },

  async importAssets(module: ToolchainModule, kind?: ToolAssetKind): Promise<ToolAsset[]> {
    const value = await invoke("ImportAssets", { module, mediaType: kind ?? "" });
    return asArray<any>(value).map(normalizeAsset);
  },

  async deleteAsset(id: string): Promise<void> {
    await invoke("DeleteAsset", id);
  },

  async exportAsset(id: string): Promise<string> {
    return invoke("ExportAsset", id);
  },

  async exportUIProject(request: { sourceAssetId: string; regions: UIRegion[]; prompt?: string }): Promise<string> {
    return invoke("ExportUIProject", request);
  },

  async sendAssetToModule(request: Record<string, unknown>): Promise<any> {
    return invoke("SendAssetToModule", request);
  },

  async getCapabilities(): Promise<GenerationCapabilities> {
    return (await invoke("GetGenerationCapabilities")) ?? {};
  },
};

export function assetMediaKind(asset: ToolAsset): ToolAssetKind {
  return asset.kind ?? asset.mediaType ?? "image";
}

export function assetModule(asset: ToolAsset): ToolchainModule {
  return asset.originModule ?? asset.sourceModule ?? "image";
}

export function assetURL(asset: ToolAsset, thumbnail = false): string {
  if (thumbnail && asset.thumbnailUrl) return asset.thumbnailUrl;
  if (asset.contentUrl) return asset.contentUrl;
  const suffix = thumbnail ? "thumbnail" : "content";
  return `/toolchain/assets/${encodeURIComponent(asset.id)}/${suffix}`;
}

export function isJobActive(job: GenerationJob): boolean {
  return ["queued", "running", "submitting", "polling", "needs_review"].includes(job.status);
}

export function jobResultText(job?: GenerationJob | null): string {
  if (!job) return "";
  if (typeof job.result === "string") return job.result;
  const result = job.result as Record<string, unknown> | undefined;
  return String(result?.optimizedPrompt ?? result?.prompt ?? result?.text ?? "");
}

export function dispatchAssetHandoff(targetModule: ToolchainModule, assetId: string, result: any): void {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new CustomEvent("toolchain-asset-sent", {
    detail: { targetModule, assetId, ...(result ?? {}) },
  }));
}
