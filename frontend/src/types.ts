// 프론트엔드 도메인 타입 정의

export interface FrameItem {
  id: string;
  png: string; // dataURL
  selected: boolean;
  source?: string;
  sourceRef?: string;
  replacement?: FrameReplacement;
}

export interface FrameReplacement {
  candidateId: string;
  source: string;
  label?: string;
  targetFrameIndex: number;
  timeMs?: number;
  quality?: number;
  reason?: string;
  replacedAt: string;
}

export interface FrameScores {
  identity: number;
  motion: number;
  contact: number;
  overall: number;
}

export interface TaskProgress {
  phase: string;
  message: string;
  percent?: number;
  current?: number;
  total?: number;
}

export type ToolchainModule = "animation" | "image" | "video" | "ui";
export type AppModule = "home" | ToolchainModule;
export type StudioMode = "simple" | "expert";
export type ToolAssetKind = "image" | "video" | "audio" | "text" | "json" | "archive";
export type GenerationJobKind = "image" | "video" | "ui_analysis" | "ui_split" | "prompt_optimization";
export type GenerationJobStatus =
  | "queued"
  | "running"
  | "submitting"
  | "polling"
  | "completed"
  | "failed"
  | "cancelled"
  | "stopped"
  | "needs_review";

export interface GenerationJob {
  id: string;
  kind: GenerationJobKind;
  status: GenerationJobStatus;
  phase?: string;
  progress: number;
  provider?: string;
  model?: string;
  providerTaskId?: string;
  inputAssetIds?: string[];
  outputAssetIds?: string[];
  error?: string;
  request?: Record<string, unknown>;
  result?: Record<string, unknown> | string;
  providerResponseSummary?: Record<string, unknown> | string;
  sourceModule?: ToolchainModule;
  retryOfJobId?: string;
  attempt?: number;
  upstreamMayContinue?: boolean;
  createdAt: string;
  updatedAt: string;
  startedAt?: string;
  completedAt?: string;
}

export interface ToolAsset {
  id: string;
  kind?: ToolAssetKind;
  mediaType?: ToolAssetKind;
  role?: string;
  usage?: string;
  name: string;
  mime?: string;
  mimeType?: string;
  sizeBytes?: number;
  width?: number;
  height?: number;
  durationMs?: number;
  thumbnailUrl?: string;
  contentUrl?: string;
  thumbnailAvailable?: boolean;
  originModule?: ToolchainModule;
  sourceModule?: ToolchainModule;
  parentIds?: string[];
  parentAssetId?: string;
  prompt?: string;
  provider?: string;
  model?: string;
  tags?: string[];
  legacyIndexed?: boolean;
  createdAt: string;
  updatedAt?: string;
}

export interface UIPixelBounds {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface UIRegion {
  id: string;
  parentId?: string;
  name: string;
  type: string;
  bounds: UIPixelBounds;
  zIndex: number;
  confidence: number;
  text?: string;
  style?: Record<string, unknown>;
  exportEnabled: boolean;
}

export interface ModelCapability {
  provider: string;
  model: string;
  label?: string;
  supportsText?: boolean;
  supportsImageReference?: boolean;
  supportsMultipleReferences?: boolean;
  maxReferenceImages?: number;
  supportsEdit?: boolean;
  supportsMask?: boolean;
  supportsFirstFrame?: boolean;
  supportsLastFrame?: boolean;
  supportsIdentityReference?: boolean;
  supportsAudio?: boolean;
  supportsWatermark?: boolean;
  aspectRatios?: string[];
  resolutions?: string[];
  durations?: number[];
}

export interface GenerationCapabilities {
  image?: ModelCapability[];
  video?: ModelCapability[];
  vision?: ModelCapability[];
  text?: ModelCapability[];
  imageModels?: ModelCapability[];
  videoModels?: ModelCapability[];
  visionModels?: ModelCapability[];
  textModels?: ModelCapability[];
}

export type StateStatus = "idle" | "generating" | "review" | "done" | "error";
export type AnimationPipeline = "video_to_frames" | "image_strip";
export type MotionSourceMode = "none" | "direct" | "guide_image_strip" | "ai_video";
export type VideoReferenceMode = "auto" | "none" | "identity_reference" | "first_frame" | "start_end_frame";
export type VideoSubjectMode = "auto" | "include_description" | "reference_only";
export type VideoPromptLanguage = "auto" | "zh" | "en";
export type VideoShadowMode = "avoid" | "allow_contact";
export type FrameSamplerMode = "candidate_pool" | "uniform";
export type AlignmentMode = "preserve_source_canvas" | "game_anchor_normalize";
export type FrameFitMode = "contain" | "auto_expand" | "crop";

export interface GenerationReference {
  kind: string;
  label: string;
  source: string;
  id?: string;
  path?: string;
  index?: number;
  preview?: string;
}

export interface GenerationAttempt {
  index: number;
  pipeline: string;
  provider?: string;
  model?: string;
  prompt: string;
  references?: GenerationReference[];
  parameters?: Record<string, string>;
}

export interface GenerationMediaSlotTrace {
  slot: string;
  field: string;
  role?: string;
  count: number;
}

export interface GenerationTrace {
  pipeline: string;
  provider?: string;
  model?: string;
  videoModel?: string;
  motionSourceMode: MotionSourceMode | string;
  motionSourceId?: string;
  motionSourceName?: string;
  videoReferenceMode?: VideoReferenceMode | string;
  videoSubjectMode?: VideoSubjectMode | string;
  actualVideoReferenceMode?: VideoReferenceMode | string;
  videoReferenceField?: string;
  videoWorkflow?: string;
  videoRequestRoute?: string;
  videoMediaSlots?: GenerationMediaSlotTrace[];
  frameSamplerMode?: FrameSamplerMode | string;
  alignmentMode?: AlignmentMode | string;
  prompt?: string;
  calledAi: boolean;
  parameters?: Record<string, string>;
  references?: GenerationReference[];
  attempts?: GenerationAttempt[];
  selections?: FrameSelectionTrace[];
  startedAt?: string;
  completedAt?: string;
}

export interface CandidateFrame {
  id: string;
  index: number;
  png?: string;
  source: string;
  label: string;
  timeMs?: number;
  sourcePath?: string;
  quality?: number;
  motion?: number;
  similarity?: number;
  continuity?: number;
  nearDuplicate?: boolean;
  fingerprint?: string;
  reason?: string;
  selected?: boolean;
}

export interface FrameSequenceMetrics {
  duplicateRate: number;
  continuity: number;
  motionCoverage: number;
  loopClosure?: number;
  activeStartMs?: number;
  activeEndMs?: number;
}

export interface FrameSelectionTrace {
  targetFrameIndex: number;
  candidateId: string;
  source: string;
  timeMs?: number;
  quality?: number;
  reason?: string;
  selectedAt?: string;
}

export interface QualityReview {
  summary: string;
  ignored: boolean;
  source: "generation" | "reprocess" | "session";
  ignoredAt?: string;
}

export interface MotionLibraryItem {
  id: string;
  name: string;
  action: string;
  facing?: string;
  frames: number;
  fps: number;
  loop: boolean;
  assetUse?: string;
  sourceKind: string;
  sourcePath?: string;
  sourceFps?: number;
  sourceDurationMs?: number;
  rawFrames?: string[];
  processedFrames?: string[];
  candidateFrames?: CandidateFrame[];
  strip?: string;
  thumbnail?: string;
  tags?: string[];
  directUsable: boolean;
  referenceOnly: boolean;
  alignmentMode?: AlignmentMode | string;
  createdAt: string;
  updatedAt: string;
}

export interface StateDef {
  id: string;
  name: string; // 영문 상태명 (export용)
  label: string; // 한글 표시명
  isCustom?: boolean;
  customLabel?: string;
  frames: number;
  fps: number;
  loop: boolean;
  action: string;
  status: StateStatus;
  error?: string;
  qualityReview?: QualityReview;
  progress?: TaskProgress;
  pipeline?: AnimationPipeline;
  lastPipeline?: AnimationPipeline;
  motionSourceId?: string;
  motionSourceMode?: MotionSourceMode;
  videoReferenceMode?: VideoReferenceMode;
  videoSubjectMode?: VideoSubjectMode;
  videoPromptLanguage?: VideoPromptLanguage;
  videoShadowMode?: VideoShadowMode;
  frameSamplerMode?: FrameSamplerMode;
  alignmentMode?: AlignmentMode;
  frameFitMode?: FrameFitMode;
  cellSize?: number;
  rawStrip?: string;
  rawVideoPath?: string;
  rawExtractedFrames?: string[];
  trace?: GenerationTrace;
  candidateFrames?: CandidateFrame[];
  sequenceMetrics?: FrameSequenceMetrics;
  items: FrameItem[];
  warnings: string[];
  feedback: string;
  scores?: FrameScores;
  facing?: string; // 8방향 키 (south 등, 미지정 시 방향 지시 없음)
  dirBase?: string; // 8방향 세트의 베이스 상태명 (세트 소속일 때만)
  mirrorOf?: string; // 미러링 소스 방향 키 (west→east 등, AI 생성 안 함)
}

// 백엔드 sprite.DirectionInfo와 동일 구조 (ListDirections 응답)
export interface DirectionInfo {
  key: string;
  label: string;
  short: string;
  mirrorOf: string;
  row: number;
  col: number;
}

// 방향 키 → 한글 라벨 변환 (목록에 없으면 키 그대로)
export function directionLabel(directions: DirectionInfo[], key: string | undefined): string {
  if (!key) return "";
  return directions.find((d) => d.key === key)?.label ?? key;
}

export interface CharacterDef {
  libraryId?: string;
  image: string | null; // dataURL
  name: string; // 내보내기 파일 프리픽스로 사용
  description: string;
  styleKey: string;
  styleCustom: string;
}

export interface CharacterLibraryItem {
  id: string;
  name: string;
  description?: string;
  styleKey: string;
  styleCustom?: string;
  imagePath?: string;
  image?: string;
  createdAt: string;
  updatedAt: string;
}

export interface CharacterWorkspace {
  characterId: string;
  character: CharacterDef;
  cellSize: number;
  frameFitMode: FrameFitMode;
  atlasColumns: number;
  states: StateDef[];
  selectedId: string | null;
}

export interface StatePreset {
  name: string;
  label: string;
  frames: number;
  fps: number;
  loop: boolean;
  action: string;
}

// 백엔드 sprite.PresetInfo와 동일 구조 (ListPresets 응답)
export interface PresetInfo {
  name: string;
  label: string;
  category: string;
  action: string;
  frames: number;
  fps: number;
  loop: boolean;
}

export const STATE_PRESETS: StatePreset[] = [
  { name: "idle", label: "待机", frames: 4, fps: 6, loop: true, action: "subtle breathing idle in place" },
  { name: "walk", label: "行走", frames: 6, fps: 10, loop: true, action: "side-view walking cycle facing right" },
  { name: "run", label: "奔跑", frames: 6, fps: 12, loop: true, action: "fast side-view running cycle facing right" },
  { name: "jump", label: "跳跃", frames: 5, fps: 10, loop: false, action: "crouch, take off, airborne peak, land" },
  { name: "attack", label: "攻击", frames: 5, fps: 12, loop: false, action: "melee attack with wind-up, strike, recovery" },
  { name: "hurt", label: "受击", frames: 3, fps: 10, loop: false, action: "recoil from being hit" },
  { name: "death", label: "死亡", frames: 5, fps: 8, loop: false, action: "stagger, collapse, lie flat on the ground" },
  { name: "wave", label: "挥手", frames: 4, fps: 8, loop: true, action: "friendly hand wave, body still" },
];

export const STYLE_OPTIONS = [
  { key: "pixel", label: "像素画" },
  { key: "chibi", label: "Q版" },
  { key: "cartoon", label: "卡通" },
  { key: "retro16", label: "16 位复古" },
  { key: "custom", label: "自定义" },
];

export const CELL_SIZES = [128, 256, 512];

let seq = 0;
export function uid(prefix: string): string {
  seq += 1;
  return `${prefix}-${Date.now().toString(36)}-${seq}`;
}

export function presetToState(p: StatePreset): StateDef {
  return {
    id: uid("st"),
    name: p.name,
    label: p.label,
    frames: p.frames,
    fps: p.fps,
    loop: p.loop,
    action: p.action,
    status: "idle",
    items: [],
    warnings: [],
    feedback: "",
  };
}

// ListPresets 카탈로그 항목을 애니메이션 상태로 변환합니다.
export function presetInfoToState(p: PresetInfo): StateDef {
  return {
    id: uid("st"),
    name: p.name,
    label: p.label,
    frames: p.frames,
    fps: p.fps,
    loop: p.loop,
    action: p.action,
    status: "idle",
    items: [],
    warnings: [],
    feedback: "",
  };
}

// STATE_PRESETS를 PresetInfo 형태로 변환한 폴백 카탈로그 (ListPresets 실패 시 사용)
export const FALLBACK_PRESETS: PresetInfo[] = STATE_PRESETS.map((p) => ({
  ...p,
  category: "基础动作",
}));

export function selectedFrames(s: StateDef): FrameItem[] {
  return s.items.filter((f) => f.selected);
}
