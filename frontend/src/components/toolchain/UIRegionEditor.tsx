import { Minus, Plus, ScanLine, Trash2, ZoomIn, ZoomOut } from "lucide-react";
import { PointerEvent, useMemo, useRef, useState } from "react";
import { useI18n } from "../../i18n";
import { ToolAsset, UIRegion, uid } from "../../types";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Textarea } from "../ui/textarea";
import { assetURL } from "./api";

interface Props {
  asset: ToolAsset;
  regions: UIRegion[];
  onChange: (regions: UIRegion[]) => void;
  inferred?: boolean;
}

type Gesture = {
  type: "move" | "resize" | "draw";
  id: string;
  startX: number;
  startY: number;
  bounds: UIRegion["bounds"];
  region?: UIRegion;
};

export default function UIRegionEditor({ asset, regions, onChange, inferred = false }: Props) {
  const { t } = useI18n();
  const [selectedId, setSelectedId] = useState(regions[0]?.id ?? "");
  const [zoom, setZoom] = useState(1);
  const [drawing, setDrawing] = useState(false);
  const canvasRef = useRef<HTMLDivElement>(null);
  const gesture = useRef<Gesture | null>(null);
  const sourceWidth = Math.max(1, asset.width ?? 1024);
  const sourceHeight = Math.max(1, asset.height ?? 768);
  const selected = regions.find((region) => region.id === selectedId);
  const sorted = useMemo(() => [...regions].sort((a, b) => b.zIndex - a.zIndex), [regions]);
  const regionDepth = (region: UIRegion) => {
    let depth = 0;
    let parentId = region.parentId;
    const seen = new Set<string>();
    while (parentId && depth < 8 && !seen.has(parentId)) {
      seen.add(parentId);
      depth += 1;
      parentId = regions.find((item) => item.id === parentId)?.parentId;
    }
    return depth;
  };

  const point = (event: PointerEvent) => {
    const rect = canvasRef.current!.getBoundingClientRect();
    return {
      x: Math.max(0, Math.min(sourceWidth, ((event.clientX - rect.left) / rect.width) * sourceWidth)),
      y: Math.max(0, Math.min(sourceHeight, ((event.clientY - rect.top) / rect.height) * sourceHeight)),
    };
  };

  const updateRegion = (id: string, patch: Partial<UIRegion>) => {
    onChange(regions.map((region) => region.id === id ? { ...region, ...patch } : region));
  };

  const startGesture = (event: PointerEvent, type: "move" | "resize", region: UIRegion) => {
    event.preventDefault();
    event.stopPropagation();
    setSelectedId(region.id);
    const p = point(event);
    gesture.current = { type, id: region.id, startX: p.x, startY: p.y, bounds: { ...region.bounds } };
    canvasRef.current?.setPointerCapture(event.pointerId);
  };

  const startDraw = (event: PointerEvent<HTMLDivElement>) => {
    if (!drawing) return;
    event.preventDefault();
    const p = point(event);
    const id = uid("ui-region");
    const region: UIRegion = {
      id,
      name: `${t("tool_region")} ${regions.length + 1}`,
      type: "component",
      bounds: { x: Math.round(p.x), y: Math.round(p.y), width: 1, height: 1 },
      zIndex: regions.length,
      confidence: 1,
      exportEnabled: true,
    };
    onChange([...regions, region]);
    setSelectedId(id);
    gesture.current = { type: "draw", id, startX: p.x, startY: p.y, bounds: { ...region.bounds }, region };
    event.currentTarget.setPointerCapture(event.pointerId);
  };

  const moveGesture = (event: PointerEvent<HTMLDivElement>) => {
    const active = gesture.current;
    if (!active) return;
    const p = point(event);
    const dx = p.x - active.startX;
    const dy = p.y - active.startY;
    let bounds = { ...active.bounds };
    if (active.type === "move") {
      bounds.x = Math.round(Math.max(0, Math.min(sourceWidth - bounds.width, active.bounds.x + dx)));
      bounds.y = Math.round(Math.max(0, Math.min(sourceHeight - bounds.height, active.bounds.y + dy)));
    } else if (active.type === "resize") {
      bounds.width = Math.round(Math.max(2, Math.min(sourceWidth - bounds.x, active.bounds.width + dx)));
      bounds.height = Math.round(Math.max(2, Math.min(sourceHeight - bounds.y, active.bounds.height + dy)));
    } else {
      bounds.x = Math.round(Math.min(active.startX, p.x));
      bounds.y = Math.round(Math.min(active.startY, p.y));
      bounds.width = Math.round(Math.max(2, Math.abs(p.x - active.startX)));
      bounds.height = Math.round(Math.max(2, Math.abs(p.y - active.startY)));
    }
    const exists = regions.some((region) => region.id === active.id);
    onChange(exists
      ? regions.map((region) => region.id === active.id ? { ...region, bounds } : region)
      : active.region ? [...regions, { ...active.region, bounds }] : regions);
  };

  const endGesture = (event: PointerEvent<HTMLDivElement>) => {
    if (!gesture.current) return;
    gesture.current = null;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
  };

  const nudge = (event: React.KeyboardEvent, region: UIRegion) => {
    const amount = event.shiftKey ? 10 : 1;
    const delta = { x: 0, y: 0 };
    if (event.key === "ArrowLeft") delta.x = -amount;
    else if (event.key === "ArrowRight") delta.x = amount;
    else if (event.key === "ArrowUp") delta.y = -amount;
    else if (event.key === "ArrowDown") delta.y = amount;
    else return;
    event.preventDefault();
    updateRegion(region.id, { bounds: { ...region.bounds, x: Math.max(0, Math.min(sourceWidth - region.bounds.width, region.bounds.x + delta.x)), y: Math.max(0, Math.min(sourceHeight - region.bounds.height, region.bounds.y + delta.y)) } });
  };

  const removeSelected = () => {
    if (!selected) return;
    onChange(regions.filter((region) => region.id !== selected.id).map((region) => region.parentId === selected.id ? { ...region, parentId: undefined } : region));
    setSelectedId("");
  };

  return (
    <div className="ui-region-editor">
      <div className="ui-editor-toolbar">
        <Button type="button" variant={drawing ? "secondary" : "ghost"} size="sm" onClick={() => setDrawing((value) => !value)}><ScanLine size={13} />{t("tool_draw_region")}</Button>
        <div className="ui-editor-zoom"><Button type="button" variant="ghost" size="icon-sm" onClick={() => setZoom((value) => Math.max(.5, value - .1))}><ZoomOut size={12} /></Button><span>{Math.round(zoom * 100)}%</span><Button type="button" variant="ghost" size="icon-sm" onClick={() => setZoom((value) => Math.min(2, value + .1))}><ZoomIn size={12} /></Button></div>
        <Button type="button" variant="destructive-ghost" size="icon-sm" disabled={!selected} onClick={removeSelected} title={t("tool_delete_region")}><Trash2 size={12} /></Button>
      </div>
      <div className="ui-editor-body">
        <div className="ui-canvas-scroll">
          <div
            ref={canvasRef}
            className={`ui-source-canvas ${drawing ? "drawing" : ""}`}
            style={{ width: `min(100%, ${sourceWidth * zoom}px)`, aspectRatio: `${sourceWidth} / ${sourceHeight}` }}
            onPointerDown={startDraw}
            onPointerMove={moveGesture}
            onPointerUp={endGesture}
            onPointerCancel={endGesture}
          >
            <img src={assetURL(asset)} alt={asset.name} draggable={false} />
            {regions.map((region) => (
              <button
                key={region.id}
                type="button"
                className={`ui-region-box${region.id === selectedId ? " selected" : ""}${!region.exportEnabled ? " disabled" : ""}`}
                style={{
                  left: `${(region.bounds.x / sourceWidth) * 100}%`,
                  top: `${(region.bounds.y / sourceHeight) * 100}%`,
                  width: `${(region.bounds.width / sourceWidth) * 100}%`,
                  height: `${(region.bounds.height / sourceHeight) * 100}%`,
                }}
                aria-label={region.name}
                onPointerDown={(event) => startGesture(event, "move", region)}
                onKeyDown={(event) => nudge(event, region)}
              >
                <span>{region.name}</span>
                <i onPointerDown={(event) => startGesture(event, "resize", region)} aria-hidden="true" />
              </button>
            ))}
          </div>
        </div>
        <aside className="ui-layer-panel">
          <div className="ui-layer-head"><strong>{t("tool_layers")}</strong><label><input type="checkbox" checked={regions.length > 0 && regions.every((region) => region.exportEnabled)} onChange={(event) => onChange(regions.map((region) => ({ ...region, exportEnabled: event.target.checked })))} />{t("tool_select_all")}</label><span>{regions.length}</span></div>
          <div className="ui-layer-list">
            {sorted.map((region) => (
              <button key={region.id} type="button" className={region.id === selectedId ? "active" : ""} style={{ paddingLeft: 5 + regionDepth(region) * 12 }} onClick={() => setSelectedId(region.id)}>
                <input type="checkbox" checked={region.exportEnabled} onClick={(event) => event.stopPropagation()} onChange={(event) => updateRegion(region.id, { exportEnabled: event.target.checked })} aria-label={t("tool_export_region")} />
                <span title={region.name}>{region.name}</span>
                {inferred && <small>{Math.round(region.confidence * 100)}%</small>}
              </button>
            ))}
          </div>
          {selected && (
            <div className="ui-region-properties">
              <label>{t("tool_region_name")}<Input value={selected.name} onChange={(event) => updateRegion(selected.id, { name: event.target.value })} /></label>
              <label>{t("tool_region_type")}<Input value={selected.type} onChange={(event) => updateRegion(selected.id, { type: event.target.value })} /></label>
              <label>{t("tool_region_parent")}
                <select value={selected.parentId ?? ""} onChange={(event) => updateRegion(selected.id, { parentId: event.target.value || undefined })}>
                  <option value="">{t("tool_no_parent")}</option>
                  {regions.filter((region) => region.id !== selected.id).map((region) => <option key={region.id} value={region.id}>{region.name}</option>)}
                </select>
              </label>
              <div className="ui-bounds-grid">
                {(["x", "y", "width", "height"] as const).map((key) => <label key={key}>{key.toUpperCase()}<Input type="number" value={selected.bounds[key]} onChange={(event) => updateRegion(selected.id, { bounds: { ...selected.bounds, [key]: Number(event.target.value) } })} /></label>)}
              </div>
              <label>{t("tool_z_index")}<Input type="number" value={selected.zIndex} onChange={(event) => updateRegion(selected.id, { zIndex: Number(event.target.value) })} /></label>
              <label>{t("tool_region_text")}<Input value={selected.text ?? ""} onChange={(event) => updateRegion(selected.id, { text: event.target.value })} /></label>
              <label>{t("tool_region_style")}<Textarea key={`${selected.id}-${JSON.stringify(selected.style ?? {})}`} rows={4} defaultValue={JSON.stringify(selected.style ?? {}, null, 2)} onBlur={(event) => {
                try { updateRegion(selected.id, { style: JSON.parse(event.target.value) }); } catch { /* Keep the last valid style object. */ }
              }} /></label>
            </div>
          )}
        </aside>
      </div>
    </div>
  );
}
