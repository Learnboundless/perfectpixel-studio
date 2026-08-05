import { ImagePlus, Layers3, Sparkles, WandSparkles } from "lucide-react";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { useI18n } from "../../i18n";
import { ToolchainModule } from "../../types";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../ui/select";
import { Textarea } from "../ui/textarea";
import AssetPicker from "./AssetPicker";
import { isJobActive } from "./api";
import PromptOptimizer from "./PromptOptimizer";
import ResultGallery from "./ResultGallery";
import { FieldLabel, StudioModeToggle, useStudioMode } from "./StudioControls";
import { useToolchain } from "./ToolchainContext";

interface Props {
  onOpenAssets: (assetId?: string) => void;
  onOpenTasks: () => void;
  onNavigate: (module: ToolchainModule) => void;
  defaultProvider?: string;
  defaultModel?: string;
}

export default function ImageStudio({ onOpenAssets, onOpenTasks, onNavigate, defaultProvider, defaultModel }: Props) {
  const { t } = useI18n();
  const { jobs, capabilities, createJob, notify, backendAvailable } = useToolchain();
  const [mode, setMode] = useStudioMode("image");
  const [prompt, setPrompt] = useState("");
  const [references, setReferences] = useState<string[]>([]);
  const [aspectRatio, setAspectRatio] = useState("1:1");
  const [variants, setVariants] = useState(1);
  const [modelKey, setModelKey] = useState("auto");
  const [negativePrompt, setNegativePrompt] = useState("");
  const [editMode, setEditMode] = useState(false);
  const [removeBackground, setRemoveBackground] = useState(false);
  const [pixelize, setPixelize] = useState(false);
  const [pixelSize, setPixelSize] = useState(128);
  const imageJobs = useMemo(() => jobs.filter((job) => job.kind === "image" && (job.sourceModule === "image" || job.request?.sourceModule === "image")), [jobs]);
  const activeJob = imageJobs.find(isJobActive);
  const models = capabilities.image ?? capabilities.imageModels ?? [];
  const selectedCapability = modelKey === "auto"
    ? models.find((item) => item.provider === defaultProvider && item.model === defaultModel)
    : models.find((item) => `${item.provider}::${item.model}` === modelKey);
  const supportsReferences = selectedCapability?.supportsImageReference ?? true;
  const maxReferences = supportsReferences ? Math.max(1, selectedCapability?.maxReferenceImages || 4) : 0;
  const aspectRatios = selectedCapability?.aspectRatios?.length ? selectedCapability.aspectRatios : ["1:1", "4:3", "3:4", "16:9", "9:16"];

  useEffect(() => {
    const receive = (event: Event) => {
      const detail = (event as CustomEvent).detail;
      if (detail?.targetModule !== "image") return;
      const ids = detail?.prefill?.referenceAssetIds;
      if (Array.isArray(ids)) setReferences(ids.slice(0, 4));
    };
    window.addEventListener("toolchain-asset-sent", receive);
    return () => window.removeEventListener("toolchain-asset-sent", receive);
  }, []);

  useEffect(() => {
    if (!supportsReferences) {
      setReferences([]);
      setEditMode(false);
      return;
    }
    setReferences((current) => current.slice(0, Math.min(4, maxReferences)));
  }, [supportsReferences, maxReferences]);

  useEffect(() => {
    if (!aspectRatios.includes(aspectRatio)) setAspectRatio(aspectRatios[0]);
  }, [aspectRatios, aspectRatio]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!prompt.trim()) return;
    const [provider, model] = modelKey === "auto" ? [undefined, undefined] : modelKey.split("::");
    try {
      await createJob("image", {
        sourceModule: "image",
        prompt: prompt.trim(),
        negativePrompt: negativePrompt.trim(),
        referenceAssetIds: references,
        mode: editMode && references.length ? "edit" : references.length ? "reference" : "text",
        aspectRatio,
        variants,
        provider,
        model,
        postProcess: { removeBackground, pixelize, pixelSize: pixelize ? pixelSize : undefined },
      });
      notify("success", t("tool_job_created"));
    } catch (error) {
      notify("error", String(error));
    }
  };

  return (
    <div className="studio-page image-studio">
      <div className="studio-toolbar">
        <div className="studio-title"><span className="studio-icon image"><ImagePlus size={18} /></span><div><h1>{t("tool_image_title")}</h1><p>{t("tool_image_desc")}</p></div></div>
        <StudioModeToggle value={mode} onChange={setMode} />
      </div>
      {!backendAvailable && <div className="tool-backend-warning">{t("tool_backend_unavailable")}</div>}
      <div className="studio-columns">
        <form className="studio-form" onSubmit={submit}>
          <section className="studio-section">
            <div className="studio-section-head"><div><h2>{t("tool_create_image")}</h2><p>{t("tool_create_image_desc")}</p></div><span className="studio-step">01</span></div>
            <div className="tool-field"><FieldLabel>{t("tool_prompt")}</FieldLabel><Textarea rows={7} value={prompt} onChange={(event) => setPrompt(event.target.value)} placeholder={t("tool_image_prompt_ph")} /></div>
            <PromptOptimizer purpose="image" value={prompt} onChange={setPrompt} compact />
            {supportsReferences && <AssetPicker kind="image" module="image" value={references} onChange={setReferences} max={Math.min(4, maxReferences)} label={t("tool_reference_images")} />}
            {references.length > 0 && (selectedCapability?.supportsEdit ?? true) && (
              <label className="tool-checkbox"><input type="checkbox" checked={editMode} onChange={(event) => setEditMode(event.target.checked)} /><span><strong>{t("tool_edit_reference")}</strong><small>{t("tool_edit_reference_desc")}</small></span></label>
            )}
          </section>

          <section className="studio-section studio-options">
            <div className="studio-section-head"><div><h2>{t("tool_output_settings")}</h2><p>{t("tool_output_settings_desc")}</p></div><span className="studio-step">02</span></div>
            <div className="tool-option-grid">
              <div className="tool-field"><FieldLabel>{t("tool_aspect_ratio")}</FieldLabel><Select value={aspectRatio} onValueChange={setAspectRatio}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{aspectRatios.map((ratio) => <SelectItem key={ratio} value={ratio}>{ratio}</SelectItem>)}</SelectContent></Select></div>
              <div className="tool-field"><FieldLabel>{t("tool_variants")}</FieldLabel><Select value={String(variants)} onValueChange={(value) => setVariants(Number(value))}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{[1, 2, 3, 4].map((count) => <SelectItem key={count} value={String(count)}>{count}</SelectItem>)}</SelectContent></Select></div>
            </div>
            {mode === "expert" && (
              <div className="expert-options">
                <div className="tool-field"><FieldLabel>{t("tool_model")}</FieldLabel><Select value={modelKey} onValueChange={setModelKey}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="auto">{t("tool_auto_model")}</SelectItem>{models.map((item) => <SelectItem key={`${item.provider}::${item.model}`} value={`${item.provider}::${item.model}`}>{item.label || item.model} · {item.provider}</SelectItem>)}</SelectContent></Select></div>
                <div className="tool-field"><FieldLabel optional>{t("tool_negative_prompt")}</FieldLabel><Textarea rows={3} value={negativePrompt} onChange={(event) => setNegativePrompt(event.target.value)} placeholder={t("tool_negative_prompt_ph")} /></div>
              </div>
            )}
            <div className="derivative-options">
              <label className="tool-checkbox"><input type="checkbox" checked={removeBackground} onChange={(event) => setRemoveBackground(event.target.checked)} /><span><strong>{t("tool_remove_background")}</strong><small>{t("tool_keep_original")}</small></span></label>
              <label className="tool-checkbox"><input type="checkbox" checked={pixelize} onChange={(event) => setPixelize(event.target.checked)} /><span><strong>{t("tool_pixelize")}</strong><small>{t("tool_keep_original")}</small></span></label>
              {pixelize && <div className="tool-field pixel-size"><FieldLabel>{t("tool_pixel_canvas")}</FieldLabel><Input type="number" min={32} max={512} step={16} value={pixelSize} onChange={(event) => setPixelSize(Number(event.target.value))} /></div>}
            </div>
          </section>

          <div className="studio-submitbar">
            <div>{activeJob ? <><span className="spinner" /><span>{t("tool_generation_running")}</span></> : <><Layers3 size={14} /><span>{t("tool_jobs_run_background")}</span></>}</div>
            {activeJob && <Button type="button" variant="ghost" size="sm" onClick={onOpenTasks}>{t("tool_view_task")}</Button>}
            <Button type="submit" disabled={!backendAvailable || !prompt.trim()}><WandSparkles size={14} />{t("tool_generate_images")}</Button>
          </div>
        </form>

        <ResultGallery jobs={imageJobs} title={t("tool_image_results")} empty={t("tool_image_results_empty")} onOpenAssets={onOpenAssets} onNavigate={onNavigate} />
      </div>
    </div>
  );
}
