import { Clapperboard, ImagePlus, PanelsTopLeft, Video } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { ListGalleryImages, LoadImageFull } from "../../../wailsjs/go/main/App";
import { useI18n } from "../../i18n";
import { GenerationJob, ToolAsset, ToolAssetKind, ToolchainModule } from "../../types";
import { assetMediaKind, assetModule, assetURL, isJobActive } from "./api";
import { useToolchain } from "./ToolchainContext";

type WorkspaceModule = ToolchainModule;

interface Props {
  active?: boolean;
  onNavigate: (module: ToolchainModule) => void;
  onOpenAssets?: () => void;
  onOpenTasks?: () => void;
}

interface ModuleDefinition {
  id: WorkspaceModule;
  icon: typeof Clapperboard;
  eyebrowKey: string;
  titleKey: string;
  descriptionKey: string;
  mediaLabelKey: string;
  mediaKinds: ToolAssetKind[];
  fallbackKind: "image" | "video";
  fallbackSource: string;
  posterSource?: string;
}

const MODULES: ModuleDefinition[] = [
  {
    id: "animation",
    icon: Clapperboard,
    eyebrowKey: "tool_home_animation_eyebrow",
    titleKey: "tool_home_animation_title",
    descriptionKey: "tool_home_animation_desc",
    mediaLabelKey: "tool_home_media_animation",
    mediaKinds: ["image", "video"],
    fallbackKind: "image",
    fallbackSource: "/home/animation.gif",
  },
  {
    id: "image",
    icon: ImagePlus,
    eyebrowKey: "tool_home_image_eyebrow",
    titleKey: "tool_home_image_title",
    descriptionKey: "tool_home_image_desc",
    mediaLabelKey: "tool_home_media_image",
    mediaKinds: ["image"],
    fallbackKind: "image",
    fallbackSource: "/home/image.webp",
  },
  {
    id: "video",
    icon: Video,
    eyebrowKey: "tool_home_video_eyebrow",
    titleKey: "tool_home_video_title",
    descriptionKey: "tool_home_video_desc",
    mediaLabelKey: "tool_home_media_video",
    mediaKinds: ["video"],
    fallbackKind: "video",
    fallbackSource: "/home/video.mp4",
    posterSource: "/home/video-poster.webp",
  },
  {
    id: "ui",
    icon: PanelsTopLeft,
    eyebrowKey: "tool_home_ui_eyebrow",
    titleKey: "tool_home_ui_title",
    descriptionKey: "tool_home_ui_desc",
    mediaLabelKey: "tool_home_media_ui",
    mediaKinds: ["image"],
    fallbackKind: "image",
    fallbackSource: "/home/ui.webp",
  },
];

function assetTimestamp(asset: ToolAsset): number {
  const timestamp = Date.parse(asset.updatedAt || asset.createdAt);
  return Number.isFinite(timestamp) ? timestamp : 0;
}

function latestAssetForModule(assets: ToolAsset[], definition: ModuleDefinition): ToolAsset | undefined {
  return assets
    .filter((asset) => assetModule(asset) === definition.id && definition.mediaKinds.includes(assetMediaKind(asset)))
    .reduce<ToolAsset | undefined>((latest, asset) => {
      if (!latest || assetTimestamp(asset) > assetTimestamp(latest)) return asset;
      return latest;
    }, undefined);
}

function jobBelongsToModule(job: GenerationJob, module: WorkspaceModule): boolean {
  if (job.sourceModule) return job.sourceModule === module;
  if (module === "image") return job.kind === "image";
  if (module === "video") return job.kind === "video";
  if (module === "ui") return job.kind === "ui_analysis" || job.kind === "ui_split";
  return false;
}

function ModuleMedia({ definition, asset, sourceOverride, label, openLabel, onOpen }: { definition: ModuleDefinition; asset?: ToolAsset; sourceOverride?: string; label: string; openLabel: string; onOpen: () => void }) {
  const kind = sourceOverride ? "image" : asset ? assetMediaKind(asset) : definition.fallbackKind;
  const source = sourceOverride || (asset ? assetURL(asset) : definition.fallbackSource);

  return (
    <button
      type="button"
      className={`home-module-media home-module-media-${kind}`}
      onClick={onOpen}
      aria-label={openLabel}
      title={openLabel}
    >
      <div className="home-module-media-frame">
        {kind === "video" ? (
          <video src={source} poster={asset ? undefined : definition.posterSource} muted loop autoPlay playsInline preload="metadata" />
        ) : (
          <img src={source} alt="" loading="lazy" />
        )}
      </div>
      <span className="home-module-media-label">{label}</span>
    </button>
  );
}

export default function HomeStudio({ active = true, onNavigate, onOpenAssets, onOpenTasks }: Props) {
  const { t } = useI18n();
  const { assets, jobs } = useToolchain();
  const [animationGif, setAnimationGif] = useState("");
  const latestAssets = useMemo(
    () => new Map(MODULES.map((definition) => [
      definition.id,
      definition.id === "animation" ? undefined : latestAssetForModule(assets, definition),
    ])),
    [assets],
  );
  const activeJobCount = jobs.filter(isJobActive).length;

  useEffect(() => {
    if (!active) return;
    let cancelled = false;
    const loadLatestAnimationGif = async () => {
      try {
        const items = await ListGalleryImages();
        const latest = [...items]
        .filter((item) => item.name.toLowerCase().endsWith(".gif"))
          .sort((a, b) => b.modTime - a.modTime)[0];
        if (!latest) return;
        const source = await LoadImageFull(latest.path);
        if (!cancelled && source) setAnimationGif(source);
      } catch {
        // The packaged animation preview remains available without the Wails bridge.
      }
    };
    void loadLatestAnimationGif();
    return () => { cancelled = true; };
  }, [active]);

  return (
    <main className="home-studio" aria-labelledby="tool-home-title">
      <header className="home-studio-header">
        <div>
          <h1 id="tool-home-title">{t("tool_home_title")}</h1>
          <p>{t("tool_home_desc")}</p>
        </div>
        <div className="home-studio-summary" aria-label={t("tool_home_label")}>
          {onOpenAssets ? (
            <button type="button" onClick={onOpenAssets}>{t("tool_home_assets_count", { count: assets.length })}</button>
          ) : (
            <span>{t("tool_home_assets_count", { count: assets.length })}</span>
          )}
          {onOpenTasks ? (
            <button type="button" onClick={onOpenTasks}>{t("tool_home_active_jobs", { count: activeJobCount })}</button>
          ) : (
            <span>{t("tool_home_active_jobs", { count: activeJobCount })}</span>
          )}
        </div>
      </header>

      <div className="home-module-list">
        {MODULES.map((definition, index) => {
          const Icon = definition.icon;
          const moduleAssets = assets.filter((asset) => assetModule(asset) === definition.id).length;
          const moduleJobs = jobs.filter((job) => isJobActive(job) && jobBelongsToModule(job, definition.id)).length;
          const reverse = index % 2 === 1;

          return (
            <article
              key={definition.id}
              className={`home-module-band home-module-${definition.id}${reverse ? " home-module-band-reverse" : ""}`}
            >
              <div className="home-module-copy">
                <span className="home-module-icon"><Icon size={18} aria-hidden="true" /></span>
                <div className="home-module-copy-body">
                  <span className="home-module-eyebrow">{t(definition.eyebrowKey)}</span>
                  <h2>{t(definition.titleKey)}</h2>
                  <p>{t(definition.descriptionKey)}</p>
                  <div className="home-module-meta">
                    <span>{t("tool_home_assets_count", { count: moduleAssets })}</span>
                    {moduleJobs > 0 && <span className="home-module-running">{t("tool_home_active_jobs", { count: moduleJobs })}</span>}
                  </div>
                </div>
              </div>
              <ModuleMedia
                definition={definition}
                asset={latestAssets.get(definition.id)}
                sourceOverride={definition.id === "animation" ? animationGif : undefined}
                label={t(definition.mediaLabelKey)}
                openLabel={`${t("tool_home_open")} ${t(definition.titleKey)}`}
                onOpen={() => onNavigate(definition.id)}
              />
            </article>
          );
        })}
      </div>
    </main>
  );
}
