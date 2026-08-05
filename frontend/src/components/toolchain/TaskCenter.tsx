import { Ban, CircleAlert, Clock3, RefreshCw, RotateCcw, Search, XCircle } from "lucide-react";
import { useMemo, useState } from "react";
import { useI18n } from "../../i18n";
import { GenerationJob, GenerationJobKind } from "../../types";
import { Button } from "../ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "../ui/dialog";
import { Input } from "../ui/input";
import { isJobActive } from "./api";
import { useToolchain } from "./ToolchainContext";

const KIND_KEYS: Record<GenerationJobKind, string> = {
  image: "tool_job_image",
  video: "tool_job_video",
  ui_analysis: "tool_job_ui_analysis",
  ui_split: "tool_job_ui_split",
  prompt_optimization: "tool_job_prompt",
};

function JobRow({ job }: { job: GenerationJob }) {
  const { t, lang } = useI18n();
  const { cancelJob, retryJob, notify } = useToolchain();
  const [working, setWorking] = useState(false);
  const progress = Math.round(job.progress || 0);
  const canRetry = ["failed", "cancelled", "stopped"].includes(job.status);
  const phaseKey = job.phase ? `tool_phase_${job.phase}` : "";
  const phaseLabel = phaseKey && t(phaseKey) !== phaseKey ? t(phaseKey) : job.phase;

  const act = async (action: "cancel" | "retry") => {
    setWorking(true);
    try {
      if (action === "cancel") await cancelJob(job.id);
      else await retryJob(job.id);
    } catch (error) {
      notify("error", String(error));
    } finally {
      setWorking(false);
    }
  };

  return (
    <article className="job-row">
      <div className={`job-kind-icon ${job.status}`} aria-hidden="true">
        {job.status === "failed" ? <CircleAlert size={15} /> : job.status === "cancelled" ? <XCircle size={15} /> : <Clock3 size={15} />}
      </div>
      <div className="job-main">
        <div className="job-title-row">
          <strong>{t(KIND_KEYS[job.kind] ?? "tool_job_unknown")}</strong>
          <span className={`job-status ${job.status}`}>{t(`tool_status_${job.status}`)}</span>
          <time>{new Date(job.updatedAt).toLocaleString(lang)}</time>
        </div>
        <div className="job-meta">
          <span>{job.provider || t("tool_local")}</span>
          {job.model && <span>{job.model}</span>}
          {phaseLabel && <span>{phaseLabel}</span>}
          <code title={job.id}>{job.id.slice(0, 8)}</code>
        </div>
        {isJobActive(job) && (
          <div className="job-progress" aria-label={`${progress}%`}>
            <span style={{ width: `${progress}%` }} />
          </div>
        )}
        {job.error && <p className="job-error">{job.error}</p>}
        {job.upstreamMayContinue && <p className="job-warning">{t("tool_upstream_warning")}</p>}
      </div>
      <div className="job-actions">
        {isJobActive(job) && (
          <Button variant="ghost" size="icon" disabled={working} onClick={() => act("cancel")} title={t("tool_cancel_job")}>
            <Ban size={14} />
          </Button>
        )}
        {canRetry && (
          <Button variant="ghost" size="icon" disabled={working} onClick={() => act("retry")} title={t("tool_retry_job")}>
            <RotateCcw size={14} />
          </Button>
        )}
      </div>
    </article>
  );
}

export default function TaskCenter({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useI18n();
  const { jobs, refreshJobs } = useToolchain();
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("all");
  const filtered = useMemo(() => jobs.filter((job) => {
    const text = `${job.kind} ${job.status} ${job.provider ?? ""} ${job.model ?? ""} ${job.phase ?? ""}`.toLowerCase();
    return (status === "all" || job.status === status) && text.includes(query.trim().toLowerCase());
  }), [jobs, query, status]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="tool-dialog task-center-dialog" aria-describedby="task-center-description">
        <div className="tool-dialog-head">
          <div>
            <DialogTitle>{t("tool_tasks_title")}</DialogTitle>
            <DialogDescription id="task-center-description">{t("tool_tasks_desc")}</DialogDescription>
          </div>
          <Button variant="ghost" size="icon" onClick={() => void refreshJobs()} title={t("refresh")}><RefreshCw size={14} /></Button>
        </div>
        <div className="tool-filterbar">
          <div className="tool-search"><Search size={13} /><Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t("tool_search_jobs")} /></div>
          <select value={status} onChange={(e) => setStatus(e.target.value)} aria-label={t("tool_filter_status")}>
            <option value="all">{t("tool_all_statuses")}</option>
            <option value="running">{t("tool_status_running")}</option>
            <option value="completed">{t("tool_status_completed")}</option>
            <option value="failed">{t("tool_status_failed")}</option>
            <option value="cancelled">{t("tool_status_cancelled")}</option>
            <option value="needs_review">{t("tool_status_needs_review")}</option>
          </select>
        </div>
        <div className="job-list">
          {filtered.length ? filtered.map((job) => <JobRow key={job.id} job={job} />) : (
            <div className="tool-empty"><Clock3 size={22} /><strong>{t("tool_tasks_empty")}</strong><span>{t("tool_tasks_empty_desc")}</span></div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
