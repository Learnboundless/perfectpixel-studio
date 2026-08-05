import { FileArchive, FileJson, FileText, Image as ImageIcon, Music, Video } from "lucide-react";
import { useEffect, useState } from "react";
import { ToolAsset } from "../../types";
import { assetMediaKind, assetURL } from "./api";

const ICONS = {
  image: ImageIcon,
  video: Video,
  audio: Music,
  text: FileText,
  json: FileJson,
  archive: FileArchive,
};

export default function AssetThumb({ asset, compact = false, full = false }: { asset: ToolAsset; compact?: boolean; full?: boolean }) {
  const [failed, setFailed] = useState(false);
  const kind = assetMediaKind(asset);
  const Icon = ICONS[kind];
  const thumbnailSource = assetURL(asset, true);
  const contentSource = assetURL(asset);
  const preferredSource = full ? contentSource : thumbnailSource;
  const [source, setSource] = useState(preferredSource);

  useEffect(() => {
    setSource(preferredSource);
    setFailed(false);
  }, [asset.id, preferredSource]);

  const handleMediaError = () => {
    if (source !== contentSource && contentSource) {
      setSource(contentSource);
      return;
    }
    setFailed(true);
  };

  return (
    <div className={`asset-thumb ${compact ? "compact" : ""} ${kind}`}>
      {!failed && kind === "image" && source ? (
        <img src={source} alt="" loading="lazy" onError={handleMediaError} />
      ) : !failed && kind === "video" && source ? (
        <video src={source} muted preload="metadata" onError={handleMediaError} />
      ) : (
        <Icon size={compact ? 18 : 28} aria-hidden="true" />
      )}
    </div>
  );
}
