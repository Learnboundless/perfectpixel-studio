export namespace gen {
  export class VideoModelCapability {
    model: string;
    family: string;
    endpoint: string;
    providerAdapter?: string;
    referenceSemantics: string;
    tags: string[];
    workflowModes?: string[];
    mediaSlots?: string[];
    supportsTextPrompt: boolean;
    supportsReferenceImage: boolean;
    supportsIdentityReference: boolean;
    supportsFirstFrameReference: boolean;
    supportsImageToVideo: boolean;
    supportsStartEndFrames: boolean;
    supportsReferenceVideo: boolean;
    supportsReferenceAudio: boolean;
    supportsMultiImageReference: boolean;
    supportsMotionTransfer: boolean;
    supportsMetadata: boolean;
    supportsAudio: boolean;
    supportsWatermarkControl: boolean;
    allowedSeconds?: number[];
    allowedResolutions?: string[];
    allowedAspectRatios?: string[];

    static createFrom(source: any = {}) {
      return new VideoModelCapability(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.model = source["model"];
      this.family = source["family"];
      this.endpoint = source["endpoint"];
      this.providerAdapter = source["providerAdapter"];
      this.referenceSemantics = source["referenceSemantics"];
      this.tags = source["tags"];
      this.workflowModes = source["workflowModes"];
      this.mediaSlots = source["mediaSlots"];
      this.supportsTextPrompt = source["supportsTextPrompt"];
      this.supportsReferenceImage = source["supportsReferenceImage"];
      this.supportsIdentityReference = source["supportsIdentityReference"];
      this.supportsFirstFrameReference = source["supportsFirstFrameReference"];
      this.supportsImageToVideo = source["supportsImageToVideo"];
      this.supportsStartEndFrames = source["supportsStartEndFrames"];
      this.supportsReferenceVideo = source["supportsReferenceVideo"];
      this.supportsReferenceAudio = source["supportsReferenceAudio"];
      this.supportsMultiImageReference = source["supportsMultiImageReference"];
      this.supportsMotionTransfer = source["supportsMotionTransfer"];
      this.supportsMetadata = source["supportsMetadata"];
      this.supportsAudio = source["supportsAudio"];
      this.supportsWatermarkControl = source["supportsWatermarkControl"];
      this.allowedSeconds = source["allowedSeconds"];
      this.allowedResolutions = source["allowedResolutions"];
      this.allowedAspectRatios = source["allowedAspectRatios"];
    }
  }
  export class VideoModelShowcase {
    model: string;
    sampleUrl: string;
    workflow?: string;
    testedAt?: string;
    bytes?: number;
    source?: string;

    static createFrom(source: any = {}) {
      return new VideoModelShowcase(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.model = source["model"];
      this.sampleUrl = source["sampleUrl"];
      this.workflow = source["workflow"];
      this.testedAt = source["testedAt"];
      this.bytes = source["bytes"];
      this.source = source["source"];
    }
  }
}

export namespace main {
  export class AnimationFrameReplacement {
    candidateId: string;
    source: string;
    label?: string;
    targetFrameIndex: number;
    timeMs?: number;
    quality?: number;
    reason?: string;
    replacedAt: string;

    static createFrom(source: any = {}) {
      return new AnimationFrameReplacement(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.candidateId = source["candidateId"];
      this.source = source["source"];
      this.label = source["label"];
      this.targetFrameIndex = source["targetFrameIndex"];
      this.timeMs = source["timeMs"];
      this.quality = source["quality"];
      this.reason = source["reason"];
      this.replacedAt = source["replacedAt"];
    }
  }
  export class FrameSelectionTrace {
    targetFrameIndex: number;
    candidateId: string;
    source: string;
    timeMs?: number;
    quality?: number;
    reason?: string;
    selectedAt?: string;

    static createFrom(source: any = {}) {
      return new FrameSelectionTrace(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.targetFrameIndex = source["targetFrameIndex"];
      this.candidateId = source["candidateId"];
      this.source = source["source"];
      this.timeMs = source["timeMs"];
      this.quality = source["quality"];
      this.reason = source["reason"];
      this.selectedAt = source["selectedAt"];
    }
  }
  export class GenerationAttempt {
    index: number;
    pipeline: string;
    provider?: string;
    model?: string;
    prompt: string;
    references?: GenerationReference[];
    parameters?: Record<string, string>;

    static createFrom(source: any = {}) {
      return new GenerationAttempt(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.index = source["index"];
      this.pipeline = source["pipeline"];
      this.provider = source["provider"];
      this.model = source["model"];
      this.prompt = source["prompt"];
      this.references = this.convertValues(
        source["references"],
        GenerationReference,
      );
      this.parameters = source["parameters"];
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }
  export class GenerationReference {
    kind: string;
    label: string;
    source: string;
    id?: string;
    path?: string;
    index?: number;
    preview?: string;

    static createFrom(source: any = {}) {
      return new GenerationReference(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.kind = source["kind"];
      this.label = source["label"];
      this.source = source["source"];
      this.id = source["id"];
      this.path = source["path"];
      this.index = source["index"];
      this.preview = source["preview"];
    }
  }
  export class GenerationMediaSlotTrace {
    slot: string;
    field: string;
    role?: string;
    count: number;

    static createFrom(source: any = {}) {
      return new GenerationMediaSlotTrace(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.slot = source["slot"];
      this.field = source["field"];
      this.role = source["role"];
      this.count = source["count"];
    }
  }
  export class GenerationTrace {
    pipeline: string;
    provider?: string;
    imageProvider?: string;
    videoProvider?: string;
    model?: string;
    videoModel?: string;
    motionSourceMode: string;
    motionSourceId?: string;
    motionSourceName?: string;
    videoReferenceMode?: string;
    videoSubjectMode?: string;
    actualVideoReferenceMode?: string;
    videoReferenceField?: string;
    videoWorkflow?: string;
    videoRequestRoute?: string;
    videoMediaSlots?: GenerationMediaSlotTrace[];
    frameSamplerMode?: string;
    alignmentMode?: string;
    backgroundRemovalMode?: string;
    backgroundRemovalModel?: string;
    backgroundRemovalSha256?: string;
    backgroundRemovalMs?: number;
    backgroundRemovalFallback?: string;
    prompt?: string;
    calledAi: boolean;
    parameters?: Record<string, string>;
    references?: GenerationReference[];
    attempts?: GenerationAttempt[];
    selections?: FrameSelectionTrace[];
    startedAt?: string;
    completedAt?: string;

    static createFrom(source: any = {}) {
      return new GenerationTrace(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.pipeline = source["pipeline"];
      this.provider = source["provider"];
      this.imageProvider = source["imageProvider"];
      this.videoProvider = source["videoProvider"];
      this.model = source["model"];
      this.videoModel = source["videoModel"];
      this.motionSourceMode = source["motionSourceMode"];
      this.motionSourceId = source["motionSourceId"];
      this.motionSourceName = source["motionSourceName"];
      this.videoReferenceMode = source["videoReferenceMode"];
      this.videoSubjectMode = source["videoSubjectMode"];
      this.actualVideoReferenceMode = source["actualVideoReferenceMode"];
      this.videoReferenceField = source["videoReferenceField"];
      this.videoWorkflow = source["videoWorkflow"];
      this.videoRequestRoute = source["videoRequestRoute"];
      this.videoMediaSlots = this.convertValues(
        source["videoMediaSlots"],
        GenerationMediaSlotTrace,
      );
      this.frameSamplerMode = source["frameSamplerMode"];
      this.alignmentMode = source["alignmentMode"];
      this.backgroundRemovalMode = source["backgroundRemovalMode"];
      this.backgroundRemovalModel = source["backgroundRemovalModel"];
      this.backgroundRemovalSha256 = source["backgroundRemovalSha256"];
      this.backgroundRemovalMs = source["backgroundRemovalMs"];
      this.backgroundRemovalFallback = source["backgroundRemovalFallback"];
      this.prompt = source["prompt"];
      this.calledAi = source["calledAi"];
      this.parameters = source["parameters"];
      this.references = this.convertValues(
        source["references"],
        GenerationReference,
      );
      this.attempts = this.convertValues(source["attempts"], GenerationAttempt);
      this.selections = this.convertValues(
        source["selections"],
        FrameSelectionTrace,
      );
      this.startedAt = source["startedAt"];
      this.completedAt = source["completedAt"];
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }
  export class CandidateFrame {
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

    static createFrom(source: any = {}) {
      return new CandidateFrame(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.id = source["id"];
      this.index = source["index"];
      this.png = source["png"];
      this.source = source["source"];
      this.label = source["label"];
      this.timeMs = source["timeMs"];
      this.sourcePath = source["sourcePath"];
      this.quality = source["quality"];
      this.motion = source["motion"];
      this.similarity = source["similarity"];
      this.continuity = source["continuity"];
      this.nearDuplicate = source["nearDuplicate"];
      this.fingerprint = source["fingerprint"];
      this.reason = source["reason"];
      this.selected = source["selected"];
    }
  }
  export class AnimationVersionFrame {
    id: string;
    png?: string;
    path?: string;
    selected: boolean;
    source?: string;
    sourceRef?: string;
    replacement?: AnimationFrameReplacement;

    static createFrom(source: any = {}) {
      return new AnimationVersionFrame(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.id = source["id"];
      this.png = source["png"];
      this.path = source["path"];
      this.selected = source["selected"];
      this.source = source["source"];
      this.sourceRef = source["sourceRef"];
      this.replacement = this.convertValues(
        source["replacement"],
        AnimationFrameReplacement,
      );
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }
  export class AnimationVersion {
    id: string;
    characterId: string;
    stateId: string;
    stateName: string;
    versionNumber: number;
    status: string;
    source: string;
    parentVersionId?: string;
    frameCount: number;
    overallScore?: number;
    backgroundRemovalMode?: string;
    thumbnail?: string;
    createdAt: string;
    deletedAt?: string;
    sizeBytes: number;
    partialRecovery?: boolean;
    current: boolean;
    baselineFrames: AnimationVersionFrame[];
    workingComposition: AnimationVersionFrame[];
    rawStrip?: string;
    rawStripPath?: string;
    rawVideoPath?: string;
    rawExtractedFrames?: string[];
    candidateFrames?: CandidateFrame[];
    warnings?: string[];
    feedback?: string;
    trace: GenerationTrace;
    scores?: Record<string, number>;
    sequenceMetrics?: Record<string, number>;
    generationParameters?: Record<string, any>;
    characterSnapshot?: Record<string, any>;
    stateSnapshot?: Record<string, any>;
    sourceKey?: string;

    static createFrom(source: any = {}) {
      return new AnimationVersion(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.id = source["id"];
      this.characterId = source["characterId"];
      this.stateId = source["stateId"];
      this.stateName = source["stateName"];
      this.versionNumber = source["versionNumber"];
      this.status = source["status"];
      this.source = source["source"];
      this.parentVersionId = source["parentVersionId"];
      this.frameCount = source["frameCount"];
      this.overallScore = source["overallScore"];
      this.backgroundRemovalMode = source["backgroundRemovalMode"];
      this.thumbnail = source["thumbnail"];
      this.createdAt = source["createdAt"];
      this.deletedAt = source["deletedAt"];
      this.sizeBytes = source["sizeBytes"];
      this.partialRecovery = source["partialRecovery"];
      this.current = source["current"];
      this.baselineFrames = this.convertValues(
        source["baselineFrames"],
        AnimationVersionFrame,
      );
      this.workingComposition = this.convertValues(
        source["workingComposition"],
        AnimationVersionFrame,
      );
      this.rawStrip = source["rawStrip"];
      this.rawStripPath = source["rawStripPath"];
      this.rawVideoPath = source["rawVideoPath"];
      this.rawExtractedFrames = source["rawExtractedFrames"];
      this.candidateFrames = this.convertValues(
        source["candidateFrames"],
        CandidateFrame,
      );
      this.warnings = source["warnings"];
      this.feedback = source["feedback"];
      this.trace = this.convertValues(source["trace"], GenerationTrace);
      this.scores = source["scores"];
      this.sequenceMetrics = source["sequenceMetrics"];
      this.generationParameters = source["generationParameters"];
      this.characterSnapshot = source["characterSnapshot"];
      this.stateSnapshot = source["stateSnapshot"];
      this.sourceKey = source["sourceKey"];
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }

  export class AnimationVersionStorageStats {
    versionCount: number;
    trashCount: number;
    versionBytes: number;
    trashBytes: number;
    totalBytes: number;
    freeBytes: number;
    warningBytes: number;
    warning?: string;

    static createFrom(source: any = {}) {
      return new AnimationVersionStorageStats(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.versionCount = source["versionCount"];
      this.trashCount = source["trashCount"];
      this.versionBytes = source["versionBytes"];
      this.trashBytes = source["trashBytes"];
      this.totalBytes = source["totalBytes"];
      this.freeBytes = source["freeBytes"];
      this.warningBytes = source["warningBytes"];
      this.warning = source["warning"];
    }
  }
  export class AnimationVersionSummary {
    id: string;
    characterId: string;
    stateId: string;
    stateName: string;
    versionNumber: number;
    status: string;
    source: string;
    parentVersionId?: string;
    frameCount: number;
    overallScore?: number;
    backgroundRemovalMode?: string;
    thumbnail?: string;
    createdAt: string;
    deletedAt?: string;
    sizeBytes: number;
    partialRecovery?: boolean;
    current: boolean;

    static createFrom(source: any = {}) {
      return new AnimationVersionSummary(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.id = source["id"];
      this.characterId = source["characterId"];
      this.stateId = source["stateId"];
      this.stateName = source["stateName"];
      this.versionNumber = source["versionNumber"];
      this.status = source["status"];
      this.source = source["source"];
      this.parentVersionId = source["parentVersionId"];
      this.frameCount = source["frameCount"];
      this.overallScore = source["overallScore"];
      this.backgroundRemovalMode = source["backgroundRemovalMode"];
      this.thumbnail = source["thumbnail"];
      this.createdAt = source["createdAt"];
      this.deletedAt = source["deletedAt"];
      this.sizeBytes = source["sizeBytes"];
      this.partialRecovery = source["partialRecovery"];
      this.current = source["current"];
    }
  }

  export class CharacterLibraryItem {
    id: string;
    name: string;
    description?: string;
    styleKey: string;
    styleCustom?: string;
    imagePath?: string;
    image?: string;
    createdAt: string;
    updatedAt: string;

    static createFrom(source: any = {}) {
      return new CharacterLibraryItem(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.id = source["id"];
      this.name = source["name"];
      this.description = source["description"];
      this.styleKey = source["styleKey"];
      this.styleCustom = source["styleCustom"];
      this.imagePath = source["imagePath"];
      this.image = source["image"];
      this.createdAt = source["createdAt"];
      this.updatedAt = source["updatedAt"];
    }
  }
  export class CommitAnimationVersionArgs {
    characterId: string;
    stateId: string;
    stateName: string;
    status: string;
    source: string;
    sourceKey?: string;
    parentVersionId?: string;
    backgroundRemovalMode?: string;
    frames: AnimationVersionFrame[];
    rawStrip?: string;
    rawVideoPath?: string;
    rawExtractedFrames?: string[];
    candidateFrames?: CandidateFrame[];
    warnings?: string[];
    feedback?: string;
    trace: GenerationTrace;
    scores?: Record<string, number>;
    sequenceMetrics?: Record<string, number>;
    generationParameters?: Record<string, any>;
    characterSnapshot?: Record<string, any>;
    stateSnapshot?: Record<string, any>;
    partialRecovery?: boolean;
    createdAt?: string;

    static createFrom(source: any = {}) {
      return new CommitAnimationVersionArgs(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.characterId = source["characterId"];
      this.stateId = source["stateId"];
      this.stateName = source["stateName"];
      this.status = source["status"];
      this.source = source["source"];
      this.sourceKey = source["sourceKey"];
      this.parentVersionId = source["parentVersionId"];
      this.backgroundRemovalMode = source["backgroundRemovalMode"];
      this.frames = this.convertValues(source["frames"], AnimationVersionFrame);
      this.rawStrip = source["rawStrip"];
      this.rawVideoPath = source["rawVideoPath"];
      this.rawExtractedFrames = source["rawExtractedFrames"];
      this.candidateFrames = this.convertValues(
        source["candidateFrames"],
        CandidateFrame,
      );
      this.warnings = source["warnings"];
      this.feedback = source["feedback"];
      this.trace = this.convertValues(source["trace"], GenerationTrace);
      this.scores = source["scores"];
      this.sequenceMetrics = source["sequenceMetrics"];
      this.generationParameters = source["generationParameters"];
      this.characterSnapshot = source["characterSnapshot"];
      this.stateSnapshot = source["stateSnapshot"];
      this.partialRecovery = source["partialRecovery"];
      this.createdAt = source["createdAt"];
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }
  export class ExportAnimationArgs {
    character: string;
    name: string;
    fps: number;
    loop: boolean;
    format: string;
    frames: string[];

    static createFrom(source: any = {}) {
      return new ExportAnimationArgs(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.character = source["character"];
      this.name = source["name"];
      this.fps = source["fps"];
      this.loop = source["loop"];
      this.format = source["format"];
      this.frames = source["frames"];
    }
  }
  export class ExportState {
    name: string;
    fps: number;
    loop: boolean;
    frames: string[];

    static createFrom(source: any = {}) {
      return new ExportState(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.name = source["name"];
      this.fps = source["fps"];
      this.loop = source["loop"];
      this.frames = source["frames"];
    }
  }
  export class ExportArgs {
    character: string;
    cellSize: number;
    frameFitMode: string;
    atlasColumns: number;
    states: ExportState[];

    static createFrom(source: any = {}) {
      return new ExportArgs(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.character = source["character"];
      this.cellSize = source["cellSize"];
      this.frameFitMode = source["frameFitMode"];
      this.atlasColumns = source["atlasColumns"];
      this.states = this.convertValues(source["states"], ExportState);
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }

  export class GalleryImage {
    name: string;
    path: string;
    size: number;
    modTime: number;

    static createFrom(source: any = {}) {
      return new GalleryImage(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.name = source["name"];
      this.path = source["path"];
      this.size = source["size"];
      this.modTime = source["modTime"];
    }
  }
  export class GenerateCharacterArgs {
    description: string;
    styleKey: string;
    styleCustom: string;

    static createFrom(source: any = {}) {
      return new GenerateCharacterArgs(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.description = source["description"];
      this.styleKey = source["styleKey"];
      this.styleCustom = source["styleCustom"];
    }
  }
  export class GenerateStateArgs {
    requestId: string;
    stateId: string;
    baseImage: string;
    description: string;
    styleKey: string;
    styleCustom: string;
    cellSize: number;
    safeMargin: number;
    feedback: string;
    refStrip: string;
    pipeline: string;
    videoModel: string;
    videoSeconds: number;
    videoFps: number;
    motionSourceId: string;
    motionSourceMode: string;
    videoReferenceMode: string;
    videoSubjectMode: string;
    videoPromptLanguage: string;
    videoShadowMode: string;
    frameSamplerMode: string;
    alignmentMode: string;
    frameFitMode: string;
    backgroundRemovalMode: string;
    targetFrameIndex: number;
    state: sprite.StateSpec;

    static createFrom(source: any = {}) {
      return new GenerateStateArgs(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.requestId = source["requestId"];
      this.stateId = source["stateId"];
      this.baseImage = source["baseImage"];
      this.description = source["description"];
      this.styleKey = source["styleKey"];
      this.styleCustom = source["styleCustom"];
      this.cellSize = source["cellSize"];
      this.safeMargin = source["safeMargin"];
      this.feedback = source["feedback"];
      this.refStrip = source["refStrip"];
      this.pipeline = source["pipeline"];
      this.videoModel = source["videoModel"];
      this.videoSeconds = source["videoSeconds"];
      this.videoFps = source["videoFps"];
      this.motionSourceId = source["motionSourceId"];
      this.motionSourceMode = source["motionSourceMode"];
      this.videoReferenceMode = source["videoReferenceMode"];
      this.videoSubjectMode = source["videoSubjectMode"];
      this.videoPromptLanguage = source["videoPromptLanguage"];
      this.videoShadowMode = source["videoShadowMode"];
      this.frameSamplerMode = source["frameSamplerMode"];
      this.alignmentMode = source["alignmentMode"];
      this.frameFitMode = source["frameFitMode"];
      this.backgroundRemovalMode = source["backgroundRemovalMode"];
      this.targetFrameIndex = source["targetFrameIndex"];
      this.state = this.convertValues(source["state"], sprite.StateSpec);
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }

  export class GenerationJob {
    id: string;
    kind: string;
    status: string;
    phase?: string;
    progress: number;
    provider?: string;
    model?: string;
    providerTaskId?: string;
    inputAssetIds?: string[];
    outputAssetIds?: string[];
    error?: string;
    request?: Record<string, any>;
    sourceModule?: string;
    retryOfJobId?: string;
    attempt: number;
    upstreamMayContinue?: boolean;
    createdAt: string;
    updatedAt: string;
    startedAt?: string;
    completedAt?: string;
    providerResponseSummary?: number[];

    static createFrom(source: any = {}) {
      return new GenerationJob(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.id = source["id"];
      this.kind = source["kind"];
      this.status = source["status"];
      this.phase = source["phase"];
      this.progress = source["progress"];
      this.provider = source["provider"];
      this.model = source["model"];
      this.providerTaskId = source["providerTaskId"];
      this.inputAssetIds = source["inputAssetIds"];
      this.outputAssetIds = source["outputAssetIds"];
      this.error = source["error"];
      this.request = source["request"];
      this.sourceModule = source["sourceModule"];
      this.retryOfJobId = source["retryOfJobId"];
      this.attempt = source["attempt"];
      this.upstreamMayContinue = source["upstreamMayContinue"];
      this.createdAt = source["createdAt"];
      this.updatedAt = source["updatedAt"];
      this.startedAt = source["startedAt"];
      this.completedAt = source["completedAt"];
      this.providerResponseSummary = source["providerResponseSummary"];
    }
  }

  export class ImportMotionLibraryArgs {
    name: string;
    action: string;
    facing?: string;
    frames: number;
    fps: number;
    loop: boolean;
    cellSize: number;
    seconds: number;
    tags?: string[];
    sourcePath?: string;

    static createFrom(source: any = {}) {
      return new ImportMotionLibraryArgs(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.name = source["name"];
      this.action = source["action"];
      this.facing = source["facing"];
      this.frames = source["frames"];
      this.fps = source["fps"];
      this.loop = source["loop"];
      this.cellSize = source["cellSize"];
      this.seconds = source["seconds"];
      this.tags = source["tags"];
      this.sourcePath = source["sourcePath"];
    }
  }
  export class LegacyAnimationRecoveryItem {
    id: string;
    artifactDir: string;
    stateHint?: string;
    frameCount: number;
    thumbnail?: string;
    reason: string;
    baseReferenceHash?: string;
    contentHash: string;
    sizeBytes: number;
    discoveredAt: string;
    ignoredAt?: string;
    claimedVersionId?: string;

    static createFrom(source: any = {}) {
      return new LegacyAnimationRecoveryItem(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.id = source["id"];
      this.artifactDir = source["artifactDir"];
      this.stateHint = source["stateHint"];
      this.frameCount = source["frameCount"];
      this.thumbnail = source["thumbnail"];
      this.reason = source["reason"];
      this.baseReferenceHash = source["baseReferenceHash"];
      this.contentHash = source["contentHash"];
      this.sizeBytes = source["sizeBytes"];
      this.discoveredAt = source["discoveredAt"];
      this.ignoredAt = source["ignoredAt"];
      this.claimedVersionId = source["claimedVersionId"];
    }
  }
  export class MotionLibraryItem {
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
    alignmentMode?: string;
    createdAt: string;
    updatedAt: string;

    static createFrom(source: any = {}) {
      return new MotionLibraryItem(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.id = source["id"];
      this.name = source["name"];
      this.action = source["action"];
      this.facing = source["facing"];
      this.frames = source["frames"];
      this.fps = source["fps"];
      this.loop = source["loop"];
      this.assetUse = source["assetUse"];
      this.sourceKind = source["sourceKind"];
      this.sourcePath = source["sourcePath"];
      this.sourceFps = source["sourceFps"];
      this.sourceDurationMs = source["sourceDurationMs"];
      this.rawFrames = source["rawFrames"];
      this.processedFrames = source["processedFrames"];
      this.candidateFrames = this.convertValues(
        source["candidateFrames"],
        CandidateFrame,
      );
      this.strip = source["strip"];
      this.thumbnail = source["thumbnail"];
      this.tags = source["tags"];
      this.directUsable = source["directUsable"];
      this.referenceOnly = source["referenceOnly"];
      this.alignmentMode = source["alignmentMode"];
      this.createdAt = source["createdAt"];
      this.updatedAt = source["updatedAt"];
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }
  export class PendingGeneration {
    requestId: string;
    stateId?: string;
    stateName: string;
    rawVideoPath: string;
    status: string;
    startedAt: string;
    updatedAt: string;

    static createFrom(source: any = {}) {
      return new PendingGeneration(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.requestId = source["requestId"];
      this.stateId = source["stateId"];
      this.stateName = source["stateName"];
      this.rawVideoPath = source["rawVideoPath"];
      this.status = source["status"];
      this.startedAt = source["startedAt"];
      this.updatedAt = source["updatedAt"];
    }
  }
  export class ProviderInfo {
    hasKey: boolean;
    keyPreview: string;
    hasTextKey: boolean;
    textKeyPreview: string;
    textUsesProviderKey: boolean;
    model: string;
    models: string[];
    modelListSource?: string;
    modelListError?: string;
    videoModel: string;
    videoModels: string[];
    videoModelCapability: gen.VideoModelCapability;
    videoModelCapabilities: Record<string, gen.VideoModelCapability>;
    videoModelShowcases?: Record<string, gen.VideoModelShowcase>;
    supportsVideo: boolean;
    supportsImage: boolean;
    textModel: string;
    textModels: string[];
    supportsText: boolean;
    visionModel: string;
    visionModels: string[];
    supportsVision: boolean;

    static createFrom(source: any = {}) {
      return new ProviderInfo(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.hasKey = source["hasKey"];
      this.keyPreview = source["keyPreview"];
      this.hasTextKey = source["hasTextKey"];
      this.textKeyPreview = source["textKeyPreview"];
      this.textUsesProviderKey = source["textUsesProviderKey"];
      this.model = source["model"];
      this.models = source["models"];
      this.modelListSource = source["modelListSource"];
      this.modelListError = source["modelListError"];
      this.videoModel = source["videoModel"];
      this.videoModels = source["videoModels"];
      this.videoModelCapability = this.convertValues(
        source["videoModelCapability"],
        gen.VideoModelCapability,
      );
      this.videoModelCapabilities = this.convertValues(
        source["videoModelCapabilities"],
        gen.VideoModelCapability,
        true,
      );
      this.videoModelShowcases = this.convertValues(
        source["videoModelShowcases"],
        gen.VideoModelShowcase,
        true,
      );
      this.supportsVideo = source["supportsVideo"];
      this.supportsImage = source["supportsImage"];
      this.textModel = source["textModel"];
      this.textModels = source["textModels"];
      this.supportsText = source["supportsText"];
      this.visionModel = source["visionModel"];
      this.visionModels = source["visionModels"];
      this.supportsVision = source["supportsVision"];
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }
  export class ReprocessVideoStateArgs {
    requestId: string;
    stateId: string;
    baseImage: string;
    styleKey: string;
    styleCustom: string;
    cellSize: number;
    safeMargin: number;
    rawVideoPath: string;
    videoSeconds: number;
    frameSamplerMode: string;
    alignmentMode: string;
    frameFitMode: string;
    backgroundRemovalMode: string;
    state: sprite.StateSpec;

    static createFrom(source: any = {}) {
      return new ReprocessVideoStateArgs(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.requestId = source["requestId"];
      this.stateId = source["stateId"];
      this.baseImage = source["baseImage"];
      this.styleKey = source["styleKey"];
      this.styleCustom = source["styleCustom"];
      this.cellSize = source["cellSize"];
      this.safeMargin = source["safeMargin"];
      this.rawVideoPath = source["rawVideoPath"];
      this.videoSeconds = source["videoSeconds"];
      this.frameSamplerMode = source["frameSamplerMode"];
      this.alignmentMode = source["alignmentMode"];
      this.frameFitMode = source["frameFitMode"];
      this.backgroundRemovalMode = source["backgroundRemovalMode"];
      this.state = this.convertValues(source["state"], sprite.StateSpec);
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }
  export class RewritePromptArgs {
    text: string;

    static createFrom(source: any = {}) {
      return new RewritePromptArgs(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.text = source["text"];
    }
  }
  export class SaveCharacterLibraryItemArgs {
    id?: string;
    name: string;
    description?: string;
    styleKey: string;
    styleCustom?: string;
    image?: string;

    static createFrom(source: any = {}) {
      return new SaveCharacterLibraryItemArgs(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.id = source["id"];
      this.name = source["name"];
      this.description = source["description"];
      this.styleKey = source["styleKey"];
      this.styleCustom = source["styleCustom"];
      this.image = source["image"];
    }
  }
  export class SaveMotionLibraryItemArgs {
    id?: string;
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
    alignmentMode?: string;

    static createFrom(source: any = {}) {
      return new SaveMotionLibraryItemArgs(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.id = source["id"];
      this.name = source["name"];
      this.action = source["action"];
      this.facing = source["facing"];
      this.frames = source["frames"];
      this.fps = source["fps"];
      this.loop = source["loop"];
      this.assetUse = source["assetUse"];
      this.sourceKind = source["sourceKind"];
      this.sourcePath = source["sourcePath"];
      this.sourceFps = source["sourceFps"];
      this.sourceDurationMs = source["sourceDurationMs"];
      this.rawFrames = source["rawFrames"];
      this.processedFrames = source["processedFrames"];
      this.candidateFrames = this.convertValues(
        source["candidateFrames"],
        CandidateFrame,
      );
      this.strip = source["strip"];
      this.thumbnail = source["thumbnail"];
      this.tags = source["tags"];
      this.directUsable = source["directUsable"];
      this.referenceOnly = source["referenceOnly"];
      this.alignmentMode = source["alignmentMode"];
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }
  export class SettingsInfo {
    provider: string;
    imageProvider: string;
    videoProvider: string;
    textProvider: string;
    visionProvider: string;
    animationPipeline: string;
    videoSeconds: number;
    videoFps: number;
    backgroundRemovalMode: string;
    animationVersionWarningGb: number;
    providers: Record<string, ProviderInfo>;

    static createFrom(source: any = {}) {
      return new SettingsInfo(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.provider = source["provider"];
      this.imageProvider = source["imageProvider"];
      this.videoProvider = source["videoProvider"];
      this.textProvider = source["textProvider"];
      this.visionProvider = source["visionProvider"];
      this.animationPipeline = source["animationPipeline"];
      this.videoSeconds = source["videoSeconds"];
      this.videoFps = source["videoFps"];
      this.backgroundRemovalMode = source["backgroundRemovalMode"];
      this.animationVersionWarningGb = source["animationVersionWarningGb"];
      this.providers = this.convertValues(
        source["providers"],
        ProviderInfo,
        true,
      );
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }
  export class StateResult {
    requestId: string;
    name: string;
    pipeline: string;
    error: string;
    rawStrip: string;
    rawVideoPath: string;
    rawExtractedFrames: string[];
    trace: GenerationTrace;
    candidateFrames: CandidateFrame[];
    frames: string[];
    expected: number;
    found: number;
    warnings: string[];
    scores: sprite.ScoreResult;
    sequenceMetrics: sprite.FrameSequenceMetrics;
    cellSize: number;

    static createFrom(source: any = {}) {
      return new StateResult(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.requestId = source["requestId"];
      this.name = source["name"];
      this.pipeline = source["pipeline"];
      this.error = source["error"];
      this.rawStrip = source["rawStrip"];
      this.rawVideoPath = source["rawVideoPath"];
      this.rawExtractedFrames = source["rawExtractedFrames"];
      this.trace = this.convertValues(source["trace"], GenerationTrace);
      this.candidateFrames = this.convertValues(
        source["candidateFrames"],
        CandidateFrame,
      );
      this.frames = source["frames"];
      this.expected = source["expected"];
      this.found = source["found"];
      this.warnings = source["warnings"];
      this.scores = this.convertValues(source["scores"], sprite.ScoreResult);
      this.sequenceMetrics = this.convertValues(
        source["sequenceMetrics"],
        sprite.FrameSequenceMetrics,
      );
      this.cellSize = source["cellSize"];
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }
  export class ToolAsset {
    id: string;
    name: string;
    mediaType: string;
    mimeType?: string;
    usage?: string;
    sizeBytes?: number;
    width?: number;
    height?: number;
    durationMs?: number;
    thumbnailAvailable?: boolean;
    sourceModule?: string;
    parentAssetId?: string;
    prompt?: string;
    provider?: string;
    model?: string;
    tags?: string[];
    legacyIndexed?: boolean;
    createdAt: string;
    updatedAt: string;

    static createFrom(source: any = {}) {
      return new ToolAsset(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.id = source["id"];
      this.name = source["name"];
      this.mediaType = source["mediaType"];
      this.mimeType = source["mimeType"];
      this.usage = source["usage"];
      this.sizeBytes = source["sizeBytes"];
      this.width = source["width"];
      this.height = source["height"];
      this.durationMs = source["durationMs"];
      this.thumbnailAvailable = source["thumbnailAvailable"];
      this.sourceModule = source["sourceModule"];
      this.parentAssetId = source["parentAssetId"];
      this.prompt = source["prompt"];
      this.provider = source["provider"];
      this.model = source["model"];
      this.tags = source["tags"];
      this.legacyIndexed = source["legacyIndexed"];
      this.createdAt = source["createdAt"];
      this.updatedAt = source["updatedAt"];
    }
  }
  export class ToolchainModelCapability {
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

    static createFrom(source: any = {}) {
      return new ToolchainModelCapability(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.provider = source["provider"];
      this.model = source["model"];
      this.label = source["label"];
      this.supportsText = source["supportsText"];
      this.supportsImageReference = source["supportsImageReference"];
      this.supportsMultipleReferences = source["supportsMultipleReferences"];
      this.maxReferenceImages = source["maxReferenceImages"];
      this.supportsEdit = source["supportsEdit"];
      this.supportsMask = source["supportsMask"];
      this.supportsFirstFrame = source["supportsFirstFrame"];
      this.supportsLastFrame = source["supportsLastFrame"];
      this.supportsIdentityReference = source["supportsIdentityReference"];
      this.supportsAudio = source["supportsAudio"];
      this.supportsWatermark = source["supportsWatermark"];
      this.aspectRatios = source["aspectRatios"];
      this.resolutions = source["resolutions"];
      this.durations = source["durations"];
    }
  }
  export class ToolchainGenerationCapabilities {
    image: ToolchainModelCapability[];
    video: ToolchainModelCapability[];
    vision: ToolchainModelCapability[];
    text: ToolchainModelCapability[];

    static createFrom(source: any = {}) {
      return new ToolchainGenerationCapabilities(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.image = this.convertValues(
        source["image"],
        ToolchainModelCapability,
      );
      this.video = this.convertValues(
        source["video"],
        ToolchainModelCapability,
      );
      this.vision = this.convertValues(
        source["vision"],
        ToolchainModelCapability,
      );
      this.text = this.convertValues(source["text"], ToolchainModelCapability);
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
      } else if ("object" === typeof a) {
        if (asMap) {
          for (const key of Object.keys(a)) {
            a[key] = new classs(a[key]);
          }
          return a;
        }
        return new classs(a);
      }
      return a;
    }
  }
}

export namespace sprite {
  export class DirectionInfo {
    key: string;
    label: string;
    short: string;
    mirrorOf: string;
    row: number;
    col: number;

    static createFrom(source: any = {}) {
      return new DirectionInfo(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.key = source["key"];
      this.label = source["label"];
      this.short = source["short"];
      this.mirrorOf = source["mirrorOf"];
      this.row = source["row"];
      this.col = source["col"];
    }
  }
  export class FrameSequenceMetrics {
    duplicateRate: number;
    continuity: number;
    motionCoverage: number;
    loopClosure?: number;
    activeStartMs?: number;
    activeEndMs?: number;

    static createFrom(source: any = {}) {
      return new FrameSequenceMetrics(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.duplicateRate = source["duplicateRate"];
      this.continuity = source["continuity"];
      this.motionCoverage = source["motionCoverage"];
      this.loopClosure = source["loopClosure"];
      this.activeStartMs = source["activeStartMs"];
      this.activeEndMs = source["activeEndMs"];
    }
  }
  export class PresetInfo {
    name: string;
    label: string;
    category: string;
    action: string;
    frames: number;
    fps: number;
    loop: boolean;

    static createFrom(source: any = {}) {
      return new PresetInfo(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.name = source["name"];
      this.label = source["label"];
      this.category = source["category"];
      this.action = source["action"];
      this.frames = source["frames"];
      this.fps = source["fps"];
      this.loop = source["loop"];
    }
  }
  export class ScoreResult {
    identity: number;
    motion: number;
    contact: number;
    overall: number;

    static createFrom(source: any = {}) {
      return new ScoreResult(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.identity = source["identity"];
      this.motion = source["motion"];
      this.contact = source["contact"];
      this.overall = source["overall"];
    }
  }
  export class StateSpec {
    name: string;
    frames: number;
    fps: number;
    loop: boolean;
    action: string;
    facing: string;

    static createFrom(source: any = {}) {
      return new StateSpec(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.name = source["name"];
      this.frames = source["frames"];
      this.fps = source["fps"];
      this.loop = source["loop"];
      this.action = source["action"];
      this.facing = source["facing"];
    }
  }
}
