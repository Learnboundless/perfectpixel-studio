import { Film, Play, Video, WandSparkles } from "lucide-react";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { useI18n } from "../../i18n";
import { ModelCapability, ToolchainModule } from "../../types";
import { Button } from "../ui/button";
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

function supports(capability: ModelCapability | undefined, field: keyof ModelCapability, fallback = true) {
  const value = capability?.[field];
  return typeof value === "boolean" ? value : fallback;
}

export default function VideoStudio({ onOpenAssets, onOpenTasks, onNavigate, defaultProvider, defaultModel }: Props) {
  const { t } = useI18n();
  const { jobs, capabilities, createJob, notify, backendAvailable } = useToolchain();
  const [mode, setMode] = useStudioMode("video");
  const [inputMode, setInputMode] = useState<"text" | "image">("text");
  const [prompt, setPrompt] = useState("");
  const [referenceMode, setReferenceMode] = useState("first_frame");
  const [references, setReferences] = useState<string[]>([]);
  const [lastFrame, setLastFrame] = useState<string[]>([]);
  const [duration, setDuration] = useState(4);
  const [aspectRatio, setAspectRatio] = useState("16:9");
  const [resolution, setResolution] = useState("720p");
  const [modelKey, setModelKey] = useState("auto");
  const [negativePrompt, setNegativePrompt] = useState("");
  const [audio, setAudio] = useState(false);
  const [watermark, setWatermark] = useState(false);
  const videoJobs = useMemo(() => jobs.filter((job) => job.kind === "video"), [jobs]);
  const activeJob = videoJobs.find(isJobActive);
  const models = capabilities.video ?? capabilities.videoModels ?? [];
  const selectedCapability = modelKey === "auto"
    ? models.find((item) => item.provider === defaultProvider && item.model === defaultModel)
    : models.find((item) => `${item.provider}::${item.model}` === modelKey);
  const maxReferences = selectedCapability?.maxReferenceImages ?? (supports(selectedCapability, "supportsMultipleReferences") ? 4 : 1);
  const ratios = selectedCapability?.aspectRatios?.length ? selectedCapability.aspectRatios : ["16:9", "9:16", "1:1", "4:3", "3:4"];
  const durations = selectedCapability?.durations?.length ? selectedCapability.durations : [4, 5, 8, 10];
  const resolutions = selectedCapability?.resolutions?.length ? selectedCapability.resolutions : ["480p", "720p", "1080p"];
  const supportsTextInput = supports(selectedCapability, "supportsText");
  const supportsImageInput = supports(selectedCapability, "supportsImageReference");
  const referenceModes = [
    "none",
    ...(supports(selectedCapability, "supportsFirstFrame", false) ? ["first_frame"] : []),
    ...(supports(selectedCapability, "supportsLastFrame", false) ? ["start_end_frame"] : []),
    ...(supports(selectedCapability, "supportsIdentityReference", false) ? ["identity_reference"] : []),
    ...(supports(selectedCapability, "supportsMultipleReferences", false) ? ["multi_reference"] : []),
  ];

  useEffect(() => {
    const receive = (event: Event) => {
      const detail = (event as CustomEvent).detail;
      if (detail?.targetModule !== "video") return;
      const ids = detail?.prefill?.referenceAssetIds;
      if (Array.isArray(ids) && ids.length) {
        setInputMode("image");
        setReferences(ids.slice(0, maxReferences));
      }
    };
    window.addEventListener("toolchain-asset-sent", receive);
    return () => window.removeEventListener("toolchain-asset-sent", receive);
  }, [maxReferences]);

  useEffect(() => {
    if (!supportsImageInput && inputMode === "image") {
      setInputMode("text");
      setReferences([]);
      setLastFrame([]);
    } else if (!supportsTextInput && inputMode === "text" && supportsImageInput) {
      setInputMode("image");
    }
  }, [supportsImageInput, supportsTextInput, inputMode]);

  useEffect(() => {
    if (!referenceModes.includes(referenceMode)) setReferenceMode(referenceModes[0]);
  }, [referenceModes, referenceMode]);

  useEffect(() => {
    if (!ratios.includes(aspectRatio)) setAspectRatio(ratios[0]);
    if (!durations.includes(duration)) setDuration(durations[0]);
    if (!resolutions.includes(resolution)) setResolution(resolutions[0]);
  }, [ratios, durations, resolutions, aspectRatio, duration, resolution]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!prompt.trim()) return;
    const [provider, model] = modelKey === "auto" ? [undefined, undefined] : modelKey.split("::");
    try {
      await createJob("video", {
        sourceModule: "video",
        prompt: prompt.trim(),
        negativePrompt: negativePrompt.trim(),
        mode: inputMode,
        referenceMode: inputMode === "image" ? referenceMode : "none",
        referenceAssetIds: inputMode === "image" ? references : [],
        lastFrameAssetId: inputMode === "image" && referenceMode === "start_end_frame" ? lastFrame[0] : undefined,
        durationSeconds: duration,
        aspectRatio,
        resolution,
        provider,
        model,
        audio: supports(selectedCapability, "supportsAudio") ? audio : false,
        watermark: supports(selectedCapability, "supportsWatermark") ? watermark : false,
      });
      notify("success", t("tool_job_created"));
    } catch (error) {
      notify("error", String(error));
    }
  };

  return (
    <div className="studio-page video-studio">
      <div className="studio-toolbar">
        <div className="studio-title"><span className="studio-icon video"><Video size={18} /></span><div><h1>{t("tool_video_title")}</h1><p>{t("tool_video_desc")}</p></div></div>
        <StudioModeToggle value={mode} onChange={setMode} />
      </div>
      {!backendAvailable && <div className="tool-backend-warning">{t("tool_backend_unavailable")}</div>}
      <div className="studio-columns">
        <form className="studio-form" onSubmit={submit}>
          <section className="studio-section">
            <div className="studio-section-head"><div><h2>{t("tool_create_video")}</h2><p>{t("tool_create_video_desc")}</p></div><span className="studio-step">01</span></div>
            <div className={`tool-segmented${supportsTextInput && supportsImageInput ? "" : " single"}`} role="group" aria-label={t("tool_video_input_mode")}>
              {supportsTextInput && <button type="button" className={inputMode === "text" ? "active" : ""} onClick={() => setInputMode("text")}>{t("tool_text_to_video")}</button>}
              {supportsImageInput && <button type="button" className={inputMode === "image" ? "active" : ""} onClick={() => setInputMode("image")}>{t("tool_image_to_video")}</button>}
            </div>
            <div className="tool-field"><FieldLabel>{t("tool_prompt")}</FieldLabel><Textarea rows={7} value={prompt} onChange={(event) => setPrompt(event.target.value)} placeholder={t("tool_video_prompt_ph")} /></div>
            <PromptOptimizer purpose="video" value={prompt} onChange={setPrompt} compact />
            {inputMode === "image" && (
              <div className="video-reference-block">
                <div className="tool-field"><FieldLabel>{t("tool_reference_mode")}</FieldLabel><Select value={referenceMode} onValueChange={setReferenceMode}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>
                  <SelectItem value="none">{t("tool_ref_image")}</SelectItem>
                  {supports(selectedCapability, "supportsFirstFrame", false) && <SelectItem value="first_frame">{t("tool_ref_first_frame")}</SelectItem>}
                  {supports(selectedCapability, "supportsLastFrame", false) && <SelectItem value="start_end_frame">{t("tool_ref_start_end")}</SelectItem>}
                  {supports(selectedCapability, "supportsIdentityReference", false) && <SelectItem value="identity_reference">{t("tool_ref_identity")}</SelectItem>}
                  {supports(selectedCapability, "supportsMultipleReferences", false) && <SelectItem value="multi_reference">{t("tool_ref_multi")}</SelectItem>}
                </SelectContent></Select></div>
                <AssetPicker kind="image" module="video" value={references} onChange={setReferences} max={referenceMode === "multi_reference" || referenceMode === "identity_reference" ? maxReferences : 1} label={referenceMode === "identity_reference" ? t("tool_identity_images") : t("tool_reference_images")} preview />
                {referenceMode === "start_end_frame" && <AssetPicker kind="image" module="video" value={lastFrame} onChange={setLastFrame} max={1} label={t("tool_last_frame")} preview />}
              </div>
            )}
          </section>

          <section className="studio-section studio-options">
            <div className="studio-section-head"><div><h2>{t("tool_video_settings")}</h2><p>{t("tool_video_settings_desc")}</p></div><span className="studio-step">02</span></div>
            <div className="tool-option-grid three">
              <div className="tool-field"><FieldLabel>{t("tool_duration")}</FieldLabel><Select value={String(duration)} onValueChange={(value) => setDuration(Number(value))}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{durations.map((seconds) => <SelectItem key={seconds} value={String(seconds)}>{seconds}s</SelectItem>)}</SelectContent></Select></div>
              <div className="tool-field"><FieldLabel>{t("tool_aspect_ratio")}</FieldLabel><Select value={aspectRatio} onValueChange={setAspectRatio}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{ratios.map((ratio) => <SelectItem key={ratio} value={ratio}>{ratio}</SelectItem>)}</SelectContent></Select></div>
              <div className="tool-field"><FieldLabel>{t("tool_resolution")}</FieldLabel><Select value={resolution} onValueChange={setResolution}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{resolutions.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}</SelectContent></Select></div>
            </div>
            {mode === "expert" && (
              <div className="expert-options">
                <div className="tool-field"><FieldLabel>{t("tool_model")}</FieldLabel><Select value={modelKey} onValueChange={setModelKey}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="auto">{t("tool_auto_model")}</SelectItem>{models.map((item) => <SelectItem key={`${item.provider}::${item.model}`} value={`${item.provider}::${item.model}`}>{item.label || item.model} · {item.provider}</SelectItem>)}</SelectContent></Select></div>
                <div className="tool-field"><FieldLabel optional>{t("tool_negative_prompt")}</FieldLabel><Textarea rows={3} value={negativePrompt} onChange={(event) => setNegativePrompt(event.target.value)} placeholder={t("tool_negative_prompt_ph")} /></div>
                <div className="tool-inline-checks">
                  {supports(selectedCapability, "supportsAudio") && <label className="tool-checkbox"><input type="checkbox" checked={audio} onChange={(event) => setAudio(event.target.checked)} /><span><strong>{t("tool_generate_audio")}</strong></span></label>}
                  {supports(selectedCapability, "supportsWatermark") && <label className="tool-checkbox"><input type="checkbox" checked={watermark} onChange={(event) => setWatermark(event.target.checked)} /><span><strong>{t("tool_watermark")}</strong></span></label>}
                </div>
              </div>
            )}
          </section>

          <div className="studio-submitbar">
            <div>{activeJob ? <><span className="spinner" /><span>{t("tool_generation_running")}</span></> : <><Film size={14} /><span>{t("tool_jobs_run_background")}</span></>}</div>
            {activeJob && <Button type="button" variant="ghost" size="sm" onClick={onOpenTasks}>{t("tool_view_task")}</Button>}
            <Button type="submit" disabled={!backendAvailable || !prompt.trim() || (inputMode === "image" && references.length === 0)}><WandSparkles size={14} />{t("tool_generate_video")}</Button>
          </div>
        </form>

        <ResultGallery jobs={videoJobs} title={t("tool_video_results")} empty={t("tool_video_results_empty")} onOpenAssets={onOpenAssets} onNavigate={onNavigate} />
      </div>
    </div>
  );
}
