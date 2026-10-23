import { useEffect, useMemo, useState } from "react";
import { ArchiveRestore, GitCompareArrows, History, Inbox, Pause, Play, RotateCcw, Trash2 } from "lucide-react";
import { ClaimLegacyAnimationRecovery, GetAnimationVersion, IgnoreLegacyAnimationRecovery, ListAnimationVersions, ListLegacyAnimationRecovery, PurgeAnimationVersion, RestoreAnimationVersion, SetCurrentAnimationVersion, SoftDeleteAnimationVersion, UpdateAnimationVersionComposition } from "../../wailsjs/go/main/App";
import { AnimationVersionSummary, StateDef } from "../types";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "./ui/dialog";

interface IProps {
  open: boolean;
  characterId: string;
  state: StateDef;
  cellSize: number;
  onClose: () => void;
  onApply: (version: any | null, count: number) => void;
}

interface LegacyRecoveryItem {
  id: string;
  artifactDir: string;
  stateHint?: string;
  frameCount: number;
  thumbnail?: string;
  reason: string;
  sizeBytes: number;
  discoveredAt: string;
}

const formatBytes = (bytes: number) => bytes >= 1024 ** 3
  ? `${(bytes / 1024 ** 3).toFixed(2)} GB`
  : bytes >= 1024 ** 2
    ? `${(bytes / 1024 ** 2).toFixed(1)} MB`
    : `${Math.max(1, Math.round(bytes / 1024))} KB`;

export default function VersionManager({ open, characterId, state, cellSize, onClose, onApply }: IProps) {
  const [versions, setVersions] = useState<AnimationVersionSummary[]>([]);
  const [leftId, setLeftId] = useState("");
  const [rightId, setRightId] = useState("");
  const [left, setLeft] = useState<any>(null);
  const [right, setRight] = useState<any>(null);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState(0);
  const [playing, setPlaying] = useState(true);
  const [recoveryItems, setRecoveryItems] = useState<LegacyRecoveryItem[]>([]);
  const activeVersions = useMemo(() => versions.filter((version) => !version.deletedAt), [versions]);

  const refresh = async () => {
    const [list, recovery] = await Promise.all([
      ListAnimationVersions(characterId, state.id, true) as unknown as Promise<AnimationVersionSummary[]>,
      ListLegacyAnimationRecovery(false) as unknown as Promise<LegacyRecoveryItem[]>,
    ]);
    setRecoveryItems(recovery ?? []);
    const next = list ?? [];
    const active = next.filter((version) => !version.deletedAt);
    setVersions(next);
    const current = active.find((version) => version.current) ?? active[0];
    const previous = active.find((version) => version.id !== current?.id);
    setLeftId((value) => active.some((version) => version.id === value) ? value : current?.id || "");
    setRightId((value) => active.some((version) => version.id === value) ? value : previous?.id || current?.id || "");
    if (active.length === 0) {
      setLeft(null);
      setRight(null);
    }
    return next;
  };

  useEffect(() => { if (open) void refresh(); }, [open, characterId, state.id]);
  useEffect(() => { if (leftId) void GetAnimationVersion(leftId).then(setLeft); }, [leftId]);
  useEffect(() => { if (rightId) void GetAnimationVersion(rightId).then(setRight); }, [rightId]);
  useEffect(() => {
    if (!open || !playing) return;
    let frame = 0;
    let previous = performance.now();
    const tick = (now: number) => {
      const elapsed = Math.max(0, now - previous);
      previous = now;
      const maxFrames = Math.max(1, Math.max(framesOf(left).length, framesOf(right).length));
      setProgress((value) => (value + elapsed / 1000 * Math.max(1, state.fps)) % maxFrames);
      frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [open, playing, state.fps, leftId, rightId, left?.id, right?.id]);

  const setCurrent = async (id: string) => {
    setBusy(true);
    try {
      const version = await SetCurrentAnimationVersion(characterId, state.id, id);
      const next = await refresh();
      onApply(version, next.filter((item) => !item.deletedAt).length);
    } finally { setBusy(false); }
  };

  const softDelete = async (id: string) => {
    setBusy(true);
    try {
      await SoftDeleteAnimationVersion(id);
      const next = await ListAnimationVersions(characterId, state.id, true) as unknown as AnimationVersionSummary[];
      setVersions(next ?? []);
      const current = next?.find((version) => version.current);
      const count = next.filter((version) => !version.deletedAt).length;
      if (current) {
        const currentVersion = await GetAnimationVersion(current.id);
        setLeftId(current.id);
        const previous = next.find((version) => !version.deletedAt && version.id !== current.id);
        setRightId(previous?.id ?? current.id);
        onApply(currentVersion, count);
      } else {
        setLeftId("");
        setRightId("");
        setLeft(null);
        setRight(null);
        onApply(null, 0);
      }
    } finally { setBusy(false); }
  };

  const restore = async (id: string) => { setBusy(true); try { await RestoreAnimationVersion(id); await refresh(); } finally { setBusy(false); } };
  const purge = async (id: string) => { setBusy(true); try { await PurgeAnimationVersion(id); await refresh(); } finally { setBusy(false); } };
  const claimRecovery = async (id: string) => {
    setBusy(true);
    try {
      const version = await ClaimLegacyAnimationRecovery(id, characterId, state.id, state.name);
      const next = await refresh();
      onApply(version, next.filter((item) => !item.deletedAt).length);
    } finally { setBusy(false); }
  };
  const ignoreRecovery = async (id: string) => {
    setBusy(true);
    try {
      await IgnoreLegacyAnimationRecovery(id);
      await refresh();
    } finally { setBusy(false); }
  };
  const framesOf = (version: any) => (version?.workingComposition ?? []).filter((frame: any) => frame.selected).map((frame: any) => frame.png);
  const maxComparedFrames = Math.max(1, Math.max(framesOf(left).length, framesOf(right).length));
  const normalizedProgress = (progress % maxComparedFrames) / maxComparedFrames;
  const frameIndexOf = (version: any) => {
    const frames = framesOf(version);
    if (frames.length === 0) return 0;
    return Math.min(frames.length - 1, Math.floor(normalizedProgress * frames.length));
  };
  const frameAt = (version: any) => {
    const frames = framesOf(version);
    if (frames.length === 0) return "";
    return frames[frameIndexOf(version)];
  };
  const parameterRows = useMemo(() => compareParameters(left, right), [left, right]);
  const restoreBaseline = async () => {
    if (!left?.current || !Array.isArray(left.baselineFrames)) return;
    setBusy(true);
    try {
      const updated = await UpdateAnimationVersionComposition(left.id, left.baselineFrames);
      setLeft(updated);
      onApply(updated, activeVersions.length);
    } finally { setBusy(false); }
  };

  return (
    <Dialog open={open} onOpenChange={(value) => !value && onClose()}>
      <DialogContent className="!h-[82vh] !w-[1060px] !max-w-[calc(100vw-32px)] !overflow-hidden !p-0">
        <header className="settings-header"><div><DialogTitle><History size={15} /> 动作版本</DialogTitle><DialogDescription>当前 v{state.currentVersionNumber ?? "-"} · 共 {activeVersions.length} 版</DialogDescription></div></header>
        <div className="version-layout">
          <aside className="version-list">
            {versions.map((version) => (
              <div key={version.id} className={`version-row ${version.current ? "current" : ""} ${version.deletedAt ? "deleted" : ""}`}>
                {version.thumbnail && <img src={version.thumbnail} />}
                <div className="version-row-main"><strong>v{version.versionNumber} {version.current ? "· 当前" : ""}</strong><span>{version.source} · {version.backgroundRemovalMode || "-"}</span><span>{new Date(version.createdAt).toLocaleString()} · {formatBytes(version.sizeBytes)}</span></div>
                <div className="version-row-actions">
                  {!version.deletedAt ? <>
                    {!version.current && <Button size="sm" variant="outline" disabled={busy} onClick={() => setCurrent(version.id)}>设为当前</Button>}
                    <Button size="icon-sm" variant="destructive-ghost" disabled={busy} onClick={() => softDelete(version.id)}><Trash2 size={12} /></Button>
                  </> : <>
                    <Button size="icon-sm" variant="outline" disabled={busy} onClick={() => restore(version.id)}><ArchiveRestore size={12} /></Button>
                    <Button size="icon-sm" variant="destructive" disabled={busy} onClick={() => purge(version.id)}><Trash2 size={12} /></Button>
                  </>}
                </div>
              </div>
            ))}
            {recoveryItems.length > 0 && <section className="legacy-recovery-box">
              <div className="legacy-recovery-title"><Inbox size={13} /> 待认领旧结果 <span>{recoveryItems.length}</span></div>
              <p>这些结果无法唯一确定角色与动作。只在确认后认领到当前动作。</p>
              {recoveryItems.map((item) => <div key={item.id} className="legacy-recovery-row">
                {item.thumbnail && <img src={item.thumbnail} />}
                <div><strong>{item.stateHint || "未知动作"} · {item.frameCount} 帧</strong><span>{formatBytes(item.sizeBytes)} · {new Date(item.discoveredAt).toLocaleString()}</span><small title={item.reason}>{item.reason}</small></div>
                <div className="legacy-recovery-actions">
                  <Button size="sm" variant="outline" disabled={busy} onClick={() => claimRecovery(item.id)}>认领到此动作</Button>
                  <Button size="sm" variant="ghost" disabled={busy} onClick={() => ignoreRecovery(item.id)}>忽略</Button>
                </div>
              </div>)}
            </section>}
          </aside>
          <main className="version-compare">
            <div className="version-compare-head"><GitCompareArrows size={14} /> 双版本同步对比</div>
            {activeVersions.length > 0 ? <>
            <div className="version-selects">
              <select value={leftId} onChange={(event) => setLeftId(event.target.value)}>{activeVersions.map((version) => <option key={version.id} value={version.id}>v{version.versionNumber}{version.current ? "（当前）" : ""}</option>)}</select>
              <select value={rightId} onChange={(event) => setRightId(event.target.value)}>{activeVersions.map((version) => <option key={version.id} value={version.id}>v{version.versionNumber}{version.current ? "（当前）" : ""}</option>)}</select>
            </div>
            <div className="version-players">
              <section><div className="version-stage checker">{frameAt(left) && <img src={frameAt(left)} style={{ width: Math.min(320, cellSize), imageRendering: "pixelated" }} />}</div><small>第 {frameIndexOf(left) + 1}/{Math.max(1, framesOf(left).length)} 帧 · 分数 {Math.round((left?.overallScore ?? 0) * 100)} · {left?.source} · {left?.backgroundRemovalMode}</small></section>
              <section><div className="version-stage checker">{frameAt(right) && <img src={frameAt(right)} style={{ width: Math.min(320, cellSize), imageRendering: "pixelated" }} />}</div><small>第 {frameIndexOf(right) + 1}/{Math.max(1, framesOf(right).length)} 帧 · 分数 {Math.round((right?.overallScore ?? 0) * 100)} · {right?.source} · {right?.backgroundRemovalMode}</small></section>
            </div>
            <div className="version-scrubber">
              <Button size="icon-sm" variant="outline" onClick={() => setPlaying((value) => !value)} title={playing ? "暂停" : "播放"}>{playing ? <Pause size={12} /> : <Play size={12} />}</Button>
              <input aria-label="版本对比播放进度" type="range" min={0} max={1000} value={Math.round(normalizedProgress * 1000)} onChange={(event) => { setPlaying(false); setProgress(Number(event.target.value) / 1000 * maxComparedFrames); }} />
              <span>{Math.round(normalizedProgress * 100)}%</span>
            </div>
            <section className="version-parameter-diff">
              <h4>参数差异</h4>
              {parameterRows.length > 0 ? <div className="version-diff-table">
                <div className="head">参数</div><div className="head">左侧</div><div className="head">右侧</div>
                {parameterRows.map((row) => <div key={row.key} className="version-diff-row">
                  <span title={row.key}>{row.label}</span><code title={row.left}>{row.left || "—"}</code><code title={row.right}>{row.right || "—"}</code>
                </div>)}
              </div> : <p>两版的已记录参数一致。</p>}
            </section>
            <div className="version-compare-actions">
              {left?.id && left?.id !== state.currentVersionId && <Button variant="outline" disabled={busy} onClick={() => setCurrent(left.id)}><RotateCcw size={12} /> 将左侧设为当前</Button>}
              {left?.current && <Button variant="outline" disabled={busy} onClick={restoreBaseline}><RotateCcw size={12} /> 恢复本次生成原始帧</Button>}
            </div>
            </> : <div className="version-empty">此动作的所有版本都在回收站中。恢复任一版本，或关闭后重新生成。</div>}
          </main>
        </div>
      </DialogContent>
    </Dialog>
  );
}

const formatParameter = (value: unknown) => {
  if (value === null || value === undefined || value === "") return "";
  if (typeof value === "string") return value.length > 120 ? `${value.slice(0, 117)}…` : value;
  if (typeof value === "number") return Number.isInteger(value) ? String(value) : value.toFixed(4).replace(/0+$/, "").replace(/\.$/, "");
  if (typeof value === "boolean") return value ? "是" : "否";
  try {
    const encoded = JSON.stringify(value);
    return encoded.length > 120 ? `${encoded.slice(0, 117)}…` : encoded;
  } catch {
    return String(value);
  }
};

const versionParameters = (version: any): Record<string, unknown> => ({
  "状态": version?.status,
  "来源": version?.source,
  "帧数": version?.frameCount,
  "总分": version?.overallScore,
  "抠图模式": version?.backgroundRemovalMode,
  "生成管线": version?.trace?.pipeline,
  "提供方": version?.trace?.provider,
  "图片模型": version?.trace?.model,
  "视频模型": version?.trace?.videoModel,
  "抠图模型": version?.trace?.backgroundRemovalModel,
  "抠图耗时 ms": version?.trace?.backgroundRemovalMs,
  "抠图降级": version?.trace?.backgroundRemovalFallback,
  ...(version?.generationParameters ?? {}),
});

const compareParameters = (left: any, right: any) => {
  if (!left || !right) return [];
  const leftValues = versionParameters(left);
  const rightValues = versionParameters(right);
  return Array.from(new Set([...Object.keys(leftValues), ...Object.keys(rightValues)]))
    .map((key) => ({ key, label: key, left: formatParameter(leftValues[key]), right: formatParameter(rightValues[key]) }))
    .filter((row) => row.left !== row.right)
    .slice(0, 24);
};
