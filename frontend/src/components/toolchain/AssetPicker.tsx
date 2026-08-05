import { ImagePlus, Plus, Upload, X } from "lucide-react";
import { useMemo, useState } from "react";
import { useI18n } from "../../i18n";
import { ToolAsset, ToolAssetKind, ToolchainModule } from "../../types";
import { Button } from "../ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../ui/select";
import AssetThumb from "./AssetThumb";
import { assetMediaKind } from "./api";
import { useToolchain } from "./ToolchainContext";

interface Props {
  kind: ToolAssetKind;
  value: string[];
  onChange: (ids: string[]) => void;
  max?: number;
  label: string;
  module?: ToolchainModule;
  preview?: boolean;
}

export default function AssetPicker({ kind, value, onChange, max = 1, label, module, preview }: Props) {
  const { t } = useI18n();
  const { assets, importAssets, notify } = useToolchain();
  const showPreview = preview ?? kind === "image";
  const [candidate, setCandidate] = useState("");
  const [importing, setImporting] = useState(false);
  const [recentAssets, setRecentAssets] = useState<ToolAsset[]>([]);
  const availableAssets = useMemo(() => {
    const byId = new Map(assets.map((asset) => [asset.id, asset]));
    recentAssets.forEach((asset) => byId.set(asset.id, asset));
    return [...byId.values()];
  }, [assets, recentAssets]);
  const choices = useMemo(
    () => availableAssets.filter((asset) => assetMediaKind(asset) === kind && !value.includes(asset.id)),
    [availableAssets, kind, value],
  );
  const selected = value.map((id) => availableAssets.find((asset) => asset.id === id)).filter(Boolean) as ToolAsset[];

  const selectAsset = (id: string) => {
    if (!id) return;
    if (!showPreview) {
      setCandidate(id);
      return;
    }
    onChange(max === 1 ? [id] : [...value.filter((assetId) => assetId !== id), id].slice(0, max));
    setCandidate("");
  };

  const addCandidate = () => {
    if (!candidate || value.length >= max) return;
    onChange([...value, candidate].slice(0, max));
    setCandidate("");
  };

  const removeAsset = (id: string) => onChange(value.filter((assetId) => assetId !== id));

  const assetDimensions = (asset: ToolAsset) => asset.width && asset.height ? `${asset.width} x ${asset.height}` : "";

  const handleImport = async () => {
    setImporting(true);
    try {
      const imported = await importAssets(module ?? (kind === "video" ? "video" : "image"), kind);
      const compatible = imported.filter((asset) => assetMediaKind(asset) === kind);
      if (compatible.length) {
        setRecentAssets((current) => {
          const importedIds = new Set(compatible.map((asset) => asset.id));
          return [...current.filter((asset) => !importedIds.has(asset.id)), ...compatible];
        });
        onChange(max === 1
          ? [compatible[0].id]
          : [...value, ...compatible.map((asset) => asset.id)].filter((id, index, ids) => ids.indexOf(id) === index).slice(0, max));
      }
    } catch (error) {
      notify("error", String(error));
    } finally {
      setImporting(false);
    }
  };

  return (
    <div className={`asset-picker${showPreview ? " asset-picker--preview" : ""}`}>
      <div className="asset-picker-head">
        <span>{label}</span>
        <small>{value.length}/{max}</small>
      </div>
      {showPreview ? (
        <div className={`asset-picker-preview-grid${max === 1 ? " single" : ""}`}>
          {selected.map((asset) => (
            <div key={asset.id} className="asset-picker-preview-item" aria-label={asset.name}>
              {max === 1 ? (
                <button
                  type="button"
                  className="asset-picker-preview-media asset-picker-replace-surface checker"
                  onClick={handleImport}
                  disabled={importing}
                  aria-label={`${t("tool_replace_image")}: ${asset.name}`}
                >
                  <AssetThumb asset={asset} full />
                  <span className="asset-picker-replace-badge"><Upload size={12} />{importing ? t("tool_importing") : t("tool_replace_image")}</span>
                </button>
              ) : (
                <div className="asset-picker-preview-media checker">
                  <AssetThumb asset={asset} full />
                </div>
              )}
              <div className="asset-picker-preview-footer">
                <span className="asset-picker-preview-name" title={asset.name}>
                  <strong>{asset.name}</strong>
                  {assetDimensions(asset) && <small>{assetDimensions(asset)}</small>}
                </span>
                <button
                  type="button"
                  className="asset-picker-preview-remove"
                  onClick={() => removeAsset(asset.id)}
                  title={t("tool_remove_asset")}
                  aria-label={`${t("tool_remove_asset")}: ${asset.name}`}
                >
                  <X size={13} />
                </button>
              </div>
            </div>
          ))}
          {value.length < max && (
            <button type="button" className="asset-picker-upload checker" disabled={importing} onClick={handleImport}>
              <ImagePlus size={26} aria-hidden="true" />
              <strong>{importing ? t("tool_importing") : t("tool_upload_image")}</strong>
              <span>{t("tool_upload_image_hint")}</span>
            </button>
          )}
        </div>
      ) : selected.length > 0 ? (
          <div className="asset-picker-selected">
            {selected.map((asset) => (
              <div key={asset.id} className="asset-picker-chip">
                <AssetThumb asset={asset} compact />
                <span title={asset.name}>{asset.name}</span>
                <button type="button" onClick={() => removeAsset(asset.id)} title={t("tool_remove_asset")}>
                  <X size={11} />
                </button>
              </div>
            ))}
          </div>
      ) : null}
      {(value.length < max || (showPreview && max === 1)) && (
        <div className={`asset-picker-actions${showPreview ? " immediate" : ""}`}>
          <Select value={candidate} onValueChange={selectAsset}>
            <SelectTrigger aria-label={label}>
              <SelectValue placeholder={choices.length ? t("tool_choose_asset") : t("tool_no_assets")} />
            </SelectTrigger>
            <SelectContent>
              {choices.map((asset) => (
                <SelectItem key={asset.id} value={asset.id} className="asset-picker-select-item">
                  <span className="asset-picker-option">
                    <AssetThumb asset={asset} compact />
                    <span className="asset-picker-option-copy">
                      <strong>{asset.name}</strong>
                      {assetDimensions(asset) && <small>{assetDimensions(asset)}</small>}
                    </span>
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {!showPreview && (
            <Button type="button" variant="outline" size="icon" disabled={!candidate} onClick={addCandidate} title={t("tool_add_asset")}>
              <Plus size={13} />
            </Button>
          )}
          {!showPreview && <Button type="button" variant="ghost" size="sm" disabled={importing} onClick={handleImport}>{importing ? t("tool_importing") : t("tool_import")}</Button>}
        </div>
      )}
    </div>
  );
}
