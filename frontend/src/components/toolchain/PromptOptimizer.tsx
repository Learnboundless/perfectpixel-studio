import { Check, Clipboard, RotateCcw, Sparkles } from "lucide-react";
import { useEffect, useState } from "react";
import { useI18n } from "../../i18n";
import { Button } from "../ui/button";
import { Textarea } from "../ui/textarea";
import { isJobActive, jobResultText } from "./api";
import { useToolchain } from "./ToolchainContext";

interface Props {
  purpose: "image" | "video" | "ui";
  value: string;
  onChange: (value: string) => void;
  compact?: boolean;
}

export default function PromptOptimizer({ purpose, value, onChange, compact = false }: Props) {
  const { t } = useI18n();
  const { jobs, createJob, notify } = useToolchain();
  const [jobId, setJobId] = useState("");
  const [original, setOriginal] = useState("");
  const [optimized, setOptimized] = useState("");
  const job = jobs.find((item) => item.id === jobId);

  useEffect(() => {
    if (!job || job.status !== "completed") return;
    const text = jobResultText(job);
    if (text) setOptimized(text);
  }, [job]);

  const run = async () => {
    if (!value.trim()) return;
    setOriginal(value);
    setOptimized("");
    try {
      const next = await createJob("prompt_optimization", { purpose, prompt: value.trim(), sourceModule: purpose === "ui" ? "ui" : purpose });
      setJobId(next.id);
    } catch (error) {
      notify("error", String(error));
    }
  };

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(optimized);
      notify("success", t("tool_copied"));
    } catch (error) {
      notify("error", String(error));
    }
  };

  return (
    <div className={`prompt-optimizer ${compact ? "compact" : ""}`}>
      <div className="prompt-optimizer-head">
        <div><strong>{t("tool_prompt_optimize")}</strong><span>{t(`tool_prompt_${purpose}_desc`)}</span></div>
        <Button type="button" variant="outline" size="sm" disabled={!value.trim() || !!(job && isJobActive(job))} onClick={run}>
          {job && isJobActive(job) ? <span className="spinner" /> : <Sparkles size={13} />}{t("tool_optimize")}
        </Button>
      </div>
      {(optimized || (job && isJobActive(job)) || job?.error) && (
        <div className="prompt-compare">
          <div><label>{t("tool_prompt_original")}</label><Textarea readOnly value={original} /></div>
          <div>
            <label>{t("tool_prompt_optimized")}</label>
            <Textarea
              value={optimized}
              readOnly={!!(job && isJobActive(job))}
              placeholder={job && isJobActive(job) ? t("tool_optimizing") : ""}
              onChange={(event) => setOptimized(event.target.value)}
            />
          </div>
        </div>
      )}
      {job?.error && <p className="tool-inline-error">{job.error}</p>}
      {optimized && (
        <div className="prompt-optimizer-actions">
          <Button type="button" size="sm" onClick={() => onChange(optimized)}><Check size={13} />{t("tool_accept")}</Button>
          <Button type="button" variant="ghost" size="sm" onClick={() => onChange(original)}><RotateCcw size={13} />{t("tool_undo")}</Button>
          <Button type="button" variant="ghost" size="sm" onClick={copy}><Clipboard size={13} />{t("tool_copy")}</Button>
        </div>
      )}
    </div>
  );
}
