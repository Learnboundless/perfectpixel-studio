import { Download, ExternalLink, Image as ImageIcon, PanelsTopLeft, RotateCcw, Send, Video } from "lucide-react";
import { useI18n } from "../../i18n";
import { GenerationJob, ToolAsset, ToolchainModule } from "../../types";
import { Button } from "../ui/button";
import AssetThumb from "./AssetThumb";
import { assetMediaKind, assetURL, dispatchAssetHandoff, toolchainApi } from "./api";
import { useToolchain } from "./ToolchainContext";

interface Props {
  jobs: GenerationJob[];
  title: string;
  empty: string;
  onOpenAssets: (assetId?: string) => void;
  onNavigate: (module: ToolchainModule) => void;
}

export default function ResultGallery({ jobs, title, empty, onOpenAssets, onNavigate }: Props) {
  const { t } = useI18n();
  const { assets, retryJob, notify } = useToolchain();
  const outputs = jobs.flatMap((job) => (job.outputAssetIds ?? []).map((id) => ({
    job,
    asset: assets.find((asset) => asset.id === id),
  }))).filter((item): item is { job: GenerationJob; asset: ToolAsset } => !!item.asset);

  const send = async (asset: ToolAsset, targetModule: ToolchainModule) => {
    try {
      const result = await toolchainApi.sendAssetToModule({ assetId: asset.id, targetModule });
      dispatchAssetHandoff(targetModule, asset.id, result);
      notify("success", t("tool_asset_sent", { module: t(`tool_nav_${targetModule}`) }));
      onNavigate(targetModule);
    } catch (error) {
      notify("error", String(error));
    }
  };

  return (
    <section className="studio-results" aria-label={title}>
      <div className="studio-section-head"><div><h2>{title}</h2><p>{t("tool_results_desc")}</p></div><Button variant="ghost" size="sm" onClick={() => onOpenAssets()}><ExternalLink size={13} />{t("tool_view_all_assets")}</Button></div>
      {outputs.length ? (
        <div className="result-grid">
          {outputs.map(({ asset, job }) => {
            const kind = assetMediaKind(asset);
            return (
              <article key={`${job.id}-${asset.id}`} className="result-card">
                {kind === "video" ? <video className="result-video" src={assetURL(asset)} controls preload="metadata" /> : <AssetThumb asset={asset} />}
                <div className="result-card-copy"><strong title={asset.name}>{asset.name}</strong><span>{asset.provider || t("tool_local")}{asset.model ? ` · ${asset.model}` : ""}</span></div>
                <div className="result-card-actions">
                  <Button variant="ghost" size="icon-sm" title={t("tool_send_to_animation")} onClick={() => kind === "video" ? onOpenAssets(asset.id) : send(asset, "animation")}><Send size={12} /></Button>
                  {kind === "image" && <Button variant="ghost" size="icon-sm" title={t("tool_send_to_video")} onClick={() => send(asset, "video")}><Video size={12} /></Button>}
                  {kind === "image" && <Button variant="ghost" size="icon-sm" title={t("tool_send_to_ui")} onClick={() => send(asset, "ui")}><PanelsTopLeft size={12} /></Button>}
                  <Button variant="ghost" size="icon-sm" title={t("tool_retry_job")} onClick={() => retryJob(job.id).catch((error) => notify("error", String(error)))}><RotateCcw size={12} /></Button>
                  <Button variant="ghost" size="icon-sm" title={t("export")} onClick={() => toolchainApi.exportAsset(asset.id).catch((error) => notify("error", String(error)))}><Download size={12} /></Button>
                </div>
              </article>
            );
          })}
        </div>
      ) : (
        <div className="tool-empty studio-empty"><ImageIcon size={24} /><strong>{empty}</strong><span>{t("tool_results_empty_desc")}</span></div>
      )}
      {jobs.some((job) => job.status === "failed") && (
        <div className="result-failures">
          {jobs.filter((job) => job.status === "failed").slice(0, 2).map((job) => (
            <div key={job.id}><span>{job.error || t("tool_job_failed")}</span><Button variant="ghost" size="sm" onClick={() => retryJob(job.id).catch((error) => notify("error", String(error)))}><RotateCcw size={12} />{t("tool_retry_job")}</Button></div>
          ))}
        </div>
      )}
    </section>
  );
}
