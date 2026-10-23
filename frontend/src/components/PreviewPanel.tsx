import { useEffect, useRef, useState } from "react";
import { AlertTriangle, Check, ChevronLeft, ChevronRight, Clapperboard, Clock3, Download, Eraser, ExternalLink, FileVideo, FlipHorizontal2, History, LayoutGrid, ListChecks, Maximize2, Package, RefreshCw, Replace, Save, Settings2, Wand2, X, ZoomIn, ZoomOut } from "lucide-react";
import { LoadAnimationVersionResource, LoadVideoDataURL, RevealInFinder } from "../../wailsjs/go/main/App";
import { AlignmentMode, BackgroundRemovalMode, CandidateFrame, FrameFitMode, FrameSamplerMode, DirectionInfo, FrameSequenceMetrics, StateDef, selectedFrames, uid } from "../types";
import { useI18n } from "../i18n";
import { composeStateLabel, directionName } from "../i18n/catalog";
import { isCopyrightRestrictionError, localizeBackendText } from "../i18n/backend";
import AnimPlayer from "./AnimPlayer";
import DirectionGrid from "./DirectionGrid";
import LocalImage, { loadImageReference } from "./LocalImage";
import { Button } from "./ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select";
import { Tabs, TabsList, TabsTrigger } from "./ui/tabs";
import { Textarea } from "./ui/textarea";

type ViewTab = "play" | "frames" | "source" | "atlas";

interface IProps {
  state: StateDef | null;
  allStates: StateDef[];
  directions: DirectionInfo[];
  cellSize: number;
  atlasColumns: number;
  busy: boolean;
  onUpdateState: (id: string, patch: Partial<StateDef>) => void;
  onSelect: (id: string) => void;
  onRegenerate: (id: string, feedback: string) => void;
  onReprocessVideo: (id: string) => void;
  onExport: () => void;
  onAtlasColumns: (columns: number) => void;
  onExportAnimation: (id: string, format: "gif" | "mp4") => void;
  onSaveMotionLibrary: (id: string) => void;
  onOpenVersions: (id: string) => void;
}

// 우측 메인 프리뷰 영역: 재생 / 프레임 관리 / 아틀라스
export default function PreviewPanel({ state, allStates, directions, cellSize, atlasColumns, busy, onUpdateState, onSelect, onRegenerate, onReprocessVideo, onExport, onAtlasColumns, onExportAnimation, onSaveMotionLibrary, onOpenVersions }: IProps) {
  const { t, lang } = useI18n();
  const [tab, setTab] = useState<ViewTab>("play");
  const [feedback, setFeedback] = useState("");
  const [videoSrc, setVideoSrc] = useState("");
  const [videoLoading, setVideoLoading] = useState(false);
  const [videoError, setVideoError] = useState("");
  const [targetFrameIndex, setTargetFrameIndex] = useState(0);
  const [videoDurationMs, setVideoDurationMs] = useState(0);
  const [videoTimeMs, setVideoTimeMs] = useState(0);
  const [activeCandidateId, setActiveCandidateId] = useState("");
  const [sequenceDraft, setSequenceDraft] = useState<string[]>([]);
  const [sequenceMessage, setSequenceMessage] = useState("");
  const [candidateViewMode, setCandidateViewMode] = useState<"focus" | "all">("focus");
  const videoRef = useRef<HTMLVideoElement>(null);
  const timelineStripRef = useRef<HTMLDivElement>(null);
  const activeCandidateRef = useRef<HTMLButtonElement>(null);

  useEffect(() => setFeedback(state?.feedback ?? ""), [state?.id]);
  useEffect(() => {
    setVideoSrc("");
    setVideoError("");
    setVideoLoading(false);
    setTargetFrameIndex(0);
    setVideoDurationMs(0);
    setVideoTimeMs(0);
    setActiveCandidateId("");
    setSequenceDraft([]);
    setSequenceMessage("");
    setCandidateViewMode("focus");
  }, [state?.id, state?.rawVideoPath]);

  useEffect(() => {
    const candidates = timelineCandidateFrames(state?.candidateFrames ?? []);
    const selected = candidates.find((candidate) => candidate.selected) ?? candidates[0];
    setActiveCandidateId(selected?.id ?? "");
  }, [state?.id, state?.candidateFrames]);

  useEffect(() => {
    const candidates = timelineCandidateFrames(state?.candidateFrames ?? []);
    const candidateIDs = new Set(candidates.map((candidate) => candidate.id));
    const slotCount = state?.items.length || state?.frames || 0;
    const draft = Array<string>(slotCount).fill("");
    for (const selection of state?.trace?.selections ?? []) {
      if (selection.targetFrameIndex >= 0 && selection.targetFrameIndex < slotCount && candidateIDs.has(selection.candidateId)) {
        draft[selection.targetFrameIndex] = selection.candidateId;
      }
    }
    const recommended = candidates.filter((candidate) => candidate.selected);
    for (let i = 0; i < slotCount && i < recommended.length; i++) {
      if (!draft[i]) draft[i] = recommended[i].id;
    }
    setSequenceDraft(draft);
    const firstEmpty = draft.findIndex((id) => !id);
    setTargetFrameIndex(firstEmpty >= 0 ? firstEmpty : 0);
    setSequenceMessage("");
  }, [state?.id, state?.candidateFrames, state?.items.length, state?.frames]);

  useEffect(() => {
    const strip = timelineStripRef.current;
    const active = activeCandidateRef.current;
    if (!strip || !active) return;
    strip.scrollTo({
      left: Math.max(0, active.offsetLeft - strip.clientWidth / 2 + active.clientWidth / 2),
      behavior: "smooth",
    });
  }, [activeCandidateId]);

  useEffect(() => {
    if (tab !== "source" || !state?.rawVideoPath) return;
    let cancelled = false;
    setVideoLoading(true);
    setVideoError("");
    loadVideoReference(state.rawVideoPath)
      .then((src) => {
        if (!cancelled) setVideoSrc(src);
      })
      .catch((e) => {
        if (!cancelled) setVideoError(localizeBackendText(String(e), lang));
      })
      .finally(() => {
        if (!cancelled) setVideoLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [tab, state?.id, state?.rawVideoPath, lang]);

  const doneStates = allStates.filter((s) => s.status === "done" && selectedFrames(s).length > 0);

  if (!state) {
    return (
      <>
        <div className="panel-head">
          <span className="step-badge">3</span>
          <span className="panel-title">{t("preview_export")}</span>
        </div>
        <div className="preview-body">
          <EmptyHero hasResults={doneStates.length > 0} />
        </div>
      </>
    );
  }

  const frames = selectedFrames(state).map((f) => f.png);
  const timelineCandidates = timelineCandidateFrames(state.candidateFrames ?? []);
  const hasVideoTimeline = !!state.rawVideoPath && timelineCandidates.some((candidate) => candidate.timeMs !== undefined);
  const candidateMaxTime = timelineCandidates.reduce((max, candidate) => Math.max(max, candidate.timeMs ?? 0), 0);
  const timelineDurationMs = Math.max(1, videoDurationMs, candidateMaxTime);
  const activeCandidate =
    timelineCandidates.find((candidate) => candidate.id === activeCandidateId) ??
    nearestTimelineCandidate(timelineCandidates, videoTimeMs);
  const visibleTimelineCandidates = candidateViewMode === "all"
    ? timelineCandidates
    : focusedTimelineCandidates(timelineCandidates, activeCandidate?.id, sequenceDraft);
  const candidateByID = new Map(timelineCandidates.map((candidate) => [candidate.id, candidate]));
  const draftCandidates = sequenceDraft.map((id) => candidateByID.get(id)).filter((candidate): candidate is CandidateFrame => !!candidate);
  const draftCount = sequenceDraft.filter(Boolean).length;
  const draftOrderValid = isChronologicalSequence(sequenceDraft, candidateByID);
  const draftMetrics = draftCandidates.length >= 2
    ? analyzeDraftSequence(draftCandidates, timelineDurationMs, state.loop)
    : undefined;
  const displayedSequenceMetrics = draftMetrics ?? state.sequenceMetrics;
  const needsQualityReview = state.status === "review" && !!state.qualityReview && !state.qualityReview.ignored;
  const hasEditableFrames = state.status !== "generating" && frames.length > 0;
  const selectedPipeline = state.pipeline ?? "video_to_frames";
  const usedPipeline = state.lastPipeline ?? selectedPipeline;
  const copyrightBlocked = state.status === "error" && isCopyrightRestrictionError(state.error);
  const loadVideo = async (force = false) => {
    if (!state?.rawVideoPath || (!force && videoSrc) || videoLoading) return;
    setVideoLoading(true);
    setVideoError("");
    if (force) setVideoSrc("");
    try {
      setVideoSrc(await loadVideoReference(state.rawVideoPath));
    } catch (e) {
      setVideoError(localizeBackendText(String(e), lang));
    } finally {
      setVideoLoading(false);
    }
  };

  const toggleFrame = (fid: string) => {
    onUpdateState(state.id, {
      items: state.items.map((f) => (f.id === fid ? { ...f, selected: !f.selected } : f)),
    });
  };

  const moveFrame = (idx: number, dir: -1 | 1) => {
    const items = [...state.items];
    const j = idx + dir;
    if (j < 0 || j >= items.length) return;
    [items[idx], items[j]] = [items[j], items[idx]];
    onUpdateState(state.id, { items });
  };

  const seekTimeline = (timeMs: number, pause = true) => {
    const clamped = Math.max(0, Math.min(timeMs, timelineDurationMs));
    setVideoTimeMs(clamped);
    const nearest = nearestTimelineCandidate(timelineCandidates, clamped);
    if (nearest) setActiveCandidateId(nearest.id);
    const video = videoRef.current;
    if (video && Number.isFinite(video.duration)) {
      if (pause) video.pause();
      video.currentTime = clamped / 1000;
    }
  };

  const selectTimelineCandidate = (candidate: CandidateFrame) => {
    setActiveCandidateId(candidate.id);
    seekTimeline(candidate.timeMs ?? 0);
  };

  const assignCandidateToDraft = (candidate: CandidateFrame) => {
    if (sequenceDraft.length === 0) return;
    const target = Math.max(0, Math.min(targetFrameIndex, sequenceDraft.length - 1));
    const next = [...sequenceDraft];
    next[target] = candidate.id;
    setSequenceDraft(next);
    setSequenceMessage(isChronologicalSequence(next, candidateByID) ? "" : t("sequence_order_warning"));
    const nextEmpty = next.findIndex((id, index) => index > target && !id);
    setTargetFrameIndex(nextEmpty >= 0 ? nextEmpty : Math.min(target + 1, next.length - 1));
  };

  const restoreSmartDraft = () => {
    const recommended = timelineCandidates.filter((candidate) => candidate.selected);
    const source = recommended.length > 0
      ? recommended
      : evenlyPickCandidates(timelineCandidates, sequenceDraft.length);
    const next = Array<string>(sequenceDraft.length).fill("");
    for (let i = 0; i < next.length && i < source.length; i++) next[i] = source[i].id;
    setSequenceDraft(next);
    setTargetFrameIndex(0);
    setSequenceMessage(t("sequence_smart_restored"));
  };

  const clearDraft = () => {
    setSequenceDraft(Array<string>(sequenceDraft.length).fill(""));
    setTargetFrameIndex(0);
    setSequenceMessage("");
  };

  const removeDraftSlot = (index: number) => {
    const next = [...sequenceDraft];
    next[index] = "";
    setSequenceDraft(next);
    setTargetFrameIndex(index);
    setSequenceMessage("");
  };

  const applySequenceDraft = async () => {
    if (draftCount === 0) return;
    if (!draftOrderValid) {
      setSequenceMessage(t("sequence_order_warning"));
      return;
    }
    const items = [...state.items];
    const appliedSelections = [];
    const replacedAt = new Date().toISOString();
    for (let target = 0; target < sequenceDraft.length; target++) {
      const candidate = candidateByID.get(sequenceDraft[target]);
      if (!candidate) continue;
      const candidatePNG = await loadImageReference(candidate.sourcePath || candidate.png || "");
      if (!candidatePNG) continue;
      const next = {
        id: uid("fr"),
        png: candidatePNG,
        selected: true,
        source: `candidate_sequence:${candidate.source}`,
        sourceRef: candidate.id,
        replacement: {
          candidateId: candidate.id,
          source: candidate.source,
          label: candidate.label,
          targetFrameIndex: target,
          timeMs: candidate.timeMs,
          quality: candidate.quality,
          reason: candidate.reason,
          replacedAt,
        },
      };
      if (items[target]) items[target] = next;
      else items.push(next);
      appliedSelections.push({
        targetFrameIndex: target,
        candidateId: candidate.id,
        source: candidate.source,
        timeMs: candidate.timeMs,
        quality: candidate.quality,
        reason: candidate.reason || t("manual_candidate_replace"),
        selectedAt: replacedAt,
      });
    }
    onUpdateState(state.id, {
      items,
      sequenceMetrics: draftMetrics ?? state.sequenceMetrics,
      trace: state.trace
        ? {
            ...state.trace,
            selections: [...(state.trace.selections ?? []), ...appliedSelections],
          }
        : state.trace,
    });
    setSequenceMessage(t("sequence_applied", { n: appliedSelections.length }));
  };

  const syncFromVideo = () => {
    const video = videoRef.current;
    if (!video) return;
    const timeMs = Math.round(video.currentTime * 1000);
    setVideoTimeMs(timeMs);
    const nearest = nearestTimelineCandidate(timelineCandidates, timeMs);
    if (nearest) setActiveCandidateId(nearest.id);
  };

  const replaceWithCandidate = async (candidate: CandidateFrame) => {
    const candidatePNG = await loadImageReference(candidate.sourcePath || candidate.png || "");
    if (!candidatePNG) return;
    const target = Math.max(0, Math.min(targetFrameIndex, Math.max(0, state.items.length - 1)));
    const items = [...state.items];
    const replacedAt = new Date().toISOString();
    const next = {
      id: uid("fr"),
      png: candidatePNG,
      selected: true,
      source: `candidate_replace:${candidate.source}`,
      sourceRef: candidate.id,
      replacement: {
        candidateId: candidate.id,
        source: candidate.source,
        label: candidate.label,
        targetFrameIndex: target,
        timeMs: candidate.timeMs,
        quality: candidate.quality,
        reason: candidate.reason,
        replacedAt,
      },
    };
    if (items[target]) items[target] = next;
    else items.push(next);
    onUpdateState(state.id, {
      items,
      trace: state.trace
        ? {
            ...state.trace,
            selections: [
              ...(state.trace.selections ?? []),
              {
                targetFrameIndex: target,
                candidateId: candidate.id,
                source: candidate.source,
                timeMs: candidate.timeMs,
                quality: candidate.quality,
                reason: candidate.reason || t("manual_candidate_replace"),
                selectedAt: replacedAt,
              },
            ],
          }
        : state.trace,
    });
  };

  const acceptQualityRisk = () => {
    if (!state.qualityReview) return;
    onUpdateState(state.id, {
      status: "done",
      error: undefined,
      qualityReview: {
        ...state.qualityReview,
        ignored: true,
        ignoredAt: new Date().toISOString(),
      },
    });
  };

  return (
    <>
      <div className="panel-head">
        <span className="step-badge">3</span>
        <span className="panel-title">{composeStateLabel(state, lang, t("custom"))}</span>
        {(state.status === "done" || state.status === "review") && state.scores && (
          <div className="score-bar" title={t("score_tooltip")}>
            <span className="score-pill">{t("score")} {(state.scores.overall * 100).toFixed(0)}</span>
            <div className="score-segments">
              <span style={{ width: `${state.scores.identity * 100}%` }} title={`${t("score_identity")}: ${(state.scores.identity * 100).toFixed(0)}`} />
              <span style={{ width: `${state.scores.motion * 100}%` }} title={`${t("score_motion")}: ${(state.scores.motion * 100).toFixed(0)}`} />
              <span style={{ width: `${state.scores.contact * 100}%` }} title={`${t("score_contact")}: ${(state.scores.contact * 100).toFixed(0)}`} />
            </div>
          </div>
        )}
        <span className="hint">{state.name}</span>
        {(state.versionCount ?? 0) > 0 && (
          <Button size="sm" variant="outline" onClick={() => onOpenVersions(state.id)}>
            <History size={12} /> 当前 v{state.currentVersionNumber ?? "-"} · 共 {state.versionCount} 版
          </Button>
        )}
        <span className="spacer" />
        <Tabs value={tab} onValueChange={(v) => setTab(v as ViewTab)}>
          <TabsList className="h-7 w-auto">
            <TabsTrigger value="play" className="flex-none px-3">
              {t("preview")}
            </TabsTrigger>
            <TabsTrigger value="frames" className="flex-none px-3">
              {t("frames")}{state.items.length > 0 ? ` ${frames.length}/${state.items.length}` : ""}
            </TabsTrigger>
            <TabsTrigger value="source" className="flex-none px-3">
              {t("source_trace")}
            </TabsTrigger>
            <TabsTrigger value="atlas" className="flex-none px-3">
              {t("spritesheet")}
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      <div className="preview-body">
        {state.status === "error" && state.error && tab !== "atlas" && (
          <div className={copyrightBlocked ? "preview-error copyright-error" : "preview-error"}>
            {copyrightBlocked && (
              <div className="preview-error-head">
                <AlertTriangle size={15} />
                <strong>{t("copyright_review_title")}</strong>
              </div>
            )}
            <p>{localizeBackendText(state.error, lang)}</p>
            {copyrightBlocked && state.trace && (
              <div className="preview-error-actions">
                <Button size="sm" variant="outline" onClick={() => setTab("source")}>
                  <ListChecks size={12} /> {t("source_trace")}
                </Button>
              </div>
            )}
          </div>
        )}

        {needsQualityReview && tab !== "atlas" && (
          <div className="quality-review-panel">
            <div className="quality-review-head">
              <AlertTriangle size={16} />
              <div>
                <strong>{t("quality_review_title")}</strong>
                <p>{localizeBackendText(state.qualityReview!.summary, lang)}</p>
              </div>
            </div>
            <div className="quality-tool-grid">
              {state.rawVideoPath && (
                <label>
                  <span>{t("frame_sampler_label")}</span>
                  <Select value={state.frameSamplerMode ?? "candidate_pool"} disabled={busy} onValueChange={(v) => onUpdateState(state.id, { frameSamplerMode: v as FrameSamplerMode })}>
                    <SelectTrigger className="h-8"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="candidate_pool">{t("frame_sampler_candidate_pool")}</SelectItem>
                      <SelectItem value="uniform">{t("frame_sampler_uniform")}</SelectItem>
                    </SelectContent>
                  </Select>
                </label>
              )}
              {state.rawVideoPath && (
                <label>
                  <span>{t("alignment_mode_label")}</span>
                  <Select value={state.alignmentMode ?? "preserve_source_canvas"} disabled={busy} onValueChange={(v) => onUpdateState(state.id, { alignmentMode: v as AlignmentMode })}>
                    <SelectTrigger className="h-8"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="preserve_source_canvas">{t("alignment_preserve_source_canvas")}</SelectItem>
                      <SelectItem value="game_anchor_normalize">{t("alignment_game_anchor_normalize")}</SelectItem>
                    </SelectContent>
                  </Select>
                </label>
              )}
              <label>
                <span>{t("frame_fit_mode")}</span>
                <Select value={state.frameFitMode ?? "contain"} disabled={busy} onValueChange={(v) => onUpdateState(state.id, { frameFitMode: v as FrameFitMode })}>
                  <SelectTrigger className="h-8"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="contain">{t("frame_fit_contain")}</SelectItem>
                    <SelectItem value="auto_expand">{t("frame_fit_auto_expand")}</SelectItem>
                    <SelectItem value="crop">{t("frame_fit_crop")}</SelectItem>
                  </SelectContent>
                </Select>
              </label>
              <label>
                <span>抠图模式</span>
                <Select value={state.backgroundRemovalMode ?? "inherit"} disabled={busy} onValueChange={(v) => onUpdateState(state.id, { backgroundRemovalMode: v as BackgroundRemovalMode })}>
                  <SelectTrigger className="h-8"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="inherit">继承全局</SelectItem>
                    <SelectItem value="chroma">色键</SelectItem>
                    <SelectItem value="neural">神经网络</SelectItem>
                    <SelectItem value="hybrid">混合</SelectItem>
                  </SelectContent>
                </Select>
              </label>
            </div>
            <div className="quality-review-actions">
              {(state.candidateFrames?.length ?? 0) > 0 && (
                <Button size="sm" variant="outline" onClick={() => setTab("source")}>
                  <ListChecks size={12} /> {t("review_candidates")}
                </Button>
              )}
              {state.rawVideoPath && (
                <Button size="sm" variant="outline" disabled={busy} onClick={() => onReprocessVideo(state.id)}>
                  <RefreshCw size={12} /> {t("reprocess_with_settings")}
                </Button>
              )}
              <span className="spacer" />
              <Button size="sm" variant="ghost" disabled={busy} onClick={acceptQualityRisk}>
                <Check size={12} /> {t("ignore_quality_check")}
              </Button>
            </div>
          </div>
        )}

        {state.qualityReview?.ignored && tab !== "atlas" && (
          <div className="quality-ignored-note">
            <Settings2 size={12} /> {t("quality_check_ignored")}
          </div>
        )}

        {state.status !== "generating" && state.warnings.length > 0 && tab !== "atlas" && (
          <div className="preview-warn">
            {state.warnings.map((w, i) => (
              <div key={i}>{localizeBackendText(w, lang)}</div>
            ))}
          </div>
        )}

        {state.status === "generating" && (
          <div className="empty-hero">
            <div className="empty-icon">
              <span className="spinner" style={{ width: 26, height: 26, borderWidth: 3 }} />
            </div>
            <h2>{t("generating_state", { label: composeStateLabel(state, lang, t("custom")) })}</h2>
            <p>{state.progress?.message || t("generating_state_desc")}</p>
            {state.progress?.percent !== undefined && (
              <div className="state-progress wide" aria-hidden="true">
                <span style={{ width: `${state.progress.percent}%` }} />
              </div>
            )}
          </div>
        )}

        {state.status !== "generating" && state.items.length === 0 && tab !== "atlas" && tab !== "source" && (
          <div className="empty-hero">
            <div className="empty-icon">
              <Clapperboard size={26} />
            </div>
            <h2>{t("no_frames_yet")}</h2>
            <p>{t("no_frames_hint")}</p>
          </div>
        )}

        {hasEditableFrames && tab === "play" && (
          <>
            <AnimPlayer frames={frames} fps={state.fps} loop={state.loop} cellSize={state.cellSize ?? cellSize} />
            <div className="animation-export-row">
              <Button size="sm" variant="outline" disabled={busy || needsQualityReview} title={needsQualityReview ? t("quality_review_export_blocked") : undefined} onClick={() => onExportAnimation(state.id, "gif")}>
                <Download size={12} /> {t("export_animation_gif")}
              </Button>
              <Button size="sm" variant="outline" disabled={busy || needsQualityReview} title={needsQualityReview ? t("quality_review_export_blocked") : t("export_animation_mp4_tip")} onClick={() => onExportAnimation(state.id, "mp4")}>
                <FileVideo size={12} /> {t("export_animation_mp4")}
              </Button>
            </div>
          </>
        )}

        {tab === "play" && state.dirBase && (
          <>
            <div className="hint" style={{ display: "flex", alignItems: "center", gap: 4 }}>
              <LayoutGrid size={11} /> {t("dir8_hint")}
            </div>
            <DirectionGrid
              states={allStates.filter((s) => s.dirBase === state.dirBase)}
              directions={directions}
              selectedId={state.id}
              onSelect={onSelect}
            />
          </>
        )}

        {hasEditableFrames && tab === "frames" && state.items.length > 0 && (
          <>
            <div className="frame-grid">
              {state.items.map((f, i) => (
                <div
                  key={f.id}
                  className={`frame-cell checker ${f.selected ? "selected" : "deselected"}`}
                  onClick={() => toggleFrame(f.id)}
                  title={f.selected ? t("click_exclude") : t("click_include")}
                >
                  <img src={f.png} className="pixelated" alt={`frame ${i + 1}`} />
                  <span className="fc-num">#{i + 1}</span>
                  <span className="fc-check">{f.selected ? <Check size={11} /> : null}</span>
                  <span className="fc-move" onClick={(e) => e.stopPropagation()}>
                    <button onClick={() => moveFrame(i, -1)} title={t("move_earlier")}>
                      <ChevronLeft size={10} />
                    </button>
                    <button onClick={() => moveFrame(i, 1)} title={t("move_later")}>
                      <ChevronRight size={10} />
                    </button>
                  </span>
                </div>
              ))}
            </div>

            {state.mirrorOf ? (
              <div className="refine-box">
                <span className="rb-title" style={{ display: "flex", alignItems: "center", gap: 4 }}>
                  <FlipHorizontal2 size={11} /> {t("mirror_result", { dir: directionName(state.mirrorOf, lang) })}
                </span>
                <div className="row">
                  <span className="hint">{t("mirror_regen_hint", { dir: directionName(state.mirrorOf, lang) })}</span>
                  <span className="spacer" />
                  <Button size="sm" disabled={busy} onClick={() => onRegenerate(state.id, "")}>
                    <RefreshCw size={12} /> {t("remirror")}
                  </Button>
                </div>
              </div>
            ) : (
              <div className="refine-box">
                <span className="rb-title">{t("refine_title")}</span>
                <Textarea
                  className="min-h-[52px] bg-white"
                  placeholder={t("feedback_ph")}
                  value={feedback}
                  onChange={(e) => setFeedback(e.target.value)}
                />
                <div className="row">
                  <span className="hint">{t("frame_exclude_hint")}</span>
                  <span className="spacer" />
                  <Button size="sm" disabled={busy} onClick={() => onRegenerate(state.id, feedback)}>
                    <RefreshCw size={12} /> {t("regen_feedback")}
                  </Button>
                </div>
              </div>
            )}

            {state.rawStrip && (
              <div className="raw-strip checker">
                <img src={state.rawStrip} className="pixelated" alt={t("raw_strip_alt")} />
              </div>
            )}
            {state.rawVideoPath && (
              <p className="hint" title={state.rawVideoPath}>
                {t("raw_video_saved", { path: state.rawVideoPath })}
              </p>
            )}
          </>
        )}

        {tab === "source" && (
          <div className="source-panel">
            <div className="source-summary">
              <span>{t("pipeline_used")}: {usedPipeline === "image_strip" ? t("pipeline_strip_short") : t("pipeline_video_short")}</span>
              <span>{t("pipeline_selected")}: {selectedPipeline === "image_strip" ? t("pipeline_strip_short") : t("pipeline_video_short")}</span>
              {state.trace && <span>{t("trace_ai_called")}: {state.trace.calledAi ? t("yes") : t("no")}</span>}
              {state.trace?.motionSourceMode && <span>{t("trace_motion_mode")}: {t(`motion_mode_${state.trace.motionSourceMode}`)}</span>}
              {state.trace?.motionSourceName && <span>{t("trace_motion_source")}: {state.trace.motionSourceName}</span>}
              {state.trace?.actualVideoReferenceMode && <span>{t("trace_video_reference")}: {t(`video_reference_${state.trace.actualVideoReferenceMode}`)} {state.trace.videoReferenceField ? `(${state.trace.videoReferenceField})` : ""}</span>}
              {state.trace?.videoSubjectMode && <span>{t("trace_video_subject_mode")}: {t(`video_subject_${state.trace.videoSubjectMode}`)}</span>}
              {state.error && <span className="source-error">{localizeBackendText(state.error, lang)}</span>}
              {state.qualityReview && !state.qualityReview.ignored && <span className="source-error">{state.qualityReview.summary}</span>}
              {state.qualityReview?.ignored && <span>{t("quality_check_ignored")}</span>}
            </div>

            <div className="source-block">
              <div className="source-block-head">
                <span>{t("trace_package")}</span>
                <span className="spacer" />
                {state.items.length > 0 && (
                  <Button size="sm" variant="outline" disabled={busy} onClick={() => onSaveMotionLibrary(state.id)}>
                    <Save size={12} /> {t("save_to_motion_library")}
                  </Button>
                )}
              </div>
              {state.trace ? (
                <>
                  <div className="trace-grid">
                    <span>{t("trace_provider")}</span><strong>{state.trace.provider || "-"}</strong>
                    <span>{t("trace_model")}</span><strong>{state.trace.pipeline === "video_to_frames" ? state.trace.videoModel || "-" : state.trace.model || "-"}</strong>
                    <span>{t("trace_pipeline")}</span><strong>{state.trace.pipeline || "-"}</strong>
                    <span>{t("trace_video_workflow")}</span><strong>{state.trace.videoWorkflow || "-"}</strong>
                    <span>{t("trace_video_subject_mode")}</span><strong>{state.trace.videoSubjectMode ? t(`video_subject_${state.trace.videoSubjectMode}`) : "-"}</strong>
                    <span>{t("trace_video_route")}</span><strong>{state.trace.videoRequestRoute || "-"}</strong>
                    <span>{t("trace_started")}</span><strong>{state.trace.startedAt || "-"}</strong>
                  </div>
                  {(state.trace.videoMediaSlots?.length ?? 0) > 0 && (
                    <div className="trace-grid mt-2">
                      <span>{t("trace_video_slots")}</span>
                      <strong>
                        {state.trace.videoMediaSlots!.map((slot) =>
                          `${slot.slot}:${slot.field}${slot.role ? `/${slot.role}` : ""} x${slot.count}`
                        ).join(" · ")}
                      </strong>
                    </div>
                  )}
                  {state.trace.prompt && (
                    <Textarea className="trace-prompt" value={state.trace.prompt} readOnly />
                  )}
                </>
              ) : (
                <div className="source-empty">{t("trace_empty")}</div>
              )}
            </div>

            {(state.trace?.references?.length ?? 0) > 0 && (
              <div className="source-block">
                <div className="source-block-head">
                  <span>{t("trace_references")} {state.trace!.references!.length}</span>
                </div>
                <div className="reference-list">
                  {state.trace!.references!.map((ref, i) => (
                    <div key={`${state.id}-ref-${i}`} className="reference-row">
                      {(ref.preview || ref.path) && <LocalImage className="reference-thumb checker" reference={ref.preview || ref.path} maxDim={180} alt={ref.label} />}
                      <div className="reference-meta">
                        <strong>{ref.label || ref.kind}</strong>
                        <span>{ref.kind} · {ref.source}</span>
                        {ref.path && <span title={ref.path}>{ref.path}</span>}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {(state.trace?.attempts?.length ?? 0) > 1 && (
              <div className="source-block">
                <div className="source-block-head">
                  <span>{t("trace_attempts")} {state.trace!.attempts!.length}</span>
                </div>
                <div className="attempt-list">
                  {state.trace!.attempts!.map((attempt) => (
                    <details key={`${state.id}-attempt-${attempt.index}`}>
                      <summary>#{attempt.index} · {attempt.model || attempt.pipeline}</summary>
                      <pre>{attempt.prompt}</pre>
                    </details>
                  ))}
                </div>
              </div>
            )}

            {state.rawVideoPath ? (
              <div className="source-block">
                <div className="source-block-head">
                  <span><FileVideo size={13} /> {t("raw_video")}</span>
                  <span className="spacer" />
                  <Button size="sm" variant="outline" disabled={videoLoading} onClick={() => loadVideo(true)}>
                    <FileVideo size={12} /> {videoSrc ? t("reload_video") : videoLoading ? t("loading_video") : t("play_raw_video")}
                  </Button>
                  <Button size="sm" variant="outline" disabled={busy} onClick={() => onReprocessVideo(state.id)}>
                    <RefreshCw size={12} /> {t("reprocess_video_strip")}
                  </Button>
                  <Button size="sm" variant="outline" onClick={() => RevealInFinder(state.rawVideoPath!)}>
                    <ExternalLink size={12} /> {t("open_file")}
                  </Button>
                </div>
                <p className="hint source-path" title={state.rawVideoPath}>{state.rawVideoPath}</p>
                {videoError && <p className="source-error">{videoError}</p>}
                {videoSrc && (
                  <video
                    ref={videoRef}
                    className="raw-video"
                    src={videoSrc}
                    controls
                    autoPlay
                    muted
                    loop
                    playsInline
                    onLoadedMetadata={(event) => {
                      const duration = event.currentTarget.duration;
                      if (Number.isFinite(duration)) setVideoDurationMs(Math.round(duration * 1000));
                    }}
                    onTimeUpdate={syncFromVideo}
                    onSeeked={syncFromVideo}
                  />
                )}
              </div>
            ) : (
              <div className="source-empty">{t("no_raw_video")}</div>
            )}

            {(state.rawExtractedFrames?.length ?? 0) > 0 && (
              <details className="source-evidence-details">
                <summary>
                  <span>{t("raw_extracted_frames")} {state.rawExtractedFrames!.length}</span>
                  <small>{t("raw_evidence_expand")}</small>
                </summary>
                <div className="source-frame-grid">
                  {state.rawExtractedFrames!.map((png, i) => (
                    <div key={`${state.id}-raw-${i}`} className="source-frame checker">
                      <LocalImage reference={png} maxDim={180} alt={`${t("raw_extracted_frames")} ${i + 1}`} />
                      <span>#{i + 1}</span>
                    </div>
                  ))}
                </div>
              </details>
            )}

            {(state.candidateFrames?.length ?? 0) > 0 && (
              <div className="source-block">
                <div className="source-block-head">
                  <span>{t("candidate_frames")} {hasVideoTimeline ? timelineCandidates.length : state.candidateFrames!.length}</span>
                  <span className="spacer" />
                  <span className="source-block-note">{t("candidate_workspace_hint")}</span>
                  {hasVideoTimeline && (
                    <div className="candidate-view-switch" role="group" aria-label={t("candidate_view_mode")}>
                      <button
                        type="button"
                        className={candidateViewMode === "focus" ? "active" : ""}
                        onClick={() => setCandidateViewMode("focus")}
                      >
                        {t("candidate_view_focus")}
                      </button>
                      <button
                        type="button"
                        className={candidateViewMode === "all" ? "active" : ""}
                        onClick={() => setCandidateViewMode("all")}
                      >
                        {t("candidate_view_all")}
                      </button>
                    </div>
                  )}
                </div>
                {hasVideoTimeline ? (
                  <div className="candidate-timeline">
                    {displayedSequenceMetrics && (
                      <div className="sequence-metrics">
                        <SequenceMetric
                          label={t("metric_duplicate_rate")}
                          value={displayedSequenceMetrics.duplicateRate}
                          inverse
                        />
                        <SequenceMetric label={t("metric_continuity")} value={displayedSequenceMetrics.continuity} />
                        <SequenceMetric label={t("metric_motion_coverage")} value={displayedSequenceMetrics.motionCoverage} />
                        {state.loop && (
                          <SequenceMetric label={t("metric_loop_closure")} value={displayedSequenceMetrics.loopClosure ?? 0} />
                        )}
                        <span className="sequence-metric-source">
                          {draftMetrics ? t("metric_source_draft") : t("metric_source_smart")}
                        </span>
                      </div>
                    )}
                    <div className="sequence-composer">
                      <div className="sequence-composer-head">
                        <div>
                          <strong>{t("sequence_composer_title")}</strong>
                          <span>{t("sequence_selected_count", { selected: draftCount, total: sequenceDraft.length })}</span>
                        </div>
                        <div className="sequence-composer-actions">
                          <Button size="sm" variant="outline" onClick={restoreSmartDraft}>
                            <RefreshCw size={12} /> {t("restore_smart_selection")}
                          </Button>
                          <Button size="sm" variant="ghost" disabled={draftCount === 0} onClick={clearDraft}>
                            <Eraser size={12} /> {t("clear_selection")}
                          </Button>
                          <Button
                            size="sm"
                            disabled={draftCount === 0 || !draftOrderValid || state.items.length === 0}
                            onClick={() => void applySequenceDraft()}
                          >
                            <Check size={12} /> {t("apply_sequence")}
                          </Button>
                        </div>
                      </div>
                      <div className="sequence-slots" role="listbox" aria-label={t("final_frame_slots")}>
                        {sequenceDraft.map((candidateID, index) => {
                          const candidate = candidateByID.get(candidateID);
                          const invalid = isSequenceSlotOutOfOrder(sequenceDraft, index, candidateByID);
                          return (
                            <div key={`${state.id}-sequence-slot-${index}`} className="sequence-slot-wrap">
                              <button
                                type="button"
                                role="option"
                                aria-selected={targetFrameIndex === index}
                                className={`sequence-slot checker ${targetFrameIndex === index ? "active" : ""} ${candidate ? "filled" : ""} ${invalid ? "invalid" : ""}`}
                                onClick={() => {
                                  setTargetFrameIndex(index);
                                  if (candidate) selectTimelineCandidate(candidate);
                                }}
                              >
                                {candidate && (
                                  <LocalImage
                                    className="pixelated"
                                    reference={candidate.png || candidate.sourcePath}
                                    maxDim={120}
                                    alt={`${t("final_frames")} ${index + 1}`}
                                  />
                                )}
                                <b>#{index + 1}</b>
                                <small>{candidate ? formatTimelineTime(candidate.timeMs ?? 0) : t("slot_empty")}</small>
                              </button>
                              {candidate && (
                                <button
                                  type="button"
                                  className="sequence-slot-clear"
                                  title={t("clear_slot")}
                                  onClick={() => removeDraftSlot(index)}
                                >
                                  <X size={9} />
                                </button>
                              )}
                            </div>
                          );
                        })}
                      </div>
                      <div className={`sequence-composer-status ${!draftOrderValid ? "error" : ""}`}>
                        <span>{sequenceMessage || t("sequence_composer_hint", { frame: Math.min(targetFrameIndex + 1, sequenceDraft.length) })}</span>
                        {!draftOrderValid && <strong>{t("sequence_order_warning")}</strong>}
                      </div>
                    </div>
                    <div className="timeline-readout">
                      <span><Clock3 size={12} /> {t("video_playhead")}</span>
                      <strong>{formatTimelineTime(videoTimeMs)} / {formatTimelineTime(timelineDurationMs)}</strong>
                      <span className="spacer" />
                      <span>{t("timeline_sync_hint")}</span>
                    </div>
                    <div className="timeline-control">
                      <input
                        type="range"
                        min={0}
                        max={timelineDurationMs}
                        step={1}
                        value={Math.min(videoTimeMs, timelineDurationMs)}
                        aria-label={t("candidate_timeline")}
                        onChange={(event) => seekTimeline(Number(event.target.value) || 0)}
                      />
                      <div className="timeline-markers">
                        {timelineCandidates.map((candidate, index) => (
                          <button
                            key={`${candidate.id}-marker`}
                            type="button"
                            className={`${candidate.id === activeCandidate?.id ? "active" : ""} ${candidate.selected ? "recommended" : ""} ${sequenceDraft.includes(candidate.id) ? "assigned" : ""}`}
                            style={{ left: `${((candidate.timeMs ?? 0) / timelineDurationMs) * 100}%` }}
                            title={`#${index + 1} · ${formatTimelineTime(candidate.timeMs ?? 0)}`}
                            onClick={() => selectTimelineCandidate(candidate)}
                          />
                        ))}
                      </div>
                      <div className="timeline-scale">
                        <span>0:00.000</span>
                        <span>{formatTimelineTime(timelineDurationMs)}</span>
                      </div>
                    </div>
                    <div ref={timelineStripRef} className="timeline-filmstrip" role="listbox" aria-label={t("candidate_timeline")}>
                      {visibleTimelineCandidates.map((candidate) => {
                        const index = timelineCandidates.findIndex((item) => item.id === candidate.id);
                        return (
                          <button
                            ref={candidate.id === activeCandidate?.id ? activeCandidateRef : undefined}
                            key={`${state.id}-timeline-${candidate.id || index}`}
                            type="button"
                            role="option"
                            aria-selected={candidate.id === activeCandidate?.id}
                            className={`timeline-frame checker ${candidate.id === activeCandidate?.id ? "active" : ""} ${candidate.selected ? "recommended" : ""} ${sequenceDraft.includes(candidate.id) ? "assigned" : ""} ${candidate.nearDuplicate ? "near-duplicate" : ""}`}
                            onClick={() => selectTimelineCandidate(candidate)}
                          >
                            <LocalImage className="pixelated" reference={candidate.png || candidate.sourcePath} maxDim={180} alt={`${candidate.label} ${index + 1}`} />
                            <span>#{index + 1}</span>
                            {candidate.nearDuplicate && <em>{t("candidate_near_duplicate")}</em>}
                            <small>{formatTimelineTime(candidate.timeMs ?? 0)}</small>
                          </button>
                        );
                      })}
                    </div>
                    {activeCandidate && (
                      <div className="timeline-selection">
                        <div className="timeline-selected-preview checker">
                          <LocalImage className="pixelated" reference={activeCandidate.png || activeCandidate.sourcePath} maxDim={256} alt={activeCandidate.label} />
                        </div>
                        <div className="timeline-selected-meta">
                          <strong>{t("selected_candidate")} · {formatTimelineTime(activeCandidate.timeMs ?? 0)}</strong>
                          <span>{activeCandidate.reason || activeCandidate.label}</span>
                          {activeCandidate.quality !== undefined && (
                            <span>{t("score")}: {(activeCandidate.quality * 100).toFixed(0)}</span>
                          )}
                          <span>
                            {t("candidate_motion")}: {Math.round((activeCandidate.motion ?? 0) * 100)}
                            {" · "}
                            {t("metric_continuity")}: {Math.round((activeCandidate.continuity ?? 0) * 100)}
                          </span>
                        </div>
                        <div className="timeline-selection-actions">
                          <Button size="sm" variant="outline" disabled={sequenceDraft.length === 0} onClick={() => assignCandidateToDraft(activeCandidate)}>
                            <Check size={12} /> {t("assign_to_slot", { frame: targetFrameIndex + 1 })}
                          </Button>
                          <Button size="sm" disabled={state.items.length === 0} onClick={() => void replaceWithCandidate(activeCandidate)}>
                            <Replace size={12} /> {t("replace_single_frame", { frame: targetFrameIndex + 1 })}
                          </Button>
                        </div>
                      </div>
                    )}
                  </div>
                ) : (
                  <div className="source-frame-grid">
                    {state.candidateFrames!.map((candidate, i) => (
                      <div key={`${state.id}-candidate-${candidate.id || i}`} className="source-frame checker candidate-frame">
                        <LocalImage className="pixelated" reference={candidate.png || candidate.sourcePath} maxDim={180} alt={`${candidate.label} ${i + 1}`} />
                        <span>#{(candidate.index ?? i) + 1}{candidate.selected ? ` · ${t("candidate_selected")}` : ""}</span>
                        {(candidate.timeMs !== undefined || candidate.quality !== undefined) && (
                          <small title={candidate.reason || ""}>
                            {candidate.timeMs !== undefined ? `${candidate.timeMs}ms` : ""}
                            {candidate.quality !== undefined ? ` · ${(candidate.quality * 100).toFixed(0)}` : ""}
                            {candidate.reason ? ` · ${candidate.reason}` : ""}
                          </small>
                        )}
                        <button onClick={() => void replaceWithCandidate(candidate)}>{t("replace_final_frame")}</button>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {state.rawStrip && (
              <div className="source-block">
                <div className="source-block-head">
                  <span>{t("processed_strip")}</span>
                </div>
                <div className="raw-strip checker">
                  <img src={state.rawStrip} className="pixelated" alt={t("raw_strip_alt")} />
                </div>
              </div>
            )}

            {state.items.length > 0 && (
              <div className="source-block">
                <div className="source-block-head">
                  <span>{t("final_frames")} {state.items.length}</span>
                </div>
                <div className="source-frame-grid">
                  {state.items.map((f, i) => (
                    <div key={`${state.id}-final-${f.id}`} className="source-frame checker">
                      <img className="pixelated" src={f.png} alt={`${t("final_frames")} ${i + 1}`} />
                      <span>#{i + 1}</span>
                      {f.replacement && <small>{t("frame_replaced_from")}: {f.replacement.label || f.replacement.source}</small>}
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}

        {tab === "atlas" && <AtlasView states={doneStates} cellSize={cellSize} columns={atlasColumns} onColumns={onAtlasColumns} onExport={onExport} />}
      </div>
    </>
  );
}

const loadVideoReference = (reference: string) => reference.startsWith("version-resource:")
  ? LoadAnimationVersionResource(reference, 0)
  : LoadVideoDataURL(reference);

function timelineCandidateFrames(candidates: CandidateFrame[]): CandidateFrame[] {
  const byTime = new Map<string, CandidateFrame>();
  for (const candidate of candidates) {
    const time = candidate.timeMs ?? (candidate.index === 0 ? 0 : undefined);
    const key = time === undefined ? `id:${candidate.id}` : `time:${time}`;
    const current = byTime.get(key);
    const rank = (value: CandidateFrame) =>
      (value.source.includes("processed") ? 4 : 0) +
      (value.selected ? 2 : 0) +
      (value.sourcePath ? 1 : 0);
    if (!current || rank(candidate) > rank(current)) {
      byTime.set(key, time === candidate.timeMs ? candidate : { ...candidate, timeMs: time });
    }
  }
  return [...byTime.values()].sort((a, b) => {
    const timeDiff = (a.timeMs ?? Number.MAX_SAFE_INTEGER) - (b.timeMs ?? Number.MAX_SAFE_INTEGER);
    return timeDiff || a.index - b.index;
  });
}

function nearestTimelineCandidate(candidates: CandidateFrame[], timeMs: number): CandidateFrame | undefined {
  let best: CandidateFrame | undefined;
  let bestDistance = Number.POSITIVE_INFINITY;
  for (const candidate of candidates) {
    const distance = Math.abs((candidate.timeMs ?? 0) - timeMs);
    if (distance < bestDistance) {
      best = candidate;
      bestDistance = distance;
    }
  }
  return best;
}

function focusedTimelineCandidates(candidates: CandidateFrame[], activeID: string | undefined, draft: string[]): CandidateFrame[] {
  if (candidates.length <= 24) return candidates;
  const included = new Set<string>(draft.filter(Boolean));
  for (const candidate of candidates) {
    if (candidate.selected) included.add(candidate.id);
  }
  const activeIndex = candidates.findIndex((candidate) => candidate.id === activeID);
  if (activeIndex >= 0) {
    for (let index = Math.max(0, activeIndex - 2); index <= Math.min(candidates.length - 1, activeIndex + 2); index++) {
      included.add(candidates[index].id);
    }
  }
  return candidates.filter((candidate) => included.has(candidate.id));
}

function formatTimelineTime(timeMs: number): string {
  const safe = Math.max(0, Math.round(timeMs));
  const minutes = Math.floor(safe / 60000);
  const seconds = Math.floor((safe % 60000) / 1000);
  const milliseconds = safe % 1000;
  return `${minutes}:${String(seconds).padStart(2, "0")}.${String(milliseconds).padStart(3, "0")}`;
}

function SequenceMetric({ label, value, inverse = false }: { label: string; value: number; inverse?: boolean }) {
  const safe = Math.max(0, Math.min(1, Number.isFinite(value) ? value : 0));
  const good = inverse ? safe <= 0.12 : safe >= 0.78;
  const warning = inverse ? safe >= 0.30 : safe < 0.52;
  return (
    <div className={`sequence-metric ${good ? "good" : warning ? "warning" : ""}`}>
      <span>{label}</span>
      <strong>{Math.round(safe * 100)}%</strong>
      <i><b style={{ width: `${inverse ? (1 - safe) * 100 : safe * 100}%` }} /></i>
    </div>
  );
}

function isChronologicalSequence(draft: string[], candidates: Map<string, CandidateFrame>): boolean {
  let previous = -1;
  for (const id of draft) {
    if (!id) continue;
    const time = candidates.get(id)?.timeMs;
    if (time === undefined) continue;
    if (time <= previous) return false;
    previous = time;
  }
  return true;
}

function isSequenceSlotOutOfOrder(draft: string[], index: number, candidates: Map<string, CandidateFrame>): boolean {
  const current = candidates.get(draft[index]);
  if (!current || current.timeMs === undefined) return false;
  for (let i = index - 1; i >= 0; i--) {
    const previous = candidates.get(draft[i]);
    if (previous?.timeMs !== undefined) return previous.timeMs >= current.timeMs;
  }
  for (let i = index + 1; i < draft.length; i++) {
    const next = candidates.get(draft[i]);
    if (next?.timeMs !== undefined) return current.timeMs >= next.timeMs;
  }
  return false;
}

function evenlyPickCandidates(candidates: CandidateFrame[], count: number): CandidateFrame[] {
  if (count <= 0 || candidates.length === 0) return [];
  if (count === 1) return [candidates[Math.floor(candidates.length / 2)]];
  const out: CandidateFrame[] = [];
  for (let i = 0; i < count; i++) {
    out.push(candidates[Math.round((i * (candidates.length - 1)) / (count - 1))]);
  }
  return out;
}

function analyzeDraftSequence(candidates: CandidateFrame[], durationMs: number, loop: boolean): FrameSequenceMetrics {
  if (candidates.length < 2) {
    return { duplicateRate: 0, continuity: 1, motionCoverage: candidates.length > 0 ? 1 : 0, loopClosure: loop ? 1 : undefined };
  }
  let duplicates = 0;
  const visualSteps: number[] = [];
  const timeSteps: number[] = [];
  for (let i = 1; i < candidates.length; i++) {
    const current = candidates[i];
    let duplicate = !!current.nearDuplicate;
    if (current.fingerprint) {
      duplicate = false;
      for (let j = 0; j < i; j++) {
        if (loop && i === candidates.length - 1 && j === 0) continue;
        const distance = fingerprintDistance(current.fingerprint, candidates[j].fingerprint);
        if (distance >= 0 && distance <= 6) {
          duplicate = true;
          break;
        }
      }
    }
    if (duplicate) duplicates++;
    const distance = fingerprintDistance(candidates[i - 1].fingerprint, current.fingerprint);
    visualSteps.push(distance >= 0 ? distance / 64 : Math.max(0, Math.min(1, current.motion ?? 0)));
    timeSteps.push(Math.max(0, (current.timeMs ?? 0) - (candidates[i - 1].timeMs ?? 0)));
  }

  const duplicateRate = duplicates / Math.max(1, candidates.length - 1);
  const visualContinuity = smoothStepScore(visualSteps, 0.08);
  const temporalContinuity = smoothStepScore(timeSteps, 20);
  const candidateContinuity = candidates.reduce((sum, candidate) => sum + (candidate.continuity ?? 1), 0) / candidates.length;
  const continuity = clampMetric(0.50 * visualContinuity + 0.32 * temporalContinuity + 0.18 * candidateContinuity);
  const firstTime = candidates[0].timeMs ?? 0;
  const lastTime = candidates[candidates.length - 1].timeMs ?? firstTime;
  const spanCoverage = clampMetric((lastTime - firstTime) / Math.max(1, durationMs));
  const motionCoverage = clampMetric(0.48 * spanCoverage + 0.52 * (1 - duplicateRate));
  let loopClosure: number | undefined;
  if (loop) {
    const seam = fingerprintDistance(candidates[0].fingerprint, candidates[candidates.length - 1].fingerprint);
    loopClosure = seam >= 0 ? clampMetric(1 - seam / 24) : 0;
  }
  return { duplicateRate, continuity, motionCoverage, loopClosure, activeStartMs: firstTime, activeEndMs: lastTime };
}

function smoothStepScore(values: number[], floor: number): number {
  if (values.length <= 1) return 1;
  const sorted = [...values].sort((a, b) => a - b);
  const median = sorted[Math.floor(sorted.length / 2)];
  const scale = Math.max(floor, median);
  let total = 0;
  for (let i = 1; i < values.length; i++) {
    total += Math.exp(-Math.abs(values[i] - values[i - 1]) / scale);
  }
  return clampMetric(total / (values.length - 1));
}

function fingerprintDistance(a?: string, b?: string): number {
  if (!a || !b || a.length !== b.length) return -1;
  const bitCounts = [0, 1, 1, 2, 1, 2, 2, 3, 1, 2, 2, 3, 2, 3, 3, 4];
  let distance = 0;
  for (let i = 0; i < a.length; i++) {
    const left = Number.parseInt(a[i], 16);
    const right = Number.parseInt(b[i], 16);
    if (Number.isNaN(left) || Number.isNaN(right)) return -1;
    distance += bitCounts[left ^ right];
  }
  return distance;
}

function clampMetric(value: number): number {
  return Math.max(0, Math.min(1, Number.isFinite(value) ? value : 0));
}

function EmptyHero({ hasResults }: { hasResults: boolean }) {
  const { t } = useI18n();
  return (
    <div className="empty-hero">
      <div className="empty-icon">
        <Wand2 size={26} />
      </div>
      <h2>{hasResults ? t("select_anim") : t("hero_title")}</h2>
      <p>{t("hero_desc")}</p>
      <div className="steps">
        <div className="step-item">
          <span className="n">1</span>
          <span className="hint">{t("step_char")}</span>
        </div>
        <ChevronRight size={14} className="step-arrow" />
        <div className="step-item">
          <span className="n">2</span>
          <span className="hint">{t("step_anim")}</span>
        </div>
        <ChevronRight size={14} className="step-arrow" />
        <div className="step-item">
          <span className="n">3</span>
          <span className="hint">{t("step_gen")}</span>
        </div>
        <ChevronRight size={14} className="step-arrow" />
        <div className="step-item">
          <span className="n">4</span>
          <span className="hint">{t("step_export")}</span>
        </div>
      </div>
    </div>
  );
}

// 클라이언트 사이드 아틀라스 미리보기
function AtlasView({ states, cellSize, columns, onColumns, onExport }: { states: StateDef[]; cellSize: number; columns: number; onColumns: (columns: number) => void; onExport: () => void }) {
  const { t } = useI18n();
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  const [dims, setDims] = useState({ w: 0, h: 0 });
  const [rowsCount, setRowsCount] = useState(0);
  const [previewCellSize, setPreviewCellSize] = useState(cellSize);
  const [zoom, setZoom] = useState(1);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    if (states.length === 0) {
      setDims({ w: 0, h: 0 });
      return;
    }
    let cancelled = false;

    (async () => {
      const rows = await Promise.all(
        states.map(async (s) => {
          const imgs = await Promise.all(
            selectedFrames(s).map(
              (f) =>
                new Promise<HTMLImageElement>((resolve) => {
                  const img = new Image();
                  img.onload = () => resolve(img);
                  img.onerror = () => resolve(img);
                  img.src = f.png;
                })
            )
          );
          return { name: s.name, imgs };
        })
      );
      if (cancelled) return;

      const effectiveCellSize = Math.max(cellSize, ...states.map((s) => s.cellSize ?? cellSize));
      setPreviewCellSize(effectiveCellSize);
      const safeColumns = Math.max(1, columns);
      const maxFrames = Math.max(1, ...rows.map((r) => r.imgs.length));
      const usedColumns = Math.min(safeColumns, maxFrames);
      const totalRows = rows.reduce((sum, row) => sum + Math.max(1, Math.ceil(row.imgs.length / safeColumns)), 0);
      const w = usedColumns * effectiveCellSize;
      const h = totalRows * effectiveCellSize;
      canvas.width = w;
      canvas.height = h;
      setDims({ w, h });
      setRowsCount(totalRows);
      const ctx = canvas.getContext("2d")!;
      ctx.imageSmoothingEnabled = false;
      ctx.clearRect(0, 0, w, h);
      let rowOffset = 0;
      rows.forEach((row) => {
        row.imgs.forEach((img, index) => {
          const col = index % safeColumns;
          const rowIndex = rowOffset + Math.floor(index / safeColumns);
          if (img.naturalWidth) ctx.drawImage(img, col * effectiveCellSize, rowIndex * effectiveCellSize, effectiveCellSize, effectiveCellSize);
        });
        rowOffset += Math.max(1, Math.ceil(row.imgs.length / safeColumns));
      });
      requestAnimationFrame(() => {
        const wrap = wrapRef.current;
        if (!wrap) return;
        const fit = Math.min((wrap.clientWidth - 18) / w, (wrap.clientHeight - 18) / h, 1);
        setZoom(Math.max(0.05, fit));
      });
    })();

    return () => {
      cancelled = true;
    };
  }, [states, cellSize, columns]);

  const fitToView = () => {
    const wrap = wrapRef.current;
    if (!wrap || dims.w < 1 || dims.h < 1) return;
    setZoom(Math.max(0.05, Math.min((wrap.clientWidth - 18) / dims.w, (wrap.clientHeight - 18) / dims.h, 1)));
  };

  if (states.length === 0) {
    return (
      <div className="empty-hero">
        <div className="empty-icon">
          <LayoutGrid size={26} />
        </div>
        <h2>{t("no_done_anim")}</h2>
        <p>{t("atlas_empty_hint")}</p>
      </div>
    );
  }

  return (
    <>
      <div className="row">
        <div className="atlas-meta">
          <span>{t("sheet_size", { w: dims.w, h: dims.h })}</span>
          <span>{t("cell", { n: previewCellSize })}</span>
          <span>{t("rows_states", { n: states.length })}</span>
          <span>{t("rows_total", { n: rowsCount })}</span>
        </div>
        <span className="spacer" />
        <div className="inline-select">
          <span>{t("atlas_columns")}</span>
          <Select value={`${columns}`} onValueChange={(v) => onColumns(Math.max(1, Number(v) || 8))}>
            <SelectTrigger className="h-7 w-[72px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {[4, 6, 8, 10, 12, 16, 24, 32].map((n) => <SelectItem key={n} value={`${n}`}>{n}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
        <Button variant="outline" size="icon-sm" onClick={() => setZoom((z) => Math.max(0.05, z - 0.1))} title={t("atlas_zoom", { n: Math.round(zoom * 100) })}>
          <ZoomOut size={12} />
        </Button>
        <Button variant="outline" size="icon-sm" onClick={fitToView} title={t("fit_atlas_view")}>
          <Maximize2 size={12} />
        </Button>
        <Button variant="outline" size="icon-sm" onClick={() => setZoom((z) => Math.min(2, z + 0.1))} title={t("atlas_zoom", { n: Math.round(zoom * 100) })}>
          <ZoomIn size={12} />
        </Button>
        <Button onClick={onExport}>
          <Package size={13} /> {t("export_full")}
        </Button>
      </div>
      <div className="atlas-wrap checker" ref={wrapRef}>
        <canvas ref={canvasRef} className="atlas-canvas" style={{ width: `${dims.w * zoom}px`, height: `${dims.h * zoom}px` }} />
      </div>
      <p className="hint">{t("atlas_hint")}</p>
    </>
  );
}
