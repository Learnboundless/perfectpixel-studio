import { useEffect, useRef, useState } from "react";
import { Images, LibraryBig, ListTodo, Package, Plus, Settings, X } from "lucide-react";
import { AcknowledgeGeneration, CancelGeneration, ClearSession, DeleteCharacterLibraryItem, ExportAnimation, ExportProject, GenerateState, GetSettings, ImportMotionLibraryFile, ImportMotionLibraryFolder, ListCharacterLibrary, ListDirections, ListMotionLibrary, ListPendingGenerations, ListPresets, LoadSession, MirrorFrames, ReprocessVideoState, RevealInFinder, SaveCharacterLibraryItem, SaveMotionLibraryItem, SaveSession } from "../wailsjs/go/main/App";
import { EventsOn } from "../wailsjs/runtime/runtime";
import CharacterPanel from "./components/CharacterPanel";
import CharacterSidebar from "./components/CharacterSidebar";
import GalleryModal from "./components/GalleryModal";
import PreviewPanel from "./components/PreviewPanel";
import SettingsModal, { ISettings } from "./components/SettingsModal";
import StatesPanel from "./components/StatesPanel";
import ModuleNav from "./components/toolchain/ModuleNav";
import ToolchainHub from "./components/toolchain/ToolchainHub";
import { Button } from "./components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "./components/ui/dialog";
import { AlignmentMode, AnimationPipeline, AppModule, CharacterDef, CharacterLibraryItem, CharacterWorkspace, DirectionInfo, FALLBACK_PRESETS, FrameFitMode, FrameItem, FrameSamplerMode, MotionLibraryItem, MotionSourceMode, PresetInfo, StateDef, TaskProgress, VideoPromptLanguage, VideoReferenceMode, VideoShadowMode, VideoSubjectMode, selectedFrames, uid } from "./types";
import { Lang, useI18n } from "./i18n";
import { isBackendCancel, localizeBackendList, localizeBackendText } from "./i18n/backend";
import { directionName } from "./i18n/catalog";
import { findIPRiskTerms } from "./ipRisk";
import logoUrl from "./assets/logo.svg";

interface IToast {
  id: string;
  kind: "info" | "error" | "success";
  text: string;
}

interface IPendingGeneration {
  requestId: string;
  stateId?: string;
  stateName: string;
  rawVideoPath: string;
}

const sessionStateSnapshot = (states: StateDef[]): StateDef[] =>
  states.map((state) => ({
    ...state,
    status: state.status === "generating" ? (state.items.length > 0 ? "done" : "idle") : state.status,
    progress: undefined,
    candidateFrames: state.candidateFrames?.map((candidate) =>
      candidate.sourcePath ? { ...candidate, png: undefined } : candidate
    ),
  }));

const characterFromLibrary = (item: CharacterLibraryItem): CharacterDef => ({
  libraryId: item.id,
  image: item.image ?? null,
  name: item.name ?? "",
  description: item.description ?? "",
  styleKey: item.styleKey || "pixel",
  styleCustom: item.styleCustom ?? "",
});

const emptyCharacter = (libraryId?: string): CharacterDef => ({
  libraryId,
  image: null,
  name: "",
  description: "",
  styleKey: "pixel",
  styleCustom: "",
});

const restoreStates = (
  value: unknown,
  defaultPipeline: string | undefined,
  fallbackFitMode: unknown,
  language: Lang,
): StateDef[] => {
  if (!Array.isArray(value)) return [];
  return value
    .filter((state: any) => state?.mode !== "procedural")
    .map((state: StateDef) => {
      const hasItems = Array.isArray(state.items) && state.items.length > 0;
      const legacyQualityFailure = state.status === "error" && hasItems;
      const restoredStatus = state.status === "generating"
        ? (hasItems ? "done" : "idle")
        : legacyQualityFailure
          ? "review"
          : state.status;
      return {
        ...state,
        isCustom: (state as any).isCustom === true || /^custom\d*$/i.test(state.name),
        customLabel: (state as any).customLabel ?? "",
        status: restoredStatus,
        error: legacyQualityFailure ? undefined : state.error,
        qualityReview: (state as any).qualityReview ?? (legacyQualityFailure
          ? { summary: state.error || localizeBackendText("质检未通过，请检查当前帧。", language), ignored: false, source: "session" }
          : undefined),
        pipeline: normalizePipeline((state as any).pipeline ?? defaultPipeline),
        lastPipeline: (state as any).lastPipeline ? normalizePipeline((state as any).lastPipeline) : undefined,
        progress: undefined,
        rawExtractedFrames: Array.isArray((state as any).rawExtractedFrames) ? (state as any).rawExtractedFrames : [],
        motionSourceMode: normalizeMotionMode((state as any).motionSourceMode),
        motionSourceId: (state as any).motionSourceId ?? "",
        videoReferenceMode: normalizeVideoReferenceMode((state as any).videoReferenceMode),
        videoSubjectMode: normalizeVideoSubjectMode((state as any).videoSubjectMode),
        videoPromptLanguage: normalizeVideoPromptLanguage((state as any).videoPromptLanguage),
        videoShadowMode: normalizeVideoShadowMode((state as any).videoShadowMode),
        frameSamplerMode: normalizeFrameSamplerMode((state as any).frameSamplerMode),
        alignmentMode: normalizeAlignmentMode((state as any).alignmentMode),
        frameFitMode: normalizeFrameFitMode((state as any).frameFitMode ?? fallbackFitMode),
        trace: (state as any).trace,
        candidateFrames: Array.isArray((state as any).candidateFrames) ? (state as any).candidateFrames : [],
        items: Array.isArray(state.items)
          ? state.items.map((frame: any) => ({
              ...frame,
              source: frame.source ?? "session",
              sourceRef: frame.sourceRef ?? frame.replacement?.candidateId ?? "",
            }))
          : [],
      } as StateDef;
    });
};

// 활성 프로바이더에 키가 있는지 확인
const activeImageProvider = (s: ISettings | null) => s?.imageProvider || s?.provider || "gemini";
const activeVideoProvider = (s: ISettings | null) => s?.videoProvider || "zetatechs";
const hasActiveKey = (s: ISettings | null) => !!s?.providers?.[activeImageProvider(s)]?.hasKey;
const hasVideoKey = (s: ISettings | null) => !!s?.providers?.[activeVideoProvider(s)]?.hasKey;
const supportsVideo = (s: ISettings | null) =>
  !!s?.providers?.[activeVideoProvider(s)]?.supportsVideo && hasVideoKey(s);
const hasGenerationKey = (s: ISettings | null) => hasActiveKey(s) || hasVideoKey(s);
const normalizePipeline = (value?: string | null): AnimationPipeline =>
  value === "image_strip" ? "image_strip" : "video_to_frames";
const normalizeMotionMode = (value?: string | null): MotionSourceMode =>
  value === "direct" || value === "guide_image_strip" || value === "ai_video" ? value : "none";
const normalizeVideoReferenceMode = (value?: string | null): VideoReferenceMode =>
  value === "auto" || value === "none" || value === "identity_reference" || value === "first_frame" || value === "start_end_frame" ? value : "identity_reference";
const normalizeVideoSubjectMode = (value?: string | null): VideoSubjectMode =>
  value === "auto" || value === "include_description" || value === "reference_only" ? value : "include_description";
const normalizeVideoPromptLanguage = (value?: string | null): VideoPromptLanguage =>
  value === "zh" || value === "en" ? value : "auto";
const normalizeVideoShadowMode = (value?: string | null): VideoShadowMode =>
  value === "allow_contact" ? "allow_contact" : "avoid";
const normalizeFrameSamplerMode = (value?: string | null): FrameSamplerMode =>
  value === "uniform" ? "uniform" : "candidate_pool";
const normalizeAlignmentMode = (value?: string | null): AlignmentMode =>
  value === "game_anchor_normalize" ? "game_anchor_normalize" : "preserve_source_canvas";
const normalizeFrameFitMode = (value?: string | null): FrameFitMode =>
  value === "auto_expand" || value === "crop" ? value : "contain";
const defaultAlignmentMode = (pipeline: AnimationPipeline, motionMode: MotionSourceMode): AlignmentMode =>
  motionMode === "direct" || pipeline === "video_to_frames" ? "preserve_source_canvas" : "game_anchor_normalize";

const PROVIDER_LABELS: Record<string, string> = {
  gemini: "Gemini",
  openai: "OpenAI",
  openrouter: "OpenRouter",
  fal: "fal.ai",
  byteplus: "BytePlus",
  zetatechs: "ZetaTechs",
  volcengine: "火山方舟",
};

export default function App() {
  const { t, lang } = useI18n();
  const [settings, setSettings] = useState<ISettings | null>(null);
  const [showSettings, setShowSettings] = useState(false);
  const [showGallery, setShowGallery] = useState(false);
  const [confirmNew, setConfirmNew] = useState(false);
  const [character, setCharacter] = useState<CharacterDef>(emptyCharacter());
  const [characterLibrary, setCharacterLibrary] = useState<CharacterLibraryItem[]>([]);
  const [activeCharacterId, setActiveCharacterId] = useState<string | null>(null);
  const [cellSize, setCellSize] = useState(256);
  const [frameFitMode, setFrameFitMode] = useState<FrameFitMode>("contain");
  const [atlasColumns, setAtlasColumns] = useState(8);
  const [states, setStates] = useState<StateDef[]>([]);
  const [motionLibrary, setMotionLibrary] = useState<MotionLibraryItem[]>([]);
  const [directions, setDirections] = useState<DirectionInfo[]>([]);
  const [presets, setPresets] = useState<PresetInfo[]>(FALLBACK_PRESETS);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState("");
  const [progressPercent, setProgressPercent] = useState<number | null>(null);
  const [toasts, setToasts] = useState<IToast[]>([]);
  const [activeModule, setActiveModule] = useState<AppModule>("home");
  const [showToolAssets, setShowToolAssets] = useState(false);
  const [showToolTasks, setShowToolTasks] = useState(false);

  useEffect(() => {
    const receiveToolchainAsset = (event: Event) => {
      const detail = (event as CustomEvent).detail;
      if (detail?.targetModule !== "animation") return;
      void Promise.all([ListCharacterLibrary(), ListMotionLibrary()])
        .then(([characters, motions]) => {
          setCharacterLibrary(Array.isArray(characters) ? characters as unknown as CharacterLibraryItem[] : []);
          setMotionLibrary(Array.isArray(motions) ? motions as unknown as MotionLibraryItem[] : []);
        })
        .catch((error) => {
          const id = uid("toast");
          setToasts((current) => [...current, { id, kind: "error", text: localizeBackendText(error, lang) }]);
          window.setTimeout(() => setToasts((current) => current.filter((item) => item.id !== id)), 4200);
        });
    };
    window.addEventListener("toolchain-asset-sent", receiveToolchainAsset);
    return () => window.removeEventListener("toolchain-asset-sent", receiveToolchainAsset);
  }, []);

  // 최신 상태 참조 (비동기 루프에서 사용)
  const statesRef = useRef(states);
  statesRef.current = states;
  const charRef = useRef(character);
  charRef.current = character;
  const cellRef = useRef(cellSize);
  cellRef.current = cellSize;
  const frameFitRef = useRef(frameFitMode);
  frameFitRef.current = frameFitMode;
  const atlasColumnsRef = useRef(atlasColumns);
  atlasColumnsRef.current = atlasColumns;
  const selectedRef = useRef(selectedId);
  selectedRef.current = selectedId;
  const activeCharacterIdRef = useRef(activeCharacterId);
  activeCharacterIdRef.current = activeCharacterId;
  const workspacesRef = useRef<CharacterWorkspace[]>([]);
  const cancelRef = useRef(false); // 전체 생성 루프 중단 플래그
  const busyRef = useRef(busy);
  busyRef.current = busy;
  const langRef = useRef(lang);
  langRef.current = lang;

  const restoredRef = useRef(false); // 세션 복원 완료 전 자동 저장 방지
  const lastSavedSessionRef = useRef("");

  const syncCurrentWorkspace = (stateList: StateDef[] = statesRef.current): CharacterWorkspace[] => {
    const characterId = activeCharacterIdRef.current;
    if (!characterId) return workspacesRef.current;
    const workspace: CharacterWorkspace = {
      characterId,
      character: { ...charRef.current, libraryId: characterId },
      cellSize: cellRef.current,
      frameFitMode: frameFitRef.current,
      atlasColumns: atlasColumnsRef.current,
      states: sessionStateSnapshot(stateList),
      selectedId: selectedRef.current,
    };
    const exists = workspacesRef.current.some((item) => item.characterId === characterId);
    workspacesRef.current = exists
      ? workspacesRef.current.map((item) => (item.characterId === characterId ? workspace : item))
      : [...workspacesRef.current, workspace];
    return workspacesRef.current;
  };

  const sessionPayload = (stateList: StateDef[] = statesRef.current) => JSON.stringify({
    v: 4,
    activeCharacterId: activeCharacterIdRef.current,
    workspaces: syncCurrentWorkspace(stateList).map((workspace) => ({
      ...workspace,
      character: { ...workspace.character, image: null },
      states: sessionStateSnapshot(workspace.states),
    })),
  });

  const persistSessionNow = async () => {
    const payload = sessionPayload();
    await SaveSession(payload);
    lastSavedSessionRef.current = payload;
  };

  useEffect(() => {
    ListDirections()
      .then((list: any) => setDirections(list ?? []))
      .catch(() => {});
    ListPresets()
      .then((list: any) => {
        if (Array.isArray(list) && list.length > 0) setPresets(list);
      })
      .catch(() => {}); // 실패 시 FALLBACK_PRESETS 유지
    ListMotionLibrary()
      .then((list: any) => setMotionLibrary(Array.isArray(list) ? list : []))
      .catch(() => {});
    const off = EventsOn("progress", (data: any) => {
      const st = data?.state ? `[${data.state}] ` : "";
      const message = localizeBackendText(data?.message ?? "", langRef.current);
      setProgress(message ? `${st}${message}` : "");
      const rawPercent = Number(data?.percent);
      const percent = Number.isFinite(rawPercent) && message ? Math.max(0, Math.min(100, Math.round(rawPercent))) : null;
      setProgressPercent(percent);
      if (data?.state) {
        const current = Number(data?.current);
        const total = Number(data?.total);
        const itemProgress: TaskProgress | undefined = message
          ? {
              phase: String(data?.phase ?? ""),
              message,
              percent: percent ?? undefined,
              current: Number.isFinite(current) ? current : undefined,
              total: Number.isFinite(total) ? total : undefined,
            }
          : undefined;
        setStates((prev) => {
          const next = prev.map((s) => (s.name === data.state ? { ...s, progress: itemProgress } : s));
          statesRef.current = next;
          return next;
        });
      }
    });
    const offArtifact = EventsOn("generation-artifact", (data: IPendingGeneration) => {
      if (!data?.rawVideoPath) return;
      setStates((prev) => {
        const next = prev.map((state) =>
          (data.stateId && state.id === data.stateId) || (!data.stateId && state.name === data.stateName)
            ? { ...state, rawVideoPath: data.rawVideoPath }
            : state
        );
        statesRef.current = next;
        return next;
      });
    });
    const offToolchainAsset = EventsOn("toolchain-asset-sent", (data: any) => {
      if (data?.targetModule !== "animation") return;
      void Promise.all([ListCharacterLibrary(), ListMotionLibrary()])
        .then(([characters, motions]: any[]) => {
          setCharacterLibrary(Array.isArray(characters) ? characters : []);
          setMotionLibrary(Array.isArray(motions) ? motions : []);
        })
        .catch(() => {});
    });

    // 이전 작업 세션 복원
    (async () => {
      try {
        const initialSettings = await refreshSettings();
        const [raw, rawLibrary] = await Promise.all([LoadSession(), ListCharacterLibrary()]);
        const parsed = raw ? JSON.parse(raw) : {};
        let libraryItems = Array.isArray(rawLibrary) ? rawLibrary as unknown as CharacterLibraryItem[] : [];
        let restoredWorkspaces: CharacterWorkspace[] = [];

        if (Array.isArray(parsed?.workspaces)) {
          for (const rawWorkspace of parsed.workspaces) {
            const characterId = String(rawWorkspace?.characterId ?? rawWorkspace?.character?.libraryId ?? "").trim();
            if (!characterId) continue;
            const libraryItem = libraryItems.find((item) => item.id === characterId);
            let restoredCharacter: CharacterDef = libraryItem
              ? characterFromLibrary(libraryItem)
              : {
                  ...emptyCharacter(characterId),
                  ...(rawWorkspace.character ?? {}),
                  libraryId: characterId,
                };
            if (!libraryItem) {
              const saved = await SaveCharacterLibraryItem({
                id: characterId,
                name: restoredCharacter.name,
                description: restoredCharacter.description,
                styleKey: restoredCharacter.styleKey,
                styleCustom: restoredCharacter.styleCustom,
                image: restoredCharacter.image ?? "",
              } as any) as unknown as CharacterLibraryItem;
              libraryItems = [...libraryItems, saved];
              restoredCharacter = characterFromLibrary(saved);
            }
            const restoredStates = restoreStates(
              rawWorkspace.states,
              initialSettings?.animationPipeline,
              rawWorkspace.frameFitMode,
              langRef.current,
            );
            const selected = restoredStates.some((state) => state.id === rawWorkspace.selectedId)
              ? rawWorkspace.selectedId
              : null;
            restoredWorkspaces.push({
              characterId,
              character: restoredCharacter,
              cellSize: typeof rawWorkspace.cellSize === "number" ? rawWorkspace.cellSize : 256,
              frameFitMode: normalizeFrameFitMode(rawWorkspace.frameFitMode),
              atlasColumns: typeof rawWorkspace.atlasColumns === "number"
                ? Math.max(1, Math.min(32, rawWorkspace.atlasColumns))
                : 8,
              states: restoredStates,
              selectedId: selected,
            });
          }
        } else if (parsed?.character) {
          const legacyCharacter: CharacterDef = {
            ...emptyCharacter(),
            image: parsed.character.image ?? null,
            name: parsed.character.name ?? "",
            description: parsed.character.description ?? "",
            styleKey: parsed.character.styleKey ?? "pixel",
            styleCustom: parsed.character.styleCustom ?? "",
          };
          const saved = await SaveCharacterLibraryItem({
            name: legacyCharacter.name,
            description: legacyCharacter.description,
            styleKey: legacyCharacter.styleKey,
            styleCustom: legacyCharacter.styleCustom,
            image: legacyCharacter.image ?? "",
          } as any) as unknown as CharacterLibraryItem;
          libraryItems = [saved, ...libraryItems.filter((item) => item.id !== saved.id)];
          const restoredStates = restoreStates(
            parsed.states,
            initialSettings?.animationPipeline,
            parsed.frameFitMode,
            langRef.current,
          );
          restoredWorkspaces = [{
            characterId: saved.id,
            character: characterFromLibrary(saved),
            cellSize: typeof parsed.cellSize === "number" ? parsed.cellSize : 256,
            frameFitMode: normalizeFrameFitMode(parsed.frameFitMode),
            atlasColumns: typeof parsed.atlasColumns === "number" ? Math.max(1, Math.min(32, parsed.atlasColumns)) : 8,
            states: restoredStates,
            selectedId: restoredStates.some((state) => state.id === parsed.selectedId) ? parsed.selectedId : null,
          }];
        }

        if (restoredWorkspaces.length === 0) {
          let first = libraryItems[0];
          if (!first) {
            first = await SaveCharacterLibraryItem({
              name: t("unnamed_character"),
              styleKey: "pixel",
            } as any) as unknown as CharacterLibraryItem;
            libraryItems = [first];
          }
          restoredWorkspaces = [{
            characterId: first.id,
            character: characterFromLibrary(first),
            cellSize: 256,
            frameFitMode: "contain",
            atlasColumns: 8,
            states: [],
            selectedId: null,
          }];
        }

        let restoredActiveId = String(parsed?.activeCharacterId ?? "").trim();
        if (!restoredWorkspaces.some((workspace) => workspace.characterId === restoredActiveId)) {
          restoredActiveId = restoredWorkspaces[0].characterId;
        }
        const pending = ((await ListPendingGenerations()) ?? []) as unknown as IPendingGeneration[];
        const recovered: IPendingGeneration[] = [];
        for (const item of pending) {
          if (!item.rawVideoPath) continue;
          for (const workspace of restoredWorkspaces) {
            const index = workspace.states.findIndex((state) =>
              (item.stateId && state.id === item.stateId) || (!item.stateId && state.name === item.stateName)
            );
            if (index < 0) continue;
            const state = workspace.states[index];
            workspace.states[index] = {
              ...state,
              rawVideoPath: item.rawVideoPath,
              status: state.items.length > 0 ? "done" : "idle",
              progress: undefined,
              warnings: [...(state.warnings ?? []), localizeBackendText("检测到上次生成已下载的原视频，可在朔源页重新处理。", langRef.current)],
            };
            recovered.push(item);
            break;
          }
        }
        const activeWorkspace = restoredWorkspaces.find((workspace) => workspace.characterId === restoredActiveId)!;
        workspacesRef.current = restoredWorkspaces;
        activeCharacterIdRef.current = restoredActiveId;
        setCharacterLibrary(libraryItems);
        setActiveCharacterId(restoredActiveId);
        setCharacter(activeWorkspace.character);
        setCellSize(activeWorkspace.cellSize);
        setFrameFitMode(activeWorkspace.frameFitMode);
        setAtlasColumns(activeWorkspace.atlasColumns);
        setStates(activeWorkspace.states);
        statesRef.current = activeWorkspace.states;
        setSelectedId(activeWorkspace.selectedId);
        selectedRef.current = activeWorkspace.selectedId;
        if (recovered.length > 0) {
          const recoveredPayload = JSON.stringify({
            v: 4,
            activeCharacterId: restoredActiveId,
            workspaces: restoredWorkspaces.map((workspace) => ({
              ...workspace,
              character: { ...workspace.character, image: null },
              states: sessionStateSnapshot(workspace.states),
            })),
          });
          await SaveSession(recoveredPayload);
          lastSavedSessionRef.current = recoveredPayload;
          await Promise.all(recovered.map((item) => AcknowledgeGeneration(item.requestId)));
        }
      } catch {
        // 손상된 세션은 무시
      } finally {
        restoredRef.current = true;
      }
    })();

    return () => {
      off();
      offArtifact();
      offToolchainAsset();
    };
  }, []);

  // 작업 세션 자동 저장 (디바운스)
  useEffect(() => {
    if (!restoredRef.current) return;
    if (states.some((state) => state.status === "generating")) return;
    const t = setTimeout(() => {
      const payload = sessionPayload(states);
      if (payload === lastSavedSessionRef.current) return;
      SaveSession(payload)
        .then(() => {
          lastSavedSessionRef.current = payload;
        })
        .catch(() => {});
    }, 1200);
    return () => clearTimeout(t);
  }, [character, activeCharacterId, cellSize, frameFitMode, atlasColumns, states, selectedId]);

  // 角色元数据和基础图独立保存到角色库，避免新建项目或会话损坏时丢失。
  useEffect(() => {
    if (!restoredRef.current || !activeCharacterId) return;
    const timer = setTimeout(() => {
      SaveCharacterLibraryItem({
        id: activeCharacterId,
        name: character.name,
        description: character.description,
        styleKey: character.styleKey,
        styleCustom: character.styleCustom,
        image: character.image ?? "",
      } as any)
        .then((saved: any) => {
          setCharacterLibrary((prev) => {
            const exists = prev.some((item) => item.id === saved.id);
            return exists
              ? prev.map((item) => (item.id === saved.id ? saved : item))
              : [saved, ...prev];
          });
        })
        .catch(() => {});
    }, 700);
    return () => clearTimeout(timer);
  }, [activeCharacterId, character]);

  // 전역 단축키: ⌘, 설정 / ⌘E 내보내기 / ⌘G 갤러리
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey)) return;
      if (e.key === ",") {
        e.preventDefault();
        setShowSettings(true);
      } else if (e.key.toLowerCase() === "e") {
        e.preventDefault();
        if (!busyRef.current) handleExport();
      } else if (e.key.toLowerCase() === "g") {
        e.preventDefault();
        setShowGallery((v) => !v);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const refreshSettings = async (): Promise<ISettings | null> => {
    try {
      const s = (await GetSettings()) as unknown as ISettings;
      setSettings(s);
      if (!hasGenerationKey(s)) setShowSettings(true);
      return s;
    } catch {
      return null;
    }
  };

  const refreshMotionLibrary = async (): Promise<MotionLibraryItem[]> => {
    try {
      const list = (await ListMotionLibrary()) as unknown as MotionLibraryItem[];
      const safe = Array.isArray(list) ? list : [];
      setMotionLibrary(safe);
      return safe;
    } catch {
      return motionLibrary;
    }
  };

  const activateCharacterWorkspace = (item: CharacterLibraryItem) => {
    if (busyRef.current) return;
    syncCurrentWorkspace();
    let workspace = workspacesRef.current.find((entry) => entry.characterId === item.id);
    if (!workspace) {
      workspace = {
        characterId: item.id,
        character: characterFromLibrary(item),
        cellSize: 256,
        frameFitMode: "contain",
        atlasColumns: 8,
        states: [],
        selectedId: null,
      };
      workspacesRef.current = [...workspacesRef.current, workspace];
    }
    activeCharacterIdRef.current = item.id;
    charRef.current = workspace.character;
    cellRef.current = workspace.cellSize;
    frameFitRef.current = workspace.frameFitMode;
    atlasColumnsRef.current = workspace.atlasColumns;
    statesRef.current = workspace.states;
    selectedRef.current = workspace.selectedId;
    setActiveCharacterId(item.id);
    setCharacter(workspace.character);
    setCellSize(workspace.cellSize);
    setFrameFitMode(workspace.frameFitMode);
    setAtlasColumns(workspace.atlasColumns);
    setStates(workspace.states);
    setSelectedId(workspace.selectedId);
  };

  const handleAddCharacter = async () => {
    if (busyRef.current) return;
    try {
      const saved = await SaveCharacterLibraryItem({
        name: t("unnamed_character"),
        styleKey: "pixel",
      } as any) as unknown as CharacterLibraryItem;
      setCharacterLibrary((prev) => [saved, ...prev]);
      activateCharacterWorkspace(saved);
    } catch (error) {
      toast("error", localizeBackendText(error, lang));
    }
  };

  const handleSelectCharacter = (id: string) => {
    const item = characterLibrary.find((entry) => entry.id === id);
    if (item) activateCharacterWorkspace(item);
  };

  const handleDeleteCharacter = async (id: string) => {
    if (busyRef.current || characterLibrary.length <= 1) return;
    try {
      await DeleteCharacterLibraryItem(id);
      const remaining = characterLibrary.filter((item) => item.id !== id);
      workspacesRef.current = workspacesRef.current.filter((workspace) => workspace.characterId !== id);
      setCharacterLibrary(remaining);
      if (activeCharacterIdRef.current === id && remaining[0]) {
        activeCharacterIdRef.current = null;
        setActiveCharacterId(null);
        activateCharacterWorkspace(remaining[0]);
      }
    } catch (error) {
      toast("error", localizeBackendText(error, lang));
    }
  };

  const toast = (kind: IToast["kind"], text: string) => {
    const t: IToast = { id: uid("toast"), kind, text };
    setToasts((prev) => [...prev, t]);
    setTimeout(() => setToasts((prev) => prev.filter((x) => x.id !== t.id)), 5000);
  };

  const updateState = (id: string, patch: Partial<StateDef>) => {
    // statesRef를 즉시 갱신: 방향 세트 순차 생성 루프가 리렌더 전에
    // 직전 방향의 결과(정면 스트립, 미러 소스)를 읽을 수 있어야 함
    statesRef.current = statesRef.current.map((s) => (s.id === id ? { ...s, ...patch } : s));
    setStates(statesRef.current);
  };

  const commitGenerationJournal = async (requestId: string) => {
    if (!requestId) return;
    try {
      await persistSessionNow();
      await AcknowledgeGeneration(requestId);
    } catch (e) {
      toast("error", localizeBackendText(`结果已保留，但会话提交失败：${String(e)}`, lang));
    }
  };

  const generateOne = async (id: string, feedback = ""): Promise<boolean> => {
    const st = statesRef.current.find((s) => s.id === id);
    const ch = charRef.current;
    if (!st || (!ch.image && normalizeMotionMode(st.motionSourceMode) !== "direct")) return false;

    const prevItems = st.items;
    const requestId = uid("generation");
    const motionSourceMode = normalizeMotionMode(st.motionSourceMode);
    const preferredPipeline = normalizePipeline(st.pipeline ?? settings?.animationPipeline);
    const actualPipeline: AnimationPipeline =
      motionSourceMode === "direct" || motionSourceMode === "guide_image_strip"
        ? "image_strip"
        : motionSourceMode === "ai_video"
          ? supportsVideo(settings)
            ? "video_to_frames"
            : "image_strip"
          : preferredPipeline === "video_to_frames" && supportsVideo(settings)
            ? "video_to_frames"
            : "image_strip";
    if (
      actualPipeline === "video_to_frames" &&
      normalizeVideoSubjectMode(st.videoSubjectMode) === "auto"
    ) {
      const riskTerms = findIPRiskTerms(ch.description, lang === "zh");
      if (riskTerms.length > 0) {
        const message = t("ip_review_required", { terms: riskTerms.map((match) => match.term).join("、") });
        updateState(id, {
          status: prevItems.length > 0 ? st.status : "idle",
          error: undefined,
          progress: undefined,
          warnings: [...(st.warnings ?? []), message],
        });
        toast("info", message);
        return false;
      }
    }
    updateState(id, {
      status: "generating",
      error: undefined,
      qualityReview: undefined,
      warnings: [],
      progress: { phase: "queued", message: t("progress_queued"), percent: 1 },
      feedback,
      pipeline: preferredPipeline,
      motionSourceMode,
      videoReferenceMode: normalizeVideoReferenceMode(st.videoReferenceMode),
      videoSubjectMode: normalizeVideoSubjectMode(st.videoSubjectMode),
      videoPromptLanguage: normalizeVideoPromptLanguage(st.videoPromptLanguage),
      videoShadowMode: normalizeVideoShadowMode(st.videoShadowMode),
      frameSamplerMode: normalizeFrameSamplerMode(st.frameSamplerMode),
      alignmentMode: normalizeAlignmentMode(st.alignmentMode ?? defaultAlignmentMode(actualPipeline, motionSourceMode)),
      frameFitMode: normalizeFrameFitMode(st.frameFitMode ?? frameFitMode),
    });
    try {
      // 미러 방향: AI 호출 없이 소스 방향 프레임을 좌우 반전
      if (st.mirrorOf) {
        const src = statesRef.current.find(
          (s) => s.dirBase === st.dirBase && s.facing === st.mirrorOf && s.status === "done" && s.items.length > 0
        );
        if (!src) {
          updateState(id, {
            status: "error",
            items: prevItems,
            error: t("err_mirror_source", { dir: directionName(st.mirrorOf, lang) }),
            progress: undefined,
          });
          return false;
        }
        const mirrored: string[] = (await MirrorFrames(src.items.map((f) => f.png))) ?? [];
        const items: FrameItem[] = mirrored.map((png, i) => ({
          id: uid("fr"),
          png,
          selected: src.items[i]?.selected ?? true,
          source: "mirror",
        }));
        updateState(id, {
          status: items.length > 0 ? "done" : "error",
          error: items.length > 0 ? undefined : t("err_mirror_fail"),
          items,
          progress: undefined,
          warnings: [],
          scores: src.scores ? { ...src.scores } : undefined,
          cellSize: src.cellSize,
          frameFitMode: src.frameFitMode,
        });
        return items.length > 0;
      }

      // 방향 세트 소속이면 정면(south) 스트립을 모션 참조로 전달
      let refStrip = "";
      if (st.dirBase && st.facing && st.facing !== "south") {
        const south = statesRef.current.find((s) => s.dirBase === st.dirBase && s.facing === "south");
        refStrip = south?.rawStrip ?? "";
      }

      const res: any = await GenerateState({
        requestId,
        stateId: st.id,
        baseImage: ch.image ?? "",
        description: ch.description,
        styleKey: ch.styleKey,
        styleCustom: ch.styleCustom,
        cellSize: cellRef.current,
        safeMargin: 0,
        feedback,
        refStrip,
        pipeline: actualPipeline,
        videoModel: settings?.providers?.[activeVideoProvider(settings)]?.videoModel ?? "",
        videoSeconds: settings?.videoSeconds ?? 4,
        videoFps: settings?.videoFps ?? 12,
        motionSourceId: st.motionSourceId ?? "",
        motionSourceMode,
        videoReferenceMode: normalizeVideoReferenceMode(st.videoReferenceMode),
        videoSubjectMode: normalizeVideoSubjectMode(st.videoSubjectMode),
        videoPromptLanguage: normalizeVideoPromptLanguage(st.videoPromptLanguage),
        videoShadowMode: normalizeVideoShadowMode(st.videoShadowMode),
        frameSamplerMode: normalizeFrameSamplerMode(st.frameSamplerMode),
        alignmentMode: normalizeAlignmentMode(st.alignmentMode ?? defaultAlignmentMode(actualPipeline, motionSourceMode)),
        frameFitMode: normalizeFrameFitMode(st.frameFitMode ?? frameFitMode),
        state: { name: st.name, frames: st.frames, fps: st.fps, loop: st.loop, action: st.action, facing: st.facing ?? "" },
      } as any);

      const resultSource = res?.trace?.calledAi ? "ai" : motionSourceMode === "direct" ? "motion_library" : "local";
      const items: FrameItem[] = (res.frames ?? []).map((png: string, i: number) => ({
        id: uid("fr"),
        png,
        selected: true,
        source: resultSource,
        sourceRef: res?.trace?.selections?.find((sel: any) => sel.targetFrameIndex === i)?.candidateId ?? "",
      }));
      const scores = res.scores
        ? {
            identity: (res.scores as any).identity ?? 0,
            motion: (res.scores as any).motion ?? 0,
            contact: (res.scores as any).contact ?? 0,
            overall: (res.scores as any).overall ?? 0,
          }
        : undefined;
      // 품질 점수가 매우 낮으면 명시적 경고 추가 (사용자가 재생성을 고려할 수 있도록)
      const extraWarnings: string[] = [];
      if (scores && scores.overall < 0.35) {
        extraWarnings.push(t("score_low_warn"));
      }
      const backendError = typeof res.error === "string" && res.error.trim() ? localizeBackendText(res.error, lang) : "";
      const hasReturnedFrames = items.length > 0;
      const needsReview = hasReturnedFrames && !!backendError;
      const preservedPrevious = !hasReturnedFrames && prevItems.length > 0;
      const usable = hasReturnedFrames && !needsReview;
      updateState(id, {
        status: usable ? "done" : needsReview ? "review" : preservedPrevious ? (st.status === "review" ? "review" : "done") : "error",
        error: hasReturnedFrames || preservedPrevious ? undefined : backendError || t("err_no_frames"),
        qualityReview: needsReview
          ? { summary: backendError, ignored: false, source: "generation" }
          : preservedPrevious ? st.qualityReview : undefined,
        items: hasReturnedFrames ? items : prevItems,
        progress: undefined,
        rawStrip: res.rawStrip || st.rawStrip,
        rawVideoPath: res.rawVideoPath || st.rawVideoPath,
        rawExtractedFrames: Array.isArray(res.rawExtractedFrames) && res.rawExtractedFrames.length > 0 ? res.rawExtractedFrames : st.rawExtractedFrames ?? [],
        trace: res.trace || st.trace,
        candidateFrames: Array.isArray(res.candidateFrames) && res.candidateFrames.length > 0 ? res.candidateFrames : st.candidateFrames ?? [],
        sequenceMetrics: res.sequenceMetrics ?? st.sequenceMetrics,
        pipeline: preferredPipeline,
        lastPipeline: normalizePipeline(res.pipeline ?? actualPipeline),
        motionSourceId: st.motionSourceId ?? "",
        motionSourceMode,
        videoReferenceMode: normalizeVideoReferenceMode(st.videoReferenceMode),
        videoSubjectMode: normalizeVideoSubjectMode(st.videoSubjectMode),
        videoPromptLanguage: normalizeVideoPromptLanguage(st.videoPromptLanguage),
        videoShadowMode: normalizeVideoShadowMode(st.videoShadowMode),
        frameSamplerMode: normalizeFrameSamplerMode(st.frameSamplerMode),
        alignmentMode: normalizeAlignmentMode(st.alignmentMode ?? defaultAlignmentMode(actualPipeline, motionSourceMode)),
        frameFitMode: normalizeFrameFitMode(st.frameFitMode ?? frameFitMode),
        cellSize: typeof res.cellSize === "number" && res.cellSize > 0 ? res.cellSize : st.cellSize,
        warnings: [
          ...(preservedPrevious && backendError ? [localizeBackendText(`本次生成未产出新帧，已保留旧结果：${backendError}`, lang)] : []),
          ...localizeBackendList(res.warnings, lang),
          ...extraWarnings,
        ],
        scores,
      });
      if (res.rawVideoPath) await commitGenerationJournal(String(res.requestId || requestId));
      return usable;
    } catch (e) {
      const raw = String(e);
      const msg = localizeBackendText(raw, lang);
      let recoveredPending: IPendingGeneration | undefined;
      try {
        const pending = ((await ListPendingGenerations()) ?? []) as unknown as IPendingGeneration[];
        recoveredPending = pending.find((item) => item.requestId === requestId);
      } catch {
        // The original error remains the actionable failure.
      }
      if (isBackendCancel(raw)) {
        // 취소: 이전 결과를 보존하고 조용히 복귀
        updateState(id, {
          status: prevItems.length > 0 ? (st.status === "review" ? "review" : "done") : "idle",
          items: prevItems,
          progress: undefined,
          rawVideoPath: recoveredPending?.rawVideoPath || statesRef.current.find((state) => state.id === id)?.rawVideoPath,
        });
        if (recoveredPending) await commitGenerationJournal(requestId);
        toast("info", t("toast_gen_canceled"));
        return false;
      }
      updateState(id, {
        status: prevItems.length > 0 ? (st.status === "review" ? "review" : "done") : "error",
        error: prevItems.length > 0 ? undefined : msg,
        items: prevItems,
        progress: undefined,
        rawVideoPath: recoveredPending?.rawVideoPath || statesRef.current.find((state) => state.id === id)?.rawVideoPath,
        warnings: [
          ...(st.warnings ?? []),
          ...(prevItems.length > 0 ? [localizeBackendText(`本次生成失败，已保留旧结果：${msg}`, lang)] : []),
          ...(recoveredPending ? [localizeBackendText("新原视频已经保存，可在朔源页重新处理。", lang)] : []),
        ],
      });
      if (recoveredPending) await commitGenerationJournal(requestId);
      return false;
    }
  };

  // 여러 상태를 동시성 제한 하에 병렬 생성하고 성공 개수를 반환한다.
  // (fal 등 일부 프로바이더의 rate-limit 대비 — 8방향 세트와 동일한 기본값 3)
  const GEN_CONCURRENCY = 3;
  const generateBatch = async (ids: string[]): Promise<number> => {
    const queue = [...ids];
    let ok = 0;
    const worker = async () => {
      while (queue.length > 0) {
        if (cancelRef.current) return;
        const id = queue.shift()!;
        setSelectedId(id);
        if (await generateOne(id)) ok += 1;
      }
    };
    await Promise.all(Array.from({ length: Math.min(GEN_CONCURRENCY, queue.length) }, worker));
    return ok;
  };

  const handleGenerate = async (id: string) => {
    if (busy) return;
    setBusy(true);
    cancelRef.current = false;
    setSelectedId(id);
    await generateOne(id);
    setBusy(false);
    setProgress("");
  };

  const handleRegenerate = async (id: string, feedback: string) => {
    if (busy) return;
    setBusy(true);
    cancelRef.current = false;
    await generateOne(id, feedback);
    setBusy(false);
    setProgress("");
  };

  const handleReprocessVideo = async (id: string) => {
    if (busy) return;
    const st = statesRef.current.find((s) => s.id === id);
    if (!st?.rawVideoPath) return;
    const prevItems = st.items;
    setBusy(true);
    cancelRef.current = false;
    setSelectedId(id);
    updateState(id, {
      status: "generating",
      error: undefined,
      qualityReview: undefined,
      warnings: [],
      rawStrip: undefined,
      rawExtractedFrames: [],
      candidateFrames: [],
      trace: undefined,
      scores: undefined,
      sequenceMetrics: undefined,
      progress: { phase: "video_reprocess", message: t("reprocess_video_progress"), percent: 1 },
    });
    try {
      const res: any = await ReprocessVideoState({
        requestId: uid("reprocess"),
        stateId: st.id,
        baseImage: charRef.current.image ?? "",
        styleKey: charRef.current.styleKey,
        styleCustom: charRef.current.styleCustom,
        cellSize: cellRef.current,
        safeMargin: 0,
        rawVideoPath: st.rawVideoPath,
        videoSeconds: settings?.videoSeconds ?? 4,
        frameSamplerMode: normalizeFrameSamplerMode(st.frameSamplerMode),
        alignmentMode: normalizeAlignmentMode(st.alignmentMode ?? "preserve_source_canvas"),
        frameFitMode: normalizeFrameFitMode(st.frameFitMode ?? frameFitMode),
        state: { name: st.name, frames: st.frames, fps: st.fps, loop: st.loop, action: st.action, facing: st.facing ?? "" },
      } as any);
      const items: FrameItem[] = (res.frames ?? []).map((png: string, i: number) => ({
        id: uid("fr"),
        png,
        selected: true,
        source: "local_reprocess",
        sourceRef: res?.trace?.selections?.find((sel: any) => sel.targetFrameIndex === i)?.candidateId ?? "",
      }));
      const scores = res.scores
        ? {
            identity: (res.scores as any).identity ?? 0,
            motion: (res.scores as any).motion ?? 0,
            contact: (res.scores as any).contact ?? 0,
            overall: (res.scores as any).overall ?? 0,
          }
        : undefined;
      const backendError = typeof res.error === "string" && res.error.trim() ? localizeBackendText(res.error, lang) : "";
      const hasReturnedFrames = items.length > 0;
      const needsReview = hasReturnedFrames && !!backendError;
      const preservedPrevious = !hasReturnedFrames && prevItems.length > 0;
      const usable = hasReturnedFrames && !needsReview;
      updateState(id, {
        status: usable ? "done" : needsReview ? "review" : preservedPrevious ? (st.status === "review" ? "review" : "done") : "error",
        error: hasReturnedFrames || preservedPrevious ? undefined : backendError || t("err_no_frames"),
        qualityReview: needsReview
          ? { summary: backendError, ignored: false, source: "reprocess" }
          : preservedPrevious ? st.qualityReview : undefined,
        items: hasReturnedFrames ? items : prevItems,
        progress: undefined,
        rawStrip: res.rawStrip || undefined,
        rawVideoPath: res.rawVideoPath || st.rawVideoPath,
        rawExtractedFrames: Array.isArray(res.rawExtractedFrames) ? res.rawExtractedFrames : [],
        trace: res.trace,
        candidateFrames: Array.isArray(res.candidateFrames) ? res.candidateFrames : [],
        sequenceMetrics: res.sequenceMetrics,
        lastPipeline: "video_to_frames",
        frameSamplerMode: normalizeFrameSamplerMode(st.frameSamplerMode),
        alignmentMode: normalizeAlignmentMode(st.alignmentMode ?? "preserve_source_canvas"),
        frameFitMode: normalizeFrameFitMode(st.frameFitMode ?? frameFitMode),
        cellSize: typeof res.cellSize === "number" && res.cellSize > 0 ? res.cellSize : st.cellSize,
        warnings: [
          ...(preservedPrevious && backendError ? [localizeBackendText(`重新处理未产出新帧，已保留旧结果：${backendError}`, lang)] : []),
          ...localizeBackendList(res.warnings, lang),
        ],
        scores,
      });
      toast(usable ? "success" : "info", usable ? t("toast_video_reprocessed") : t("toast_video_reprocess_needs_review"));
    } catch (e) {
      const raw = String(e);
      const msg = localizeBackendText(raw, lang);
      if (isBackendCancel(raw)) {
        updateState(id, { status: prevItems.length > 0 ? (st.status === "review" ? "review" : "done") : "idle", items: prevItems, progress: undefined });
        toast("info", t("toast_gen_canceled"));
      } else {
        updateState(id, {
          status: prevItems.length > 0 ? (st.status === "review" ? "review" : "done") : "error",
          error: prevItems.length > 0 ? undefined : msg,
          items: prevItems,
          progress: undefined,
          warnings: prevItems.length > 0 ? [...(st.warnings ?? []), localizeBackendText(`重新处理失败，已保留旧结果：${msg}`, lang)] : st.warnings,
        });
        toast(prevItems.length > 0 ? "info" : "error", msg);
      }
    } finally {
      setBusy(false);
      setProgress("");
    }
  };

  // 8방향 세트: 5방향 AI 생성(south가 정면 레퍼런스) + 3방향 좌우 미러링
  const handleGenerateDirectionSet = async (id: string) => {
    if (busy || directions.length === 0) return;
    const origin = statesRef.current.find((s) => s.id === id);
    if (!origin || !charRef.current.image) return;

    setBusy(true);
    cancelRef.current = false;

    const base = origin.dirBase ?? origin.name;
    const labelBase = origin.dirBase ? origin.label.split("·")[0] : origin.label;

    // 세트 상태 보장: 클릭한 상태를 south로 전환하고 누락 방향만 추가
    let next = [...statesRef.current];
    if (!origin.dirBase) {
      // 다른 방향으로 생성된 기존 프레임은 south로 재사용할 수 없으므로 초기화
      const keepItems = !origin.facing || origin.facing === "south";
      next = next.map((s) =>
        s.id === id
          ? {
              ...s,
              name: `${base}-south`,
              label: `${labelBase}·${directionName("south", lang)}`,
              dirBase: base,
              facing: "south",
              ...(keepItems
                ? {}
                : {
                    items: [],
                    status: "idle" as const,
                    error: undefined,
                    rawStrip: undefined,
                    rawVideoPath: undefined,
                    rawExtractedFrames: [],
                    warnings: [],
                    scores: undefined,
                  }),
              pipeline: origin.pipeline ?? normalizePipeline(settings?.animationPipeline),
              motionSourceMode: normalizeMotionMode(origin.motionSourceMode),
              motionSourceId: origin.motionSourceId ?? "",
              videoReferenceMode: normalizeVideoReferenceMode(origin.videoReferenceMode),
              videoSubjectMode: normalizeVideoSubjectMode(origin.videoSubjectMode),
              videoPromptLanguage: normalizeVideoPromptLanguage(origin.videoPromptLanguage),
              videoShadowMode: normalizeVideoShadowMode(origin.videoShadowMode),
              frameSamplerMode: normalizeFrameSamplerMode(origin.frameSamplerMode),
              alignmentMode: normalizeAlignmentMode(origin.alignmentMode),
            }
          : s
      );
    }
    const inSet = (key: string) => next.some((s) => s.dirBase === base && s.facing === key);
    for (const d of directions) {
      if (inSet(d.key)) continue;
      next.push({
        id: uid("st"),
        name: `${base}-${d.key}`,
        label: `${labelBase}·${directionName(d.key, lang, d.label)}`,
        frames: origin.frames,
        fps: origin.fps,
        loop: origin.loop,
        action: origin.action,
        status: "idle",
        items: [],
        warnings: [],
        feedback: "",
        pipeline: origin.pipeline ?? normalizePipeline(settings?.animationPipeline),
        motionSourceMode: normalizeMotionMode(origin.motionSourceMode),
        motionSourceId: origin.motionSourceId ?? "",
        videoReferenceMode: normalizeVideoReferenceMode(origin.videoReferenceMode),
        videoSubjectMode: normalizeVideoSubjectMode(origin.videoSubjectMode),
        videoPromptLanguage: normalizeVideoPromptLanguage(origin.videoPromptLanguage),
        videoShadowMode: normalizeVideoShadowMode(origin.videoShadowMode),
        frameSamplerMode: normalizeFrameSamplerMode(origin.frameSamplerMode),
        alignmentMode: normalizeAlignmentMode(origin.alignmentMode),
        facing: d.key,
        dirBase: base,
        mirrorOf: d.mirrorOf || undefined,
      });
    }
    statesRef.current = next;
    setStates(next);

    // 생성 순서: south 먼저(정면 레퍼런스, 나머지 방향의 모션 참조) →
    // 나머지 AI 방향은 동시성 제한 병렬 생성 → 미러 3종(로컬, 빠름)
    setSelectedId(id); // 세트 미리보기(방향 그리드)가 보이도록 south 선택
    let ok = 0;
    let failed = 0;
    const genKey = async (key: string): Promise<boolean | null> => {
      const st = statesRef.current.find((s) => s.dirBase === base && s.facing === key);
      if (!st) return null;
      if (st.status === "done" && st.items.length > 0) return true; // 이미 완성된 방향 재사용
      return generateOne(st.id);
    };
    const tally = (r: boolean | null) => {
      if (r === true) ok += 1;
      else if (r === false) failed += 1;
    };

    // 1) south (모션 참조용) — 반드시 먼저 완료
    if (!cancelRef.current) tally(await genKey("south"));

    // 2) 나머지 AI 방향 병렬 (fal rate-limit 대비 동시성 3개로 제한)
    const aiRest = ["east", "north", "south-east", "north-east"];
    const queue = [...aiRest];
    const CONCURRENCY = 3;
    const worker = async () => {
      while (queue.length > 0) {
        if (cancelRef.current) return;
        const key = queue.shift()!;
        tally(await genKey(key));
      }
    };
    await Promise.all(Array.from({ length: Math.min(CONCURRENCY, queue.length) }, worker));

    // 3) 미러 방향 (east/se/ne 완료 후, AI 호출 없이 좌우 반전)
    const mirrorKeys = directions.filter((d) => d.mirrorOf).map((d) => d.key);
    for (const key of mirrorKeys) {
      if (cancelRef.current) break;
      tally(await genKey(key));
    }
    setBusy(false);
    setProgress("");
    if (!cancelRef.current) {
      toast(
        failed === 0 ? "success" : "info",
        failed === 0 ? t("toast_dirset", { ok }) : t("toast_dirset_failed", { ok, failed })
      );
    }
  };

  const handleGenerateAll = async () => {
    if (busy) return;
    setBusy(true);
    cancelRef.current = false;
    const pending = statesRef.current.filter((s) => s.status !== "done");
    // 미러 방향은 소스 방향이 먼저 생성되어야 하므로 비미러를 먼저 병렬 생성하고,
    // 그 다음 미러를 병렬 생성한다.
    const nonMirror = pending.filter((s) => !s.mirrorOf).map((s) => s.id);
    const mirror = pending.filter((s) => s.mirrorOf).map((s) => s.id);
    const total = nonMirror.length + mirror.length;
    let ok = 0;
    ok += await generateBatch(nonMirror);
    if (!cancelRef.current && mirror.length > 0) {
      ok += await generateBatch(mirror);
    }
    setBusy(false);
    setProgress("");
    if (total > 0 && !cancelRef.current) {
      toast(ok === total ? "success" : "info", t("toast_genall", { ok, total }));
    }
  };

  // Custom states are drafts first. Generation only starts from the explicit
  // button inside the expanded card after choreography has been configured.
  const handleAddCustomBatch = (count: number) => {
    if (busy) return;
    const n = Math.max(1, Math.min(10, count));
    const base = statesRef.current.length;
    const created: StateDef[] = Array.from({ length: n }, (_, i) => ({
      id: uid("st"),
      name: `custom${base + 1 + i}`,
      label: t("custom"),
      isCustom: true,
      customLabel: "",
      frames: 4,
      fps: 8,
      loop: true,
      action: "",
      status: "idle",
      items: [],
      warnings: [],
      feedback: "",
      pipeline: normalizePipeline(settings?.animationPipeline),
      motionSourceMode: "none",
      videoReferenceMode: "identity_reference",
      videoSubjectMode: "include_description",
      videoPromptLanguage: "auto",
      videoShadowMode: "avoid",
      frameSamplerMode: "candidate_pool",
      alignmentMode: defaultAlignmentMode(normalizePipeline(settings?.animationPipeline), "none"),
      frameFitMode,
    }));
    // statesRef를 즉시 갱신: generateOne이 리렌더 전에 새 상태를 찾을 수 있도록
    statesRef.current = [...statesRef.current, ...created];
    setStates(statesRef.current);
    const ids = created.map((s) => s.id);
    setSelectedId(ids[ids.length - 1]);
    toast("info", t("toast_custom_added", { n }));
  };

  const handleCancel = () => {
    cancelRef.current = true;
    CancelGeneration();
  };

  // 새 프로젝트: 작업 내용이 있으면 인앱 확인 모달을 띄우고, 없으면 바로 초기화.
  // (window.confirm은 Wails WKWebView에서 동작하지 않으므로 사용하지 않음)
  const handleNewProject = () => {
    if (busy) return;
    const hasWork = !!charRef.current.image || statesRef.current.length > 0;
    if (hasWork) {
      setConfirmNew(true);
      return;
    }
    resetProject();
  };

  const resetProject = async () => {
    setConfirmNew(false);
    let nextCharacter = emptyCharacter();
    try {
      const saved = await SaveCharacterLibraryItem({
        name: t("unnamed_character"),
        styleKey: "pixel",
      } as any) as unknown as CharacterLibraryItem;
      setCharacterLibrary((prev) => [saved, ...prev.filter((item) => item.id !== saved.id)]);
      nextCharacter = characterFromLibrary(saved);
      activeCharacterIdRef.current = saved.id;
      setActiveCharacterId(saved.id);
      workspacesRef.current = [{
        characterId: saved.id,
        character: nextCharacter,
        cellSize: 256,
        frameFitMode: "contain",
        atlasColumns: 8,
        states: [],
        selectedId: null,
      }];
    } catch {
      workspacesRef.current = [];
      activeCharacterIdRef.current = null;
      setActiveCharacterId(null);
    }
    charRef.current = nextCharacter;
    statesRef.current = [];
    selectedRef.current = null;
    setCharacter(nextCharacter);
    setStates([]);
    setSelectedId(null);
    cellRef.current = 256;
    frameFitRef.current = "contain";
    atlasColumnsRef.current = 8;
    setCellSize(256);
    setFrameFitMode("contain");
    setAtlasColumns(8);
    try {
      await ClearSession();
    } catch {
      // 세션 파일 삭제 실패는 무시 (다음 자동 저장이 덮어씀)
    }
    toast("info", t("toast_new_project"));
  };

  const handleExport = async () => {
    const done = statesRef.current.filter((s) => s.status === "done" && selectedFrames(s).length > 0);
    if (done.length === 0) {
      toast("error", t("toast_no_export"));
      return;
    }
    try {
      const outDir: any = await ExportProject({
        character: charRef.current.name.trim() || "character",
        cellSize: cellRef.current,
        frameFitMode: frameFitRef.current,
        atlasColumns: atlasColumnsRef.current,
        states: done.map((s) => ({
          name: s.name,
          fps: s.fps,
          loop: s.loop,
          frames: selectedFrames(s).map((f) => f.png),
        })),
      } as any);
      if (outDir) {
        toast("success", t("toast_export_done", { dir: outDir }));
        RevealInFinder(outDir);
      }
    } catch (e) {
      toast("error", localizeBackendText(e, lang));
    }
  };

  const handleExportAnimation = async (id: string, format: "gif" | "mp4") => {
    const st = statesRef.current.find((s) => s.id === id);
    if (!st) return;
    if (st.status === "review") {
      toast("info", t("quality_review_export_blocked"));
      return;
    }
    const frames = selectedFrames(st).map((f) => f.png);
    if (frames.length === 0) {
      toast("error", t("toast_no_export"));
      return;
    }
    try {
      const path: any = await ExportAnimation({
        character: charRef.current.name.trim() || "character",
        name: st.name,
        fps: st.fps,
        loop: st.loop,
        format,
        frames,
      } as any);
      if (path) toast("success", t("toast_animation_exported", { path }));
    } catch (e) {
      toast("error", localizeBackendText(e, lang));
    }
  };

  const handleImportMotionLibrary = async (kind: "file" | "folder", id: string) => {
    const st = statesRef.current.find((s) => s.id === id);
    if (!st || busy) return;
    try {
      const payload = {
        name: composeMotionName(st),
        action: st.action || st.name,
        facing: st.facing ?? "",
        frames: st.frames,
        fps: st.fps,
        loop: st.loop,
        cellSize: cellRef.current,
        seconds: settings?.videoSeconds ?? 4,
        tags: [st.name, st.facing ?? ""].filter(Boolean),
      };
      const item: any =
        kind === "folder"
          ? await ImportMotionLibraryFolder(payload as any)
          : await ImportMotionLibraryFile(payload as any);
      if (item?.id) {
        await refreshMotionLibrary();
        updateState(id, {
          motionSourceId: item.id,
          motionSourceMode: "direct",
          pipeline: "image_strip",
          frameSamplerMode: "candidate_pool",
          alignmentMode: "preserve_source_canvas",
        });
        toast("success", t("toast_motion_imported"));
      }
    } catch (e) {
      toast("error", localizeBackendText(e, lang));
    }
  };

  const handleSaveStateToMotionLibrary = async (id: string) => {
    const st = statesRef.current.find((s) => s.id === id);
    if (!st || st.items.length === 0) return;
    try {
      const frames = st.items.map((f) => f.png);
      const item: any = await SaveMotionLibraryItem({
        name: composeMotionName(st),
        action: st.action || st.name,
        facing: st.facing ?? "",
        frames: frames.length || st.frames,
        fps: st.fps,
        loop: st.loop,
        assetUse: "direct",
        sourceKind: st.rawVideoPath ? "video_result" : st.rawStrip ? "image_strip_result" : "manual_frames",
        sourcePath: st.rawVideoPath ?? "",
        sourceFps: st.fps,
        sourceDurationMs: Math.round((frames.length / Math.max(1, st.fps)) * 1000),
        rawFrames: st.rawExtractedFrames ?? [],
        processedFrames: frames,
        candidateFrames: st.candidateFrames ?? [],
        strip: st.rawStrip ?? "",
        thumbnail: frames[0] ?? "",
        tags: [st.name, st.facing ?? "", st.lastPipeline ?? st.pipeline ?? ""].filter(Boolean),
        directUsable: true,
        referenceOnly: false,
        alignmentMode: st.alignmentMode ?? "preserve_source_canvas",
      } as any);
      if (item?.id) {
        await refreshMotionLibrary();
        toast("success", t("toast_motion_saved"));
      }
    } catch (e) {
      toast("error", localizeBackendText(e, lang));
    }
  };

  const composeMotionName = (st: StateDef) => {
    const facing = st.facing ? ` · ${directionName(st.facing, lang)}` : "";
    return `${st.label || st.name}${facing}`;
  };

  const selectedState = states.find((s) => s.id === selectedId) ?? null;
  const exportable = states.some((s) => s.status === "done" && selectedFrames(s).length > 0);

  return (
    <div className="app">
      <header className="topbar">
        <div className="logo">
          <img className="logo-mark" src={logoUrl} alt={t("logo_alt")} />
          <span className="brand-copy">
            <span className="app-name">{t("app_name")}</span>
            <span className="app-tagline">{t("app_tagline")}</span>
          </span>
        </div>
        <ModuleNav value={activeModule} onChange={setActiveModule} />
        <div className="topbar-status">
          {progress && (
            <span className="progress-pill">
              <span className="spinner" />
              {progressPercent !== null && (
                <span className="progress-meter" aria-hidden="true">
                  <span style={{ width: `${progressPercent}%` }} />
                </span>
              )}
              {progress}
              {progressPercent !== null && <strong>{progressPercent}%</strong>}
              <button className="pp-cancel" onClick={handleCancel} title={t("cancel_generation")}>
                <X size={10} />
              </button>
            </span>
          )}
        </div>
        <div className="topbar-actions">
          {activeModule === "animation" && (
            <>
              <Button variant="ghost" size="sm" disabled={busy} onClick={handleNewProject} title={t("new_project_tip")}>
                <Plus size={13} /> {t("new_project")}
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setShowGallery(true)} title={t("gallery_tip")}>
                <Images size={13} /> {t("gallery")}
              </Button>
              <Button size="sm" disabled={!exportable} onClick={handleExport} title={t("export_tip")}>
                <Package size={13} /> {t("export")}
              </Button>
            </>
          )}
          <Button variant="ghost" size="sm" onClick={() => setShowToolAssets(true)} title={t("tool_assets_title")}>
            <LibraryBig size={13} /> {t("tool_assets")}
          </Button>
          <Button variant="ghost" size="sm" onClick={() => setShowToolTasks(true)} title={t("tool_tasks_title")}>
            <ListTodo size={13} /> {t("tool_tasks")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            title={settings
              ? t("provider_pair_tip", {
                  image: PROVIDER_LABELS[activeImageProvider(settings)] ?? activeImageProvider(settings),
                  video: PROVIDER_LABELS[activeVideoProvider(settings)] ?? activeVideoProvider(settings),
                })
              : t("settings_tip")}
            onClick={() => setShowSettings(true)}
          >
            <Settings size={13} />
            {settings
              ? `${PROVIDER_LABELS[activeImageProvider(settings)] ?? activeImageProvider(settings)} / ${PROVIDER_LABELS[activeVideoProvider(settings)] ?? activeVideoProvider(settings)}`
              : t("settings")}
            {settings && !hasGenerationKey(settings) && <span className="text-destructive font-bold">!</span>}
          </Button>
        </div>
      </header>

      <div className="animation-module-host" hidden={activeModule !== "animation"}>
        <CharacterSidebar
          items={characterLibrary}
          activeId={activeCharacterId}
          busy={busy}
          onSelect={handleSelectCharacter}
          onAdd={handleAddCharacter}
          onDelete={handleDeleteCharacter}
        />

        <main className="workspace">
        <section className="panel panel-char">
          <CharacterPanel
            character={character}
            cellSize={cellSize}
            frameFitMode={frameFitMode}
            busy={busy}
            onChange={setCharacter}
            onCellSize={setCellSize}
            onFrameFitMode={setFrameFitMode}
            onError={(m) => (isBackendCancel(m) ? toast("info", t("toast_gen_canceled")) : toast("error", localizeBackendText(m, lang)))}
          />
        </section>

        <section className="panel panel-anim">
          <StatesPanel
            states={states}
            directions={directions}
            presets={presets}
            selectedId={selectedId}
            canGenerate={!!character.image && hasGenerationKey(settings)}
            hasImage={!!character.image}
            supportsVideo={supportsVideo(settings)}
            characterDescription={character.description}
            defaultPipeline={normalizePipeline(settings?.animationPipeline)}
            defaultVideoSeconds={settings?.videoSeconds ?? 4}
            defaultFrameFitMode={frameFitMode}
            motionLibrary={motionLibrary}
            busy={busy}
            onStates={setStates}
            onSelect={setSelectedId}
            onGenerate={handleGenerate}
            onGenerateAll={handleGenerateAll}
            onAddCustomBatch={handleAddCustomBatch}
            onGenerateDirectionSet={handleGenerateDirectionSet}
            onImportMotionLibrary={handleImportMotionLibrary}
          />
        </section>

        <section className="panel panel-preview">
          <PreviewPanel
            state={selectedState}
            allStates={states}
            directions={directions}
            cellSize={cellSize}
            atlasColumns={atlasColumns}
            busy={busy}
            onUpdateState={updateState}
            onSelect={setSelectedId}
            onRegenerate={handleRegenerate}
            onReprocessVideo={handleReprocessVideo}
            onExport={handleExport}
            onAtlasColumns={setAtlasColumns}
            onExportAnimation={handleExportAnimation}
            onSaveMotionLibrary={handleSaveStateToMotionLibrary}
          />
        </section>
        </main>
      </div>

      <ToolchainHub
        activeModule={activeModule}
        showAssets={showToolAssets}
        showTasks={showToolTasks}
        characters={characterLibrary}
        onModuleChange={setActiveModule}
        onShowAssets={setShowToolAssets}
        onShowTasks={setShowToolTasks}
        onNotify={toast}
        defaults={{
          imageProvider: activeImageProvider(settings),
          imageModel: settings?.providers?.[activeImageProvider(settings)]?.model,
          videoProvider: activeVideoProvider(settings),
          videoModel: settings?.providers?.[activeVideoProvider(settings)]?.videoModel,
        }}
      />

      {showSettings && settings && (
        <SettingsModal
          settings={settings}
          onClose={() => setShowSettings(false)}
          onSaved={async (msg, keepOpen) => {
            const s = await refreshSettings();
            if (!keepOpen && hasGenerationKey(s)) setShowSettings(false);
            toast("success", msg);
          }}
        />
      )}

      {showGallery && (
        <GalleryModal onClose={() => setShowGallery(false)} onError={(m) => toast("error", localizeBackendText(m, lang))} />
      )}

      <Dialog open={confirmNew} onOpenChange={(o) => !o && setConfirmNew(false)}>
        <DialogContent className="w-[380px]">
          <DialogTitle>{t("confirm_new_title")}</DialogTitle>
          <DialogDescription>
            {t("confirm_new_desc")}
          </DialogDescription>
          <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 4 }}>
            <Button variant="ghost" size="sm" onClick={() => setConfirmNew(false)}>
              {t("cancel")}
            </Button>
            <Button variant="destructive" size="sm" onClick={resetProject}>
              {t("confirm_new_ok")}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <div className="toasts">
        {toasts.map((t) => (
          <div key={t.id} className={`toast ${t.kind}`}>
            {t.text}
          </div>
        ))}
      </div>
    </div>
  );
}
