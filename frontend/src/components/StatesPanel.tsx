import { useState } from "react";
import { Compass, FlipHorizontal2, Loader2, Plus, ShieldAlert, Sparkles, TimerReset, X, Zap } from "lucide-react";
import { AlignmentMode, DirectionInfo, FrameFitMode, FrameSamplerMode, MotionLibraryItem, MotionSourceMode, PresetInfo, StateDef, VideoPromptLanguage, VideoReferenceMode, VideoShadowMode, VideoSubjectMode, presetInfoToState, selectedFrames } from "../types";
import { useI18n } from "../i18n";
import { composeStateLabel, directionName } from "../i18n/catalog";
import { localizeBackendText } from "../i18n/backend";
import { findIPRiskTerms } from "../ipRisk";
import PresetPicker from "./PresetPicker";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select";
import { Textarea } from "./ui/textarea";

interface IProps {
  states: StateDef[];
  directions: DirectionInfo[];
  presets: PresetInfo[];
  selectedId: string | null;
  canGenerate: boolean;
  hasImage: boolean;
  supportsVideo: boolean;
  characterDescription: string;
  defaultPipeline: "video_to_frames" | "image_strip";
  defaultVideoSeconds: number;
  defaultFrameFitMode: FrameFitMode;
  motionLibrary: MotionLibraryItem[];
  busy: boolean;
  onStates: (s: StateDef[]) => void;
  onSelect: (id: string) => void;
  onGenerate: (id: string) => void;
  onGenerateAll: () => void;
  onAddCustomBatch: (count: number) => void;
  onGenerateDirectionSet: (id: string) => void;
  onImportMotionLibrary: (kind: "file" | "folder", id: string) => void;
}

// 3단계: 애니메이션 상태 구성 패널
export default function StatesPanel({
  states,
  directions,
  presets,
  selectedId,
  canGenerate,
  hasImage,
  supportsVideo,
  characterDescription,
  defaultPipeline,
  defaultVideoSeconds,
  defaultFrameFitMode,
  motionLibrary,
  busy,
  onStates,
  onSelect,
  onGenerate,
  onGenerateAll,
  onAddCustomBatch,
  onGenerateDirectionSet,
  onImportMotionLibrary,
}: IProps) {
  const { t, lang } = useI18n();
  const [pickerOpen, setPickerOpen] = useState(false);
  const [customCount, setCustomCount] = useState(1);
  const ipRiskMatches = findIPRiskTerms(characterDescription, lang === "zh");

  const addPresetInfo = (p: PresetInfo) => {
    const st = {
      ...presetInfoToState(p),
      pipeline: defaultPipeline,
      motionSourceMode: "none" as MotionSourceMode,
      videoReferenceMode: "identity_reference" as VideoReferenceMode,
      videoSubjectMode: "include_description" as VideoSubjectMode,
      videoPromptLanguage: "auto" as VideoPromptLanguage,
      videoShadowMode: "avoid" as VideoShadowMode,
      frameSamplerMode: "candidate_pool" as FrameSamplerMode,
      alignmentMode: defaultPipeline === "video_to_frames" ? "preserve_source_canvas" as AlignmentMode : "game_anchor_normalize" as AlignmentMode,
      frameFitMode: defaultFrameFitMode,
    };
    onStates([...states, st]);
    onSelect(st.id);
  };

  const addCustom = () => {
    onAddCustomBatch(Math.max(1, Math.min(10, customCount)));
  };

  const update = (id: string, patch: Partial<StateDef>) => {
    onStates(states.map((s) => (s.id === id ? { ...s, ...patch } : s)));
  };

  const remove = (id: string) => {
    onStates(states.filter((s) => s.id !== id));
  };

  const usedNames = new Set(states.map((s) => s.name));
  const pendingCount = states.filter((s) => s.status !== "done").length;
  const modeOf = (s: StateDef): MotionSourceMode => s.motionSourceMode ?? "none";
  const usesVideoPipeline = (s: StateDef) => {
    const mode = modeOf(s);
    return !s.mirrorOf && (mode === "ai_video" || (mode === "none" && (s.pipeline ?? defaultPipeline) === "video_to_frames"));
  };
  const requiresIPReview = (s: StateDef) =>
    ipRiskMatches.length > 0 && usesVideoPipeline(s) && (s.videoSubjectMode ?? "include_description") === "auto";
  const pendingAI = states.filter((s) => s.status !== "done" && !s.mirrorOf && modeOf(s) !== "direct").length;
  // 미생성에 AI 상태가 있으면 API 키 필요, 미러 상태만 남았으면 즉시 가능
  const canGenerateAll = (pendingAI > 0 ? canGenerate : pendingCount > 0) && !states.some(requiresIPReview);
  // 방향 선택지: AI로 생성 가능한 5방향만 (미러 방향은 8방향 세트로만 생성)
  const aiDirections = directions.filter((d) => !d.mirrorOf);

  const autoMatchedFPS = (state: StateDef, motion?: MotionLibraryItem) => {
    const frameCount = selectedFrames(state).length || state.frames;
    const selections = [...(state.trace?.selections ?? [])]
      .filter((selection) => Number.isFinite(selection.timeMs))
      .sort((a, b) => a.targetFrameIndex - b.targetFrameIndex);
    let durationSeconds = 0;
    if (selections.length >= 2) {
      const first = selections[0].timeMs ?? 0;
      const last = selections[selections.length - 1].timeMs ?? first;
      const span = Math.max(0, last - first);
      if (span > 0) {
        const interval = span / (selections.length - 1);
        durationSeconds = (span + interval) / 1000;
      }
    }
    if (durationSeconds <= 0 && motion?.sourceDurationMs) {
      durationSeconds = motion.sourceDurationMs / 1000;
    }
    const traceVideoSeconds = Number(state.trace?.parameters?.videoSeconds);
    if (
      durationSeconds <= 0 &&
      (state.trace?.pipeline === "video_to_frames" || state.lastPipeline === "video_to_frames") &&
      Number.isFinite(traceVideoSeconds) &&
      traceVideoSeconds > 0
    ) {
      durationSeconds = traceVideoSeconds;
    }
    if (
      durationSeconds <= 0 &&
      (usesVideoPipeline(state) || state.lastPipeline === "video_to_frames")
    ) {
      durationSeconds = Math.max(1, defaultVideoSeconds);
    }
    if (durationSeconds <= 0) {
      const preset = presets.find((item) => item.name === state.name);
      durationSeconds = preset && preset.fps > 0
        ? preset.frames / preset.fps
        : state.loop ? 1 : 1.2;
    }
    return Math.max(1, Math.min(30, Math.round(frameCount / Math.max(0.1, durationSeconds))));
  };

  return (
    <>
      <div className="panel-head">
        <span className="step-badge">2</span>
        <span className="panel-title">{t("animation")}</span>
        {states.length > 0 && <span className="hint">{t("items_count", { n: states.length })}</span>}
      </div>

      <div className="panel-body">
      <div className="preset-chips">
        <Button size="sm" variant="default" className="add-anim-btn" onClick={() => setPickerOpen(true)}>
          <Plus size={13} /> {t("add_animation")}
        </Button>
        <button className="chip" onClick={addCustom} disabled={busy}>
          + {customCount > 1 ? t("custom_n", { n: customCount }) : t("custom")}
        </button>
        <div className="custom-batch">
          <label>{t("count_label")}</label>
          <Input
            type="number"
            className="h-7"
            min={1}
            max={10}
            disabled={busy}
            value={customCount}
            onChange={(e) => setCustomCount(Math.max(1, Math.min(10, Number(e.target.value) || 1)))}
            title={t("custom_count_tip")}
          />
        </div>
      </div>

      {states.length === 0 && (
        <p className="hint">{t("states_empty_hint", { n: presets.length || 100 })}</p>
      )}

      {pickerOpen && (
        <PresetPicker
          presets={presets}
          usedNames={usedNames}
          onAdd={addPresetInfo}
          onClose={() => setPickerOpen(false)}
        />
      )}

      <div className="state-list">
        {states.map((s) => {
          const sel = selectedFrames(s).length;
          const mode = modeOf(s);
          const isCustom = s.isCustom === true || /^custom\d*$/i.test(s.name);
          const needsIPReview = requiresIPReview(s);
          const usesLibrary = mode === "direct" || mode === "guide_image_strip";
          const selectedMotion = motionLibrary.find((item) => item.id === s.motionSourceId);
          const playbackFrameCount = sel || s.frames;
          const matchedFPS = autoMatchedFPS(s, selectedMotion);
          const canGenerateThisForSource = s.mirrorOf
            ? true
            : mode === "direct"
              ? !!s.motionSourceId
              : mode === "guide_image_strip"
                ? canGenerate && !!s.motionSourceId
                : mode === "ai_video"
                  ? canGenerate && supportsVideo
                  : canGenerate;
          const canGenerateThis = canGenerateThisForSource && !needsIPReview;
          return (
            <div
              key={s.id}
              className={`state-card ${selectedId === s.id ? "active" : ""}`}
              onClick={() => onSelect(s.id)}
            >
              <div className="state-card-head">
                <span className={`status-dot ${s.status}`} />
                <span className="state-name">{composeStateLabel(s, lang, t("custom"))}</span>
                <span className="state-sub">{s.name}</span>
                {s.facing && (
                  <span
                    className="state-sub"
                    title={s.mirrorOf ? t("mirror_of_tip", { dir: directionName(s.mirrorOf, lang) }) : t("facing_tip", { dir: directionName(s.facing, lang) })}
                  >
                    {s.mirrorOf ? (
                      <FlipHorizontal2 size={9} style={{ display: "inline" }} />
                    ) : (
                      <Compass size={9} style={{ display: "inline" }} />
                    )}{" "}
                    {directionName(s.facing, lang)}
                  </span>
                )}
                <span className="spacer" />
                {(s.versionCount ?? 0) > 0 && <span className="state-sub">当前 v{s.currentVersionNumber ?? "-"} · 共 {s.versionCount} 版</span>}
                {s.status === "done" && <span className="state-sub">{t("n_frames", { n: sel })}</span>}
                <Button
                  variant="destructive-ghost"
                  size="icon-sm"
                  title={s.status === "generating" ? t("del_disabled") : t("delete")}
                  disabled={s.status === "generating"}
                  onClick={(e) => {
                    e.stopPropagation();
                    remove(s.id);
                  }}
                >
                  <X size={11} />
                </Button>
              </div>

              {selectedId === s.id && (
                <>
                  <div className="state-card-controls" onClick={(e) => e.stopPropagation()}>
                    <div className="mini-field">
                      <label>{t("frames_count")}</label>
                      <Input
                        type="number"
                        className="h-7"
                        min={1}
                        max={120}
                        value={s.frames}
                        onChange={(e) => update(s.id, { frames: Math.max(1, Math.min(120, Number(e.target.value) || 1)) })}
                      />
                    </div>
                    <div className="mini-field">
                      <label>FPS</label>
                      <Input
                        type="number"
                        className="h-7"
                        min={1}
                        max={30}
                        value={s.fps}
                        onChange={(e) => update(s.id, { fps: Math.max(1, Math.min(30, Number(e.target.value) || 1)) })}
                      />
                    </div>
                    <div className="mini-field">
                      <label>{t("loop_label")}</label>
                      <Select value={s.loop ? "1" : "0"} onValueChange={(v) => update(s.id, { loop: v === "1" })}>
                        <SelectTrigger className="h-7">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="1">{t("loop")}</SelectItem>
                          <SelectItem value="0">{t("once")}</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                  <div className="fps-match-row" onClick={(e) => e.stopPropagation()}>
                    <span>{t("playback_duration", { seconds: (playbackFrameCount / Math.max(1, s.fps)).toFixed(2) })}</span>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      disabled={busy}
                      title={t("fps_auto_match_tip")}
                      onClick={() => update(s.id, { fps: matchedFPS })}
                    >
                      <TimerReset size={12} />
                      {t("fps_auto_match", { fps: matchedFPS })}
                    </Button>
                  </div>
                  {!s.mirrorOf && (
                    <div className="animation-choreography" onClick={(e) => e.stopPropagation()}>
                      <div className="animation-choreography-head">
                        <strong>{t("animation_choreography_title")}</strong>
                        <span>{isCustom ? t("custom_animation_draft") : t("preset_action_override")}</span>
                      </div>
                      {isCustom && (
                        <div className="mini-field">
                          <label>{t("animation_display_name")}</label>
                          <Input
                            type="text"
                            className="h-7"
                            placeholder={t("custom")}
                            value={s.customLabel ?? ""}
                            onChange={(e) => update(s.id, { customLabel: e.target.value })}
                          />
                        </div>
                      )}
                      <div className="mini-field">
                        <label>{t("action_label")}</label>
                        <Textarea
                          className="animation-action-input"
                          rows={3}
                          placeholder={t("action_ph")}
                          value={s.action}
                          onChange={(e) => update(s.id, { action: e.target.value })}
                        />
                        <span className="hint">{t("action_detail_hint")}</span>
                      </div>
                    </div>
                  )}
                  {!s.mirrorOf && (
                    <div className="mini-field" style={{ marginTop: 6 }} onClick={(e) => e.stopPropagation()}>
                      <label>{t("motion_source_label")}</label>
                      <Select
                        value={mode}
                        disabled={busy}
                        onValueChange={(v) => {
                          const next = v as MotionSourceMode;
                          update(s.id, {
                            motionSourceMode: next,
                            motionSourceId:
                              next === "direct" || next === "guide_image_strip"
                                ? s.motionSourceId || motionLibrary[0]?.id || ""
                                : s.motionSourceId,
                            pipeline:
                              next === "direct" || next === "guide_image_strip"
                                ? "image_strip"
                                : next === "ai_video"
                                  ? "video_to_frames"
                                  : s.pipeline,
                            alignmentMode:
                              next === "direct" || next === "ai_video"
                                ? "preserve_source_canvas"
                                : next === "guide_image_strip"
                                  ? "game_anchor_normalize"
                                  : s.alignmentMode,
                          });
                        }}
                      >
                        <SelectTrigger className="h-7">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="none">{t("motion_source_none")}</SelectItem>
                          <SelectItem value="direct">{t("motion_source_direct")}</SelectItem>
                          <SelectItem value="guide_image_strip">{t("motion_source_guide")}</SelectItem>
                          <SelectItem value="ai_video" disabled={!supportsVideo}>
                            {t("motion_source_ai_video")}{!supportsVideo ? ` · ${t("pipeline_video_unavailable")}` : ""}
                          </SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  )}
                  {!s.mirrorOf && usesLibrary && (
                    <div className="mini-field" style={{ marginTop: 6 }} onClick={(e) => e.stopPropagation()}>
                      <label>{t("motion_library_label")}</label>
                      <Select value={s.motionSourceId || ""} disabled={busy || motionLibrary.length === 0} onValueChange={(v) => update(s.id, { motionSourceId: v })}>
                        <SelectTrigger className="h-7">
                          <SelectValue placeholder={t("motion_library_empty")} />
                        </SelectTrigger>
                        <SelectContent>
                          {motionLibrary.map((item) => (
                            <SelectItem key={item.id} value={item.id}>
                              {item.name} · {item.frames}{t("frames_suffix")} · {item.sourceKind}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      {selectedMotion && (
                        <span className="hint">
                          {selectedMotion.action || selectedMotion.name} · {selectedMotion.facing || t("facing_none")} · {selectedMotion.loop ? t("loop") : t("once")}
                        </span>
                      )}
                      <div className="row">
                        <Button size="sm" variant="outline" disabled={busy} onClick={() => onImportMotionLibrary("file", s.id)}>
                          {t("motion_import_file")}
                        </Button>
                        <Button size="sm" variant="outline" disabled={busy} onClick={() => onImportMotionLibrary("folder", s.id)}>
                          {t("motion_import_folder")}
                        </Button>
                      </div>
                    </div>
                  )}
                  {!s.mirrorOf && (
                    <div className="mini-field" style={{ marginTop: 6 }} onClick={(e) => e.stopPropagation()}>
                      <label>{t("pipeline_card_label")}</label>
                      <Select
                        value={s.pipeline ?? defaultPipeline}
                        onValueChange={(v) => update(s.id, { pipeline: v as StateDef["pipeline"] })}
                        disabled={busy || mode === "direct" || mode === "guide_image_strip" || mode === "ai_video"}
                      >
                        <SelectTrigger className="h-7">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="video_to_frames" disabled={!supportsVideo}>
                            {t("pipeline_video_short")}{!supportsVideo ? ` · ${t("pipeline_video_unavailable")}` : ""}
                          </SelectItem>
                          <SelectItem value="image_strip">{t("pipeline_strip_short")}</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  )}
                  {!s.mirrorOf && (mode === "ai_video" || (mode === "none" && (s.pipeline ?? defaultPipeline) === "video_to_frames")) && (
                    <div className="mini-field" style={{ marginTop: 6 }} onClick={(e) => e.stopPropagation()}>
                      <label>{t("video_reference_mode_label")}</label>
                      <Select value={s.videoReferenceMode ?? "identity_reference"} disabled={busy} onValueChange={(v) => update(s.id, { videoReferenceMode: v as VideoReferenceMode })}>
                        <SelectTrigger className="h-7">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="auto">{t("video_reference_auto")}</SelectItem>
                          <SelectItem value="none">{t("video_reference_none")}</SelectItem>
                          <SelectItem value="identity_reference">{t("video_reference_identity")}</SelectItem>
                          <SelectItem value="first_frame">{t("video_reference_first_frame")}</SelectItem>
                          <SelectItem value="start_end_frame">{t("video_reference_start_end")}</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  )}
                  {!s.mirrorOf && usesVideoPipeline(s) && (
                    <div className="mini-field" style={{ marginTop: 6 }} onClick={(e) => e.stopPropagation()}>
                      <label>{t("video_subject_mode_label")}</label>
                      <Select
                        value={s.videoSubjectMode ?? "include_description"}
                        disabled={busy}
                        onValueChange={(value) => {
                          const next = value as VideoSubjectMode;
                          update(s.id, {
                            videoSubjectMode: next,
                            videoReferenceMode:
                              next === "reference_only" && s.videoReferenceMode === "none"
                                ? "identity_reference"
                                : s.videoReferenceMode,
                          });
                        }}
                      >
                        <SelectTrigger className="h-7">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="auto">{t("video_subject_auto")}</SelectItem>
                          <SelectItem value="include_description">{t("video_subject_full")}</SelectItem>
                          <SelectItem value="reference_only">{t("video_subject_reference_only")}</SelectItem>
                        </SelectContent>
                      </Select>
                      {needsIPReview && (
                        <div className="state-ip-risk">
                          <ShieldAlert size={12} />
                          <span>{t("ip_review_required", { terms: ipRiskMatches.map((match) => match.term).join("、") })}</span>
                        </div>
                      )}
                      {(s.videoSubjectMode ?? "include_description") === "reference_only" && (
                        <span className="hint">{t("video_subject_reference_only_hint")}</span>
                      )}
                    </div>
                  )}
                  {!s.mirrorOf && usesVideoPipeline(s) && (
                    <div className="video-prompt-options" onClick={(e) => e.stopPropagation()}>
                      <div className="mini-field">
                        <label>{t("video_prompt_language_label")}</label>
                        <Select
                          value={s.videoPromptLanguage ?? "auto"}
                          disabled={busy}
                          onValueChange={(value) => update(s.id, { videoPromptLanguage: value as VideoPromptLanguage })}
                        >
                          <SelectTrigger className="h-7">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="auto">{t("video_prompt_language_auto")}</SelectItem>
                            <SelectItem value="zh">{t("video_prompt_language_zh")}</SelectItem>
                            <SelectItem value="en">{t("video_prompt_language_en")}</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                      <div className="mini-field">
                        <label>{t("video_shadow_mode_label")}</label>
                        <Select
                          value={s.videoShadowMode ?? "avoid"}
                          disabled={busy}
                          onValueChange={(value) => update(s.id, { videoShadowMode: value as VideoShadowMode })}
                        >
                          <SelectTrigger className="h-7">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="avoid">{t("video_shadow_avoid")}</SelectItem>
                            <SelectItem value="allow_contact">{t("video_shadow_allow_contact")}</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                    </div>
                  )}
                  {!s.mirrorOf && (mode === "ai_video" || (mode === "none" && (s.pipeline ?? defaultPipeline) === "video_to_frames")) && (
                    <div className="mini-field" style={{ marginTop: 6 }} onClick={(e) => e.stopPropagation()}>
                      <label>{t("frame_sampler_label")}</label>
                      <Select value={s.frameSamplerMode ?? "candidate_pool"} disabled={busy} onValueChange={(v) => update(s.id, { frameSamplerMode: v as FrameSamplerMode })}>
                        <SelectTrigger className="h-7">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="candidate_pool">{t("frame_sampler_candidate_pool")}</SelectItem>
                          <SelectItem value="uniform">{t("frame_sampler_uniform")}</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  )}
                  {!s.mirrorOf && (
                    <div className="mini-field" style={{ marginTop: 6 }} onClick={(e) => e.stopPropagation()}>
                      <label>{t("alignment_mode_label")}</label>
                      <Select value={s.alignmentMode ?? (mode === "direct" || (s.pipeline ?? defaultPipeline) === "video_to_frames" ? "preserve_source_canvas" : "game_anchor_normalize")} disabled={busy} onValueChange={(v) => update(s.id, { alignmentMode: v as AlignmentMode })}>
                        <SelectTrigger className="h-7">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="preserve_source_canvas">{t("alignment_preserve_source_canvas")}</SelectItem>
                          <SelectItem value="game_anchor_normalize">{t("alignment_game_anchor_normalize")}</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  )}
                  {!s.mirrorOf && (
                    <div className="mini-field" style={{ marginTop: 6 }} onClick={(e) => e.stopPropagation()}>
                      <label>{t("frame_fit_mode")}</label>
                      <Select value={s.frameFitMode ?? defaultFrameFitMode} disabled={busy} onValueChange={(v) => update(s.id, { frameFitMode: v as FrameFitMode })}>
                        <SelectTrigger className="h-7">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="contain">{t("frame_fit_contain")}</SelectItem>
                          <SelectItem value="auto_expand">{t("frame_fit_auto_expand")}</SelectItem>
                          <SelectItem value="crop">{t("frame_fit_crop")}</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  )}
                  {!s.dirBase && (
                    <div className="mini-field" style={{ marginTop: 6 }} onClick={(e) => e.stopPropagation()}>
                      <label>{t("facing_label")}</label>
                      <Select
                        value={s.facing ?? "none"}
                        onValueChange={(v) => update(s.id, { facing: v === "none" ? undefined : v })}
                      >
                        <SelectTrigger className="h-7">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="none">{t("facing_none")}</SelectItem>
                          {aiDirections.map((d) => (
                            <SelectItem key={d.key} value={d.key}>
                              {directionName(d.key, lang, d.label)} ({d.short})
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  )}
                  <div className="row" style={{ marginTop: 8 }} onClick={(e) => e.stopPropagation()}>
                    <Button
                      size="sm"
                      className="flex-1"
                      disabled={!canGenerateThis || busy}
                      onClick={() => onGenerate(s.id)}
                    >
                      {s.status === "generating" ? (
                        <>
                          <Loader2 size={12} className="animate-spin" /> {t("generating")}
                        </>
                      ) : s.items.length > 0 ? (
                        t("regenerate")
                      ) : (
                        <>
                          <Sparkles size={12} /> {t("generate")}
                        </>
                      )}
                    </Button>
                    {!s.mirrorOf && (
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={!canGenerate || busy || needsIPReview}
                        title={t("dir8_tip")}
                        onClick={() => onGenerateDirectionSet(s.id)}
                      >
                        <Compass size={12} /> {t("dir8")}
                      </Button>
                    )}
                  </div>
                </>
              )}

              {(s.status === "done" || s.status === "review") && s.items.length > 0 && (
                <div className="state-thumbs checker">
                  {s.items.slice(0, 8).map((f) => (
                    <img key={f.id} src={f.png} className="pixelated" alt="" style={{ opacity: f.selected ? 1 : 0.3 }} />
                  ))}
                </div>
              )}

              {s.status === "generating" && s.progress && (
                <div className="state-progress-wrap">
                  <div className="state-progress-label">
                    <span>{s.progress.message}</span>
                    {s.progress.percent !== undefined && <strong>{s.progress.percent}%</strong>}
                  </div>
                  <div className="state-progress" aria-hidden="true">
                    <span style={{ width: `${s.progress.percent ?? 12}%` }} />
                  </div>
                </div>
              )}

              {s.status === "error" && s.error && <div className="state-error">{localizeBackendText(s.error, lang)}</div>}
              {s.status === "review" && s.qualityReview && <div className="state-review">{t("quality_review_card")}</div>}
              {s.warnings.map((w, i) => (
                <div key={i} className="state-warn">
                  {localizeBackendText(w, lang)}
                </div>
              ))}
            </div>
          );
        })}
      </div>
      </div>

      {states.length > 0 && (
        <div className="panel-foot">
          <Button className="w-full" disabled={!canGenerateAll || busy || pendingCount === 0} onClick={onGenerateAll}>
            {busy ? (
              <>
                <Loader2 size={13} className="animate-spin" /> {t("generating_progress")}
              </>
            ) : (
              <>
                <Zap size={13} /> {t("generate_all_pending", { n: pendingCount })}
              </>
            )}
          </Button>
        </div>
      )}
    </>
  );
}
