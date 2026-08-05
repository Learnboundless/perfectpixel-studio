import { Boxes, Download, Grid3X3, LayoutPanelTop, ScanLine, Scissors, Sparkles, WandSparkles } from "lucide-react";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { useI18n } from "../../i18n";
import { GenerationJob, ToolAsset, ToolchainModule, UIRegion, uid } from "../../types";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../ui/select";
import { Textarea } from "../ui/textarea";
import AssetPicker from "./AssetPicker";
import AssetThumb from "./AssetThumb";
import { assetMediaKind, isJobActive, toolchainApi } from "./api";
import PromptOptimizer from "./PromptOptimizer";
import ResultGallery from "./ResultGallery";
import { FieldLabel, StudioModeToggle, useStudioMode } from "./StudioControls";
import { useToolchain } from "./ToolchainContext";
import UIRegionEditor from "./UIRegionEditor";

type UITab = "concept" | "split" | "analyze" | "prompt";

interface Props {
  onOpenAssets: (assetId?: string) => void;
  onOpenTasks: () => void;
  onNavigate: (module: ToolchainModule) => void;
  defaultProvider?: string;
  defaultModel?: string;
}

const TAB_ICONS = { concept: LayoutPanelTop, split: Scissors, analyze: ScanLine, prompt: Sparkles };

function parseRegions(job?: GenerationJob): UIRegion[] {
  if (!job) return [];
  const rawJob = job as any;
  const candidates = [rawJob.result?.regions, rawJob.providerResponseSummary?.regions, rawJob.request?.regions];
  for (const candidate of candidates) {
    if (Array.isArray(candidate)) return candidate as UIRegion[];
  }
  for (const candidate of [rawJob.result, rawJob.providerResponseSummary]) {
    if (typeof candidate !== "string") continue;
    try {
      const parsed = JSON.parse(candidate);
      if (Array.isArray(parsed?.regions)) return parsed.regions as UIRegion[];
    } catch {
      // Non-JSON provider summaries are ignored.
    }
  }
  return [];
}

function SourcePreview({ asset, onUse }: { asset: ToolAsset; onUse: (tab: "split" | "analyze") => void }) {
  const { t } = useI18n();
  return (
    <article className="ui-concept-result">
      <AssetThumb asset={asset} />
      <div><strong title={asset.name}>{asset.name}</strong><span>{asset.width && asset.height ? `${asset.width}×${asset.height}` : t("tool_image")}</span></div>
      <Button variant="outline" size="sm" onClick={() => onUse("split")}><Scissors size={12} />{t("tool_ui_split")}</Button>
      <Button variant="ghost" size="sm" onClick={() => onUse("analyze")}><ScanLine size={12} />{t("tool_ui_analyze")}</Button>
    </article>
  );
}

export default function UIStudio({ onOpenAssets, onOpenTasks, onNavigate, defaultProvider, defaultModel }: Props) {
  const { t } = useI18n();
  const { jobs, assets, capabilities, createJob, notify, backendAvailable } = useToolchain();
  const [mode, setMode] = useStudioMode("ui");
  const [tab, setTab] = useState<UITab>("concept");
  const [conceptPrompt, setConceptPrompt] = useState("");
  const [conceptReferences, setConceptReferences] = useState<string[]>([]);
  const [conceptRatio, setConceptRatio] = useState("16:9");
  const [conceptModel, setConceptModel] = useState("auto");
  const [maskRegions, setMaskRegions] = useState<UIRegion[]>([]);
  const [splitSource, setSplitSource] = useState<string[]>([]);
  const [splitMode, setSplitMode] = useState<"auto" | "grid" | "manual">("auto");
  const [splitRegions, setSplitRegions] = useState<UIRegion[]>([]);
  const [gridRows, setGridRows] = useState(4);
  const [gridColumns, setGridColumns] = useState(4);
  const [backgroundColor, setBackgroundColor] = useState("#000000");
  const [trimTransparent, setTrimTransparent] = useState(true);
  const [analysisSource, setAnalysisSource] = useState<string[]>([]);
  const [analysisJobId, setAnalysisJobId] = useState("");
  const [analysisRegions, setAnalysisRegions] = useState<UIRegion[]>([]);
  const [analysisModel, setAnalysisModel] = useState("auto");
  const [uiPrompt, setUIPrompt] = useState("");
  const [working, setWorking] = useState(false);

  const imageAssets = assets.filter((asset) => assetMediaKind(asset) === "image");
  const sourceForMask = assets.find((asset) => asset.id === conceptReferences[0]);
  const splitAsset = assets.find((asset) => asset.id === splitSource[0]);
  const analysisAsset = assets.find((asset) => asset.id === analysisSource[0]);
  const conceptJobs = useMemo(() => jobs.filter((job) => job.kind === "image" && (job.sourceModule === "ui" || job.request?.sourceModule === "ui")), [jobs]);
  const splitJobs = useMemo(() => jobs.filter((job) => job.kind === "ui_split"), [jobs]);
  const analysisJobs = useMemo(() => jobs.filter((job) => job.kind === "ui_analysis"), [jobs]);
  const analysisJob = jobs.find((job) => job.id === analysisJobId);
  const conceptOutputs = conceptJobs.flatMap((job) => job.outputAssetIds ?? []).map((id) => assets.find((asset) => asset.id === id)).filter(Boolean) as ToolAsset[];
  const imageModels = capabilities.image ?? capabilities.imageModels ?? [];
  const visionModels = capabilities.vision ?? capabilities.visionModels ?? [];
  const conceptCapability = conceptModel === "auto"
    ? imageModels.find((item) => item.provider === defaultProvider && item.model === defaultModel)
    : imageModels.find((item) => `${item.provider}::${item.model}` === conceptModel);
  const canMask = conceptCapability?.supportsMask ?? true;

  useEffect(() => {
    const receive = (event: Event) => {
      const detail = (event as CustomEvent).detail;
      if (detail?.targetModule !== "ui") return;
      const sourceAssetId = detail?.prefill?.sourceAssetId ?? detail?.assetId;
      if (sourceAssetId) {
        setConceptReferences([String(sourceAssetId)]);
        setTab("concept");
      }
    };
    window.addEventListener("toolchain-asset-sent", receive);
    return () => window.removeEventListener("toolchain-asset-sent", receive);
  }, []);

  useEffect(() => {
    if (analysisJob?.status !== "completed") return;
    const next = parseRegions(analysisJob);
    if (next.length) setAnalysisRegions(next);
  }, [analysisJob]);

  useEffect(() => {
    if (splitMode !== "grid" || !splitAsset) return;
    const width = Math.max(1, splitAsset.width ?? 1024);
    const height = Math.max(1, splitAsset.height ?? 768);
    const next: UIRegion[] = [];
    for (let row = 0; row < gridRows; row += 1) {
      for (let column = 0; column < gridColumns; column += 1) {
        next.push({
          id: `grid-${row}-${column}`,
          name: `${t("tool_cell")} ${row + 1}-${column + 1}`,
          type: "slice",
          bounds: {
            x: Math.round((column * width) / gridColumns),
            y: Math.round((row * height) / gridRows),
            width: Math.round(width / gridColumns),
            height: Math.round(height / gridRows),
          },
          zIndex: next.length,
          confidence: 1,
          exportEnabled: true,
        });
      }
    }
    setSplitRegions(next);
  }, [splitMode, splitAsset?.id, gridRows, gridColumns, t]);

  const generateConcept = async (event: FormEvent) => {
    event.preventDefault();
    if (!conceptPrompt.trim()) return;
    const [provider, model] = conceptModel === "auto" ? [undefined, undefined] : conceptModel.split("::");
    try {
      await createJob("image", {
        sourceModule: "ui",
        purpose: "ui_concept",
        prompt: conceptPrompt.trim(),
        referenceAssetIds: conceptReferences,
        maskRegions: sourceForMask && canMask ? maskRegions.filter((region) => region.exportEnabled).map((region) => region.bounds) : [],
        mode: sourceForMask && canMask && maskRegions.some((region) => region.exportEnabled) ? "masked_edit" : conceptReferences.length ? "reference" : "text",
        aspectRatio: conceptRatio,
        variants: 1,
        provider,
        model,
      });
      notify("success", t("tool_job_created"));
    } catch (error) {
      notify("error", String(error));
    }
  };

  const createSplit = async () => {
    if (!splitAsset) return;
    setWorking(true);
    try {
      await createJob("ui_split", {
        sourceModule: "ui",
        sourceAssetId: splitAsset.id,
        mode: splitMode === "grid" ? "manual" : splitMode,
        rows: splitMode === "grid" ? gridRows : undefined,
        columns: splitMode === "grid" ? gridColumns : undefined,
        backgroundColor: splitMode === "auto" ? backgroundColor : undefined,
        regions: splitMode === "manual" || splitMode === "grid" ? splitRegions.filter((region) => region.exportEnabled) : undefined,
        trimTransparent,
      });
      notify("success", t("tool_job_created"));
    } catch (error) {
      notify("error", String(error));
    } finally {
      setWorking(false);
    }
  };

  const analyze = async () => {
    if (!analysisAsset) return;
    setWorking(true);
    setAnalysisRegions([]);
    try {
      const [provider, model] = analysisModel === "auto" ? [undefined, undefined] : analysisModel.split("::");
      const job = await createJob("ui_analysis", { sourceModule: "ui", sourceAssetId: analysisAsset.id, prompt: uiPrompt.trim(), provider, model });
      setAnalysisJobId(job.id);
      notify("success", t("tool_job_created"));
    } catch (error) {
      notify("error", String(error));
    } finally {
      setWorking(false);
    }
  };

  const exportProject = async () => {
    if (!analysisAsset || !analysisRegions.length) return;
    setWorking(true);
    try {
      const path = await toolchainApi.exportUIProject({ sourceAssetId: analysisAsset.id, regions: analysisRegions.filter((region) => region.exportEnabled), prompt: uiPrompt });
      notify("success", t("tool_exported", { path }));
    } catch (error) {
      notify("error", String(error));
    } finally {
      setWorking(false);
    }
  };

  const useConcept = (asset: ToolAsset, target: "split" | "analyze") => {
    if (target === "split") setSplitSource([asset.id]);
    else setAnalysisSource([asset.id]);
    setTab(target);
  };

  const addDefaultRegion = () => {
    if (!analysisAsset) return;
    setAnalysisRegions((current) => [...current, {
      id: uid("ui-region"), name: `${t("tool_region")} ${current.length + 1}`, type: "component",
      bounds: { x: 0, y: 0, width: Math.round((analysisAsset.width ?? 1024) / 3), height: Math.round((analysisAsset.height ?? 768) / 4) },
      zIndex: current.length, confidence: 1, exportEnabled: true,
    }]);
  };

  return (
    <div className="studio-page ui-studio">
      <div className="studio-toolbar">
        <div className="studio-title"><span className="studio-icon ui"><LayoutPanelTop size={18} /></span><div><h1>{t("tool_ui_title")}</h1><p>{t("tool_ui_desc")}</p></div></div>
        <StudioModeToggle value={mode} onChange={setMode} />
      </div>
      {!backendAvailable && <div className="tool-backend-warning">{t("tool_backend_unavailable")}</div>}
      <nav className="ui-studio-tabs" aria-label={t("tool_ui_tools")}>
        {(["concept", "split", "analyze", "prompt"] as UITab[]).map((item) => {
          const Icon = TAB_ICONS[item];
          return <button key={item} type="button" className={tab === item ? "active" : ""} onClick={() => setTab(item)}><Icon size={14} />{t(`tool_ui_${item}`)}</button>;
        })}
      </nav>

      <div className="ui-tab-panels">
        <section className="ui-tab-panel" hidden={tab !== "concept"}>
          <form className="ui-concept-form" onSubmit={generateConcept}>
            <div className="studio-section-head"><div><h2>{t("tool_ui_concept_title")}</h2><p>{t("tool_ui_concept_desc")}</p></div></div>
            <div className="tool-field"><FieldLabel>{t("tool_prompt")}</FieldLabel><Textarea rows={6} value={conceptPrompt} onChange={(event) => setConceptPrompt(event.target.value)} placeholder={t("tool_ui_prompt_ph")} /></div>
            <PromptOptimizer purpose="ui" value={conceptPrompt} onChange={setConceptPrompt} compact />
            <AssetPicker kind="image" module="ui" value={conceptReferences} onChange={(ids) => { setConceptReferences(ids); if (!ids.length) setMaskRegions([]); }} max={1} label={t("tool_ui_reference_concept")} />
            {sourceForMask && canMask && (
              <div className="ui-mask-editor"><div className="studio-section-head"><div><h3>{t("tool_mask_edit")}</h3><p>{t("tool_mask_edit_desc")}</p></div></div><UIRegionEditor asset={sourceForMask} regions={maskRegions} onChange={setMaskRegions} /></div>
            )}
            <div className="tool-option-grid">
              <div className="tool-field"><FieldLabel>{t("tool_aspect_ratio")}</FieldLabel><Select value={conceptRatio} onValueChange={setConceptRatio}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{["16:9", "9:16", "1:1", "4:3", "3:4"].map((ratio) => <SelectItem key={ratio} value={ratio}>{ratio}</SelectItem>)}</SelectContent></Select></div>
              {mode === "expert" && <div className="tool-field"><FieldLabel>{t("tool_model")}</FieldLabel><Select value={conceptModel} onValueChange={setConceptModel}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="auto">{t("tool_auto_model")}</SelectItem>{imageModels.map((item) => <SelectItem key={`${item.provider}::${item.model}`} value={`${item.provider}::${item.model}`}>{item.label || item.model} · {item.provider}</SelectItem>)}</SelectContent></Select></div>}
            </div>
            <div className="ui-panel-actions"><span>{conceptJobs.some(isJobActive) ? t("tool_generation_running") : t("tool_original_kept")}</span>{conceptJobs.some(isJobActive) && <Button type="button" variant="ghost" size="sm" onClick={onOpenTasks}>{t("tool_view_task")}</Button>}<Button type="submit" disabled={!backendAvailable || !conceptPrompt.trim()}><WandSparkles size={14} />{sourceForMask && canMask && maskRegions.some((region) => region.exportEnabled) ? t("tool_regenerate_region") : t("tool_generate_concept")}</Button></div>
          </form>
          <div className="ui-concept-results">
            <div className="studio-section-head"><div><h2>{t("tool_concept_versions")}</h2><p>{t("tool_concept_versions_desc")}</p></div><Button variant="ghost" size="sm" onClick={() => onOpenAssets()}>{t("tool_view_all_assets")}</Button></div>
            {conceptOutputs.length ? conceptOutputs.map((asset) => <SourcePreview key={asset.id} asset={asset} onUse={(target) => useConcept(asset, target)} />) : <div className="tool-empty"><LayoutPanelTop size={24} /><strong>{t("tool_concepts_empty")}</strong><span>{t("tool_concepts_empty_desc")}</span></div>}
          </div>
        </section>

        <section className="ui-tab-panel ui-split-panel" hidden={tab !== "split"}>
          <div className="ui-split-source-column">
            <div className="ui-tool-head"><div><h2>{t("tool_split_title")}</h2><p>{t("tool_split_desc")}</p></div><Button disabled={!splitAsset || working || ((splitMode === "manual" || splitMode === "grid") && !splitRegions.some((region) => region.exportEnabled))} onClick={createSplit}><Scissors size={14} />{t("tool_split_assets")}</Button></div>
            <AssetPicker kind="image" module="ui" value={splitSource} onChange={(ids) => { setSplitSource(ids); setSplitRegions([]); }} max={1} label={t("tool_source_image")} />
            <div className="tool-segmented split-modes" role="group" aria-label={t("tool_split_mode")}>
              <button type="button" className={splitMode === "auto" ? "active" : ""} onClick={() => setSplitMode("auto")}><Boxes size={13} />{t("tool_split_auto")}</button>
              <button type="button" className={splitMode === "grid" ? "active" : ""} onClick={() => setSplitMode("grid")}><Grid3X3 size={13} />{t("tool_split_grid")}</button>
              <button type="button" className={splitMode === "manual" ? "active" : ""} onClick={() => setSplitMode("manual")}><ScanLine size={13} />{t("tool_split_manual")}</button>
            </div>
            {splitMode === "auto" && <div className="tool-option-grid"><div className="tool-field"><FieldLabel>{t("tool_background_color")}</FieldLabel><div className="color-field"><input type="color" value={backgroundColor} onChange={(event) => setBackgroundColor(event.target.value)} /><Input value={backgroundColor} onChange={(event) => setBackgroundColor(event.target.value)} /></div></div><label className="tool-checkbox"><input type="checkbox" checked={trimTransparent} onChange={(event) => setTrimTransparent(event.target.checked)} /><span><strong>{t("tool_trim_transparent")}</strong></span></label></div>}
            {splitMode === "grid" && <div className="tool-option-grid"><div className="tool-field"><FieldLabel>{t("tool_rows")}</FieldLabel><Input type="number" min={1} max={32} value={gridRows} onChange={(event) => setGridRows(Number(event.target.value))} /></div><div className="tool-field"><FieldLabel>{t("tool_columns")}</FieldLabel><Input type="number" min={1} max={32} value={gridColumns} onChange={(event) => setGridColumns(Number(event.target.value))} /></div></div>}
            <div className="ui-split-source-stage">
              {splitAsset ? (splitMode === "auto" ? <div className="ui-source-preview"><AssetThumb asset={splitAsset} /><p>{t("tool_auto_split_hint")}</p></div> : <UIRegionEditor asset={splitAsset} regions={splitRegions} onChange={setSplitRegions} />) : <div className="tool-empty"><Scissors size={24} /><strong>{t("tool_choose_source_first")}</strong></div>}
            </div>
          </div>
          <ResultGallery jobs={splitJobs} title={t("tool_split_results")} empty={t("tool_split_results_empty")} onOpenAssets={onOpenAssets} onNavigate={onNavigate} />
        </section>

        <section className="ui-tab-panel" hidden={tab !== "analyze"}>
          <div className="ui-tool-head"><div><h2>{t("tool_analyze_title")}</h2><p>{t("tool_analyze_desc")}</p></div><div><Button variant="outline" disabled={!analysisAsset || working} onClick={analyze}><ScanLine size={14} />{analysisJob && isJobActive(analysisJob) ? t("tool_analyzing") : t("tool_analyze_ui")}</Button><Button disabled={!analysisAsset || !analysisRegions.some((region) => region.exportEnabled) || working} onClick={exportProject}><Download size={14} />{t("tool_export_ui")}</Button></div></div>
          <AssetPicker kind="image" module="ui" value={analysisSource} onChange={(ids) => { setAnalysisSource(ids); setAnalysisRegions([]); setAnalysisJobId(""); }} max={1} label={t("tool_source_image")} />
          {mode === "expert" && <div className="tool-field"><FieldLabel>{t("tool_model")}</FieldLabel><Select value={analysisModel} onValueChange={setAnalysisModel}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="auto">{t("tool_auto_model")}</SelectItem>{visionModels.map((item) => <SelectItem key={`${item.provider}::${item.model}`} value={`${item.provider}::${item.model}`}>{item.label || item.model} · {item.provider}</SelectItem>)}</SelectContent></Select></div>}
          <div className="ui-inference-note">{t("tool_ui_inference_note")}</div>
          {analysisAsset ? (
            <>
              {analysisRegions.length ? <UIRegionEditor asset={analysisAsset} regions={analysisRegions} onChange={setAnalysisRegions} inferred /> : <div className="tool-empty analyze-empty"><ScanLine size={24} /><strong>{analysisJob && isJobActive(analysisJob) ? t("tool_analyzing") : t("tool_analysis_empty")}</strong><span>{analysisJob?.error || t("tool_analysis_empty_desc")}</span></div>}
              <div className="ui-panel-actions"><span>{t("tool_regions_count", { count: analysisRegions.length })}</span><Button variant="ghost" size="sm" onClick={addDefaultRegion} disabled={!analysisAsset}>{t("tool_add_region")}</Button><Button variant="ghost" size="sm" onClick={onOpenTasks}>{t("tool_view_task")}</Button></div>
            </>
          ) : <div className="tool-empty"><ScanLine size={24} /><strong>{t("tool_choose_source_first")}</strong></div>}
        </section>

        <section className="ui-tab-panel prompt-tab" hidden={tab !== "prompt"}>
          <div className="ui-tool-head"><div><h2>{t("tool_prompt_lab_title")}</h2><p>{t("tool_prompt_lab_desc")}</p></div></div>
          <div className="tool-field"><FieldLabel>{t("tool_prompt")}</FieldLabel><Textarea rows={9} value={uiPrompt} onChange={(event) => setUIPrompt(event.target.value)} placeholder={t("tool_ui_prompt_ph")} /></div>
          <PromptOptimizer purpose="ui" value={uiPrompt} onChange={setUIPrompt} />
          <div className="ui-panel-actions"><span>{t("tool_prompt_send_desc")}</span><Button variant="outline" size="sm" disabled={!uiPrompt.trim()} onClick={() => { setConceptPrompt(uiPrompt); setTab("concept"); }}>{t("tool_send_to_concept")}</Button></div>
        </section>
      </div>
    </div>
  );
}
