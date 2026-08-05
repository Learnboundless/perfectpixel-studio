import { Download, Film, FolderPlus, Image as ImageIcon, Search, Send, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useI18n } from "../../i18n";
import { AppModule, CharacterLibraryItem, ToolAsset, ToolAssetKind, ToolchainModule } from "../../types";
import { Button } from "../ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "../ui/dialog";
import { Input } from "../ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../ui/select";
import AssetThumb from "./AssetThumb";
import { assetMediaKind, assetModule, dispatchAssetHandoff, toolchainApi } from "./api";
import { useToolchain } from "./ToolchainContext";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  characters: CharacterLibraryItem[];
  activeModule: AppModule;
  focusAssetId?: string;
  onNavigate: (module: ToolchainModule) => void;
}

function AssetDetails({
  asset,
  characters,
  onNavigate,
}: {
  asset: ToolAsset;
  characters: CharacterLibraryItem[];
  onNavigate: (module: ToolchainModule) => void;
}) {
  const { t, lang } = useI18n();
  const { deleteAsset, notify } = useToolchain();
  const [target, setTarget] = useState<ToolchainModule>(assetMediaKind(asset) === "video" ? "animation" : "image");
  const [characterId, setCharacterId] = useState(characters[0]?.id ?? "");
  const [actionName, setActionName] = useState("");
  const [frames, setFrames] = useState(8);
  const [fps, setFps] = useState(12);
  const [loop, setLoop] = useState(true);
  const [working, setWorking] = useState(false);
  const kind = assetMediaKind(asset);

  const exportAsset = async () => {
    setWorking(true);
    try {
      const path = await toolchainApi.exportAsset(asset.id);
      notify("success", t("tool_exported", { path }));
    } catch (error) {
      notify("error", String(error));
    } finally {
      setWorking(false);
    }
  };

  const remove = async () => {
    if (!window.confirm(t("tool_delete_asset_confirm", { name: asset.name }))) return;
    setWorking(true);
    try {
      await deleteAsset(asset.id);
      notify("success", t("tool_asset_deleted"));
    } catch (error) {
      notify("error", String(error));
    } finally {
      setWorking(false);
    }
  };

  const send = async () => {
    setWorking(true);
    try {
      const result = await toolchainApi.sendAssetToModule({
        assetId: asset.id,
        targetModule: target,
        characterId: target === "animation" ? characterId : undefined,
        actionName: target === "animation" ? actionName.trim() : undefined,
        frames: target === "animation" ? frames : undefined,
        fps: target === "animation" ? fps : undefined,
        loop: target === "animation" ? loop : undefined,
      });
      dispatchAssetHandoff(target, asset.id, result);
      notify("success", t("tool_asset_sent", { module: t(`tool_nav_${target}`) }));
      onNavigate(target);
    } catch (error) {
      notify("error", String(error));
    } finally {
      setWorking(false);
    }
  };

  return (
    <aside className="asset-details">
      <AssetThumb asset={asset} />
      <div className="asset-details-copy">
        <h3>{asset.name}</h3>
        <p>{[kind, asset.width && asset.height ? `${asset.width}×${asset.height}` : "", asset.provider, asset.model].filter(Boolean).join(" · ")}</p>
        <time>{new Date(asset.createdAt).toLocaleString(lang)}</time>
      </div>
      {asset.prompt && <p className="asset-prompt">{asset.prompt}</p>}
      {!!asset.tags?.length && <div className="asset-tags">{asset.tags.map((tag) => <span key={tag}>{tag}</span>)}</div>}

      <div className="asset-send-form">
        <label>{t("tool_send_to")}</label>
        <Select value={target} onValueChange={(value) => setTarget(value as ToolchainModule)}>
          <SelectTrigger><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="animation">{t("tool_nav_animation")}</SelectItem>
            <SelectItem value="image">{t("tool_nav_image")}</SelectItem>
            <SelectItem value="video">{t("tool_nav_video")}</SelectItem>
            <SelectItem value="ui">{t("tool_nav_ui")}</SelectItem>
          </SelectContent>
        </Select>
        {target === "animation" && kind === "video" && (
          <div className="asset-animation-options">
            <label>{t("tool_character")}</label>
            <Select value={characterId} onValueChange={setCharacterId}>
              <SelectTrigger><SelectValue placeholder={t("tool_choose_character")} /></SelectTrigger>
              <SelectContent>{characters.map((item) => <SelectItem key={item.id} value={item.id}>{item.name || t("tool_unnamed_character")}</SelectItem>)}</SelectContent>
            </Select>
            <label>{t("tool_action_name")}</label>
            <Input value={actionName} onChange={(e) => setActionName(e.target.value)} placeholder={t("tool_action_name_ph")} />
            <div className="tool-inline-fields">
              <label>{t("tool_frames")}<Input type="number" min={2} max={64} value={frames} onChange={(e) => setFrames(Number(e.target.value))} /></label>
              <label>{t("tool_fps")}<Input type="number" min={1} max={60} value={fps} onChange={(e) => setFps(Number(e.target.value))} /></label>
              <label className="tool-checkbox"><input type="checkbox" checked={loop} onChange={(e) => setLoop(e.target.checked)} />{t("tool_loop")}</label>
            </div>
          </div>
        )}
        <Button disabled={working || (target === "animation" && kind === "video" && (!characterId || !actionName.trim()))} onClick={send}>
          <Send size={13} /> {t("tool_send")}
        </Button>
      </div>
      <div className="asset-detail-actions">
        <Button variant="outline" size="sm" disabled={working} onClick={exportAsset}><Download size={13} />{t("export")}</Button>
        <Button variant="destructive-ghost" size="sm" disabled={working} onClick={remove}><Trash2 size={13} />{t("tool_delete")}</Button>
      </div>
    </aside>
  );
}

export default function AssetLibrary({ open, onOpenChange, characters, activeModule, focusAssetId, onNavigate }: Props) {
  const { t } = useI18n();
  const { assets, importAssets, notify } = useToolchain();
  const [query, setQuery] = useState("");
  const [kind, setKind] = useState<"all" | ToolAssetKind>("all");
  const [selectedId, setSelectedId] = useState("");
  const [importing, setImporting] = useState(false);
  const filtered = useMemo(() => assets.filter((asset) => {
    const matchesKind = kind === "all" || assetMediaKind(asset) === kind;
    const text = `${asset.name} ${asset.prompt ?? ""} ${(asset.tags ?? []).join(" ")} ${assetModule(asset)}`.toLowerCase();
    return matchesKind && text.includes(query.trim().toLowerCase());
  }), [assets, kind, query]);
  const selected = assets.find((asset) => asset.id === selectedId) ?? filtered[0];

  useEffect(() => {
    if (open && focusAssetId) setSelectedId(focusAssetId);
  }, [open, focusAssetId]);

  const handleImport = async () => {
    setImporting(true);
    try {
      const sourceModule: ToolchainModule = activeModule === "home" ? "image" : activeModule;
      const imported = await importAssets(sourceModule, kind === "all" ? undefined : kind);
      if (imported[0]) setSelectedId(imported[0].id);
    } catch (error) {
      notify("error", String(error));
    } finally {
      setImporting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="tool-dialog asset-library-dialog" aria-describedby="asset-library-description">
        <div className="tool-dialog-head">
          <div><DialogTitle>{t("tool_assets_title")}</DialogTitle><DialogDescription id="asset-library-description">{t("tool_assets_desc")}</DialogDescription></div>
          <Button variant="outline" size="sm" disabled={importing} onClick={handleImport}><FolderPlus size={13} />{importing ? t("tool_importing") : t("tool_import")}</Button>
        </div>
        <div className="tool-filterbar">
          <div className="tool-search"><Search size={13} /><Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t("tool_search_assets")} /></div>
          <div className="asset-kind-filter" role="group" aria-label={t("tool_filter_media")}>
            {(["all", "image", "video"] as const).map((item) => (
              <button key={item} type="button" className={kind === item ? "active" : ""} onClick={() => setKind(item)}>
                {item === "image" && <ImageIcon size={12} />}{item === "video" && <Film size={12} />}{t(`tool_media_${item}`)}
              </button>
            ))}
          </div>
        </div>
        <div className="asset-library-body">
          <div className="asset-grid" role="list">
            {filtered.map((asset) => (
              <button key={asset.id} type="button" role="listitem" className={`asset-card${selected?.id === asset.id ? " active" : ""}`} onClick={() => setSelectedId(asset.id)}>
                <AssetThumb asset={asset} />
                <span title={asset.name}>{asset.name}</span>
                <small>{t(`tool_media_${assetMediaKind(asset)}`)} · {t(`tool_nav_${assetModule(asset)}`)}</small>
              </button>
            ))}
            {!filtered.length && <div className="tool-empty"><ImageIcon size={22} /><strong>{t("tool_assets_empty")}</strong><span>{t("tool_assets_empty_desc")}</span></div>}
          </div>
          {selected && <AssetDetails key={selected.id} asset={selected} characters={characters} onNavigate={onNavigate} />}
        </div>
      </DialogContent>
    </Dialog>
  );
}
