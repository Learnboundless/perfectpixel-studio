import { createContext, ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { EventsOn } from "../../../wailsjs/runtime/runtime";
import {
  GenerationCapabilities,
  GenerationJob,
  GenerationJobKind,
  ToolAsset,
  ToolAssetKind,
  ToolchainModule,
} from "../../types";
import { hasToolchainBackend, isJobActive, normalizeJob, toolchainApi } from "./api";

type NoticeKind = "info" | "error" | "success";

interface ToolchainValue {
  jobs: GenerationJob[];
  assets: ToolAsset[];
  capabilities: GenerationCapabilities;
  loading: boolean;
  backendAvailable: boolean;
  createJob: (kind: GenerationJobKind, request: Record<string, unknown>) => Promise<GenerationJob>;
  cancelJob: (id: string) => Promise<void>;
  retryJob: (id: string) => Promise<GenerationJob>;
  refreshJobs: () => Promise<void>;
  refreshAssets: () => Promise<void>;
  importAssets: (module: ToolchainModule, kind?: ToolAssetKind) => Promise<ToolAsset[]>;
  deleteAsset: (id: string) => Promise<void>;
  notify: (kind: NoticeKind, message: string) => void;
}

const ToolchainContext = createContext<ToolchainValue | null>(null);

function sortJobs(jobs: GenerationJob[]) {
  return [...jobs].sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt));
}

export function ToolchainProvider({
  children,
  onNotify,
}: {
  children: ReactNode;
  onNotify: (kind: NoticeKind, message: string) => void;
}) {
  const [jobs, setJobs] = useState<GenerationJob[]>([]);
  const [assets, setAssets] = useState<ToolAsset[]>([]);
  const [capabilities, setCapabilities] = useState<GenerationCapabilities>({});
  const [loading, setLoading] = useState(true);
  const backendAvailable = hasToolchainBackend();
  const completedIds = useRef(new Set<string>());

  const notify = useCallback((kind: NoticeKind, message: string) => onNotify(kind, message), [onNotify]);

  const refreshJobs = useCallback(async () => {
    if (!hasToolchainBackend()) return;
    const next = await toolchainApi.listJobs();
    setJobs(sortJobs(next));
  }, []);

  const refreshAssets = useCallback(async () => {
    if (!hasToolchainBackend()) return;
    const next = await toolchainApi.listAssets();
    setAssets(next);
  }, []);

  const mergeJob = useCallback((job: GenerationJob) => {
    setJobs((current) => sortJobs([job, ...current.filter((item) => item.id !== job.id)]));
    if (job.status === "completed" && !completedIds.current.has(job.id)) {
      completedIds.current.add(job.id);
      void refreshAssets();
    }
  }, [refreshAssets]);

  useEffect(() => {
    let cancelled = false;
    if (!backendAvailable) {
      setLoading(false);
      return;
    }
    Promise.all([toolchainApi.listJobs(), toolchainApi.listAssets(), toolchainApi.getCapabilities()])
      .then(([nextJobs, nextAssets, nextCapabilities]) => {
        if (cancelled) return;
        setJobs(sortJobs(nextJobs));
        setAssets(nextAssets);
        setCapabilities(nextCapabilities ?? {});
      })
      .catch(() => {})
      .finally(() => !cancelled && setLoading(false));
    return () => { cancelled = true; };
  }, [backendAvailable]);

  useEffect(() => {
    if (!backendAvailable) return;
    const offs: Array<() => void> = [];
    const receive = (payload: any) => {
      const raw = payload?.job ?? payload;
      if (raw?.id) mergeJob(normalizeJob(raw));
    };
    try {
      offs.push(EventsOn("generation-job-updated", receive));
    } catch {
      // Polling below remains the compatibility path for older runtimes.
    }
    return () => offs.forEach((off) => off());
  }, [backendAvailable, mergeJob]);

  useEffect(() => {
    if (!backendAvailable || !jobs.some(isJobActive)) return;
    const timer = window.setInterval(() => void refreshJobs().catch(() => {}), 3000);
    return () => window.clearInterval(timer);
  }, [backendAvailable, jobs, refreshJobs]);

  const createJob = useCallback(async (kind: GenerationJobKind, request: Record<string, unknown>) => {
    let job: GenerationJob;
    if (kind === "image") job = await toolchainApi.createImageJob(request);
    else if (kind === "video") job = await toolchainApi.createVideoJob(request);
    else if (kind === "ui_analysis") job = await toolchainApi.createUIAnalysisJob(request);
    else if (kind === "ui_split") job = await toolchainApi.createUISplitJob(request);
    else job = await toolchainApi.createPromptOptimizationJob(request);
    mergeJob(job);
    return job;
  }, [mergeJob]);

  const cancelJob = useCallback(async (id: string) => {
    const updated = await toolchainApi.cancelJob(id);
    if (updated) mergeJob(updated);
    else await refreshJobs();
  }, [mergeJob, refreshJobs]);

  const retryJob = useCallback(async (id: string) => {
    const job = await toolchainApi.retryJob(id);
    mergeJob(job);
    return job;
  }, [mergeJob]);

  const importAssets = useCallback(async (module: ToolchainModule, kind?: ToolAssetKind) => {
    const imported = await toolchainApi.importAssets(module, kind);
    await refreshAssets();
    return imported;
  }, [refreshAssets]);

  const deleteAsset = useCallback(async (id: string) => {
    await toolchainApi.deleteAsset(id);
    setAssets((current) => current.filter((asset) => asset.id !== id));
  }, []);

  const value = useMemo<ToolchainValue>(() => ({
    jobs,
    assets,
    capabilities,
    loading,
    backendAvailable,
    createJob,
    cancelJob,
    retryJob,
    refreshJobs,
    refreshAssets,
    importAssets,
    deleteAsset,
    notify,
  }), [
    jobs, assets, capabilities, loading, backendAvailable, createJob, cancelJob, retryJob,
    refreshJobs, refreshAssets, importAssets, deleteAsset, notify,
  ]);

  return <ToolchainContext.Provider value={value}>{children}</ToolchainContext.Provider>;
}

export function useToolchain() {
  const value = useContext(ToolchainContext);
  if (!value) throw new Error("useToolchain must be used within ToolchainProvider");
  return value;
}
