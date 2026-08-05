import { useEffect, useState } from "react";
import { CheckCircle2, Clapperboard, Eye, Image as ImageIcon, KeyRound, Loader2, MessageSquareText, SlidersHorizontal, Video } from "lucide-react";
import {
  SaveAnimationPipeline,
  SaveProviderKey,
  SaveProviderModel,
  SaveProviderTextKey,
  SaveProviderTextModel,
  SaveProviderVideoModel,
  SaveProviderVisionModel,
  SaveVideoDefaults,
  SetImageProvider,
  SetTextProvider,
  SetVideoProvider,
  SetVisionProvider,
} from "../../wailsjs/go/main/App";
import { useI18n, LANGUAGES, Lang } from "../i18n";
import { localizeBackendText } from "../i18n/backend";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogTitle } from "./ui/dialog";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select";

export interface IProviderInfo {
  hasKey: boolean;
  keyPreview: string;
  model: string;
  models?: string[]; // 선택 가능한 모델 목록 (최신 모델이 맨 앞)
  modelListSource?: string;
  modelListError?: string;
  videoModel?: string;
  videoModels?: string[];
  videoModelCapability?: IVideoModelCapability;
  videoModelCapabilities?: Record<string, IVideoModelCapability>;
  videoModelShowcases?: Record<string, IVideoModelShowcase>;
  supportsVideo?: boolean;
  supportsImage?: boolean;
  hasTextKey?: boolean;
  textKeyPreview?: string;
  textUsesProviderKey?: boolean;
  textModel?: string;
  textModels?: string[];
  supportsText?: boolean;
  visionModel?: string;
  visionModels?: string[];
  supportsVision?: boolean;
}

export interface IVideoModelShowcase {
  model: string;
  sampleUrl: string;
  workflow?: string;
  testedAt?: string;
  bytes?: number;
  source?: string;
}

export interface IVideoModelCapability {
  model: string;
  family: string;
  endpoint: string;
  providerAdapter?: string;
  referenceSemantics?: string;
  tags?: string[];
  workflowModes?: string[];
  mediaSlots?: string[];
  supportsTextPrompt?: boolean;
  supportsReferenceImage?: boolean;
  supportsIdentityReference?: boolean;
  supportsFirstFrameReference?: boolean;
  supportsImageToVideo?: boolean;
  supportsStartEndFrames?: boolean;
  supportsReferenceVideo?: boolean;
  supportsReferenceAudio?: boolean;
  supportsMultiImageReference?: boolean;
  supportsMotionTransfer?: boolean;
  supportsMetadata?: boolean;
}

export interface ISettings {
  provider: string;
  imageProvider: string;
  videoProvider: string;
  textProvider: string;
  visionProvider: string;
  animationPipeline: string;
  videoSeconds: number;
  videoFps: number;
  providers: Record<string, IProviderInfo>;
}

interface IProps {
  settings: ISettings;
  onClose: () => void;
  onSaved: (msg: string, keepOpen?: boolean) => void | Promise<void>;
}

const PROVIDERS: { key: string; label: string; placeholder: string }[] = [
  { key: "gemini", label: "Gemini", placeholder: "AIza..." },
  { key: "openai", label: "OpenAI", placeholder: "sk-..." },
  { key: "openrouter", label: "OpenRouter", placeholder: "sk-or-..." },
  { key: "fal", label: "fal.ai", placeholder: "key_id:key_secret" },
  { key: "byteplus", label: "BytePlus", placeholder: "ark-..." },
  { key: "zetatechs", label: "ZetaTechs", placeholder: "sk-..." },
  { key: "volcengine", label: "火山方舟", placeholder: "ark-..." },
];

// 멀티 프로바이더 설정 모달 (shadcn Dialog)
export default function SettingsModal({ settings, onClose, onSaved }: IProps) {
  const { t, lang, setLang } = useI18n();
  const selectedImageProvider = settings.imageProvider || settings.provider || "gemini";
  const selectedVideoProvider = settings.videoProvider || "zetatechs";
  const selectedTextProvider = settings.textProvider || "volcengine";
  const selectedVisionProvider = settings.visionProvider || "zetatechs";
  const activeHasKey =
    !!settings.providers?.[selectedImageProvider]?.hasKey ||
    !!settings.providers?.[selectedVideoProvider]?.hasKey;
  const [section, setSection] = useState<"generation" | "credentials" | "samples">(
    activeHasKey ? "generation" : "credentials",
  );
  const [tab, setTab] = useState(selectedImageProvider);
  const [key, setKey] = useState("");
  const [textKey, setTextKey] = useState("");
  const [imageModel, setImageModel] = useState(settings.providers?.[selectedImageProvider]?.model ?? "");
  const [videoModel, setVideoModel] = useState(settings.providers?.[selectedVideoProvider]?.videoModel ?? "");
  const [textModel, setTextModel] = useState(settings.providers?.[selectedTextProvider]?.textModel ?? "");
  const [visionModel, setVisionModel] = useState(settings.providers?.[selectedVisionProvider]?.visionModel ?? "");
  const [pipeline, setPipeline] = useState(settings.animationPipeline || "video_to_frames");
  const [videoSeconds, setVideoSeconds] = useState(settings.videoSeconds || 4);
  const [videoFps, setVideoFps] = useState(settings.videoFps || 12);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const info = settings.providers?.[tab];
  const imageInfo = settings.providers?.[selectedImageProvider];
  const videoInfo = settings.providers?.[selectedVideoProvider];
  const textInfo = settings.providers?.[selectedTextProvider];
  const visionInfo = settings.providers?.[selectedVisionProvider];
  const meta = PROVIDERS.find((p) => p.key === tab)!;
  const isActiveImage = selectedImageProvider === tab;
  const isActiveVideo = selectedVideoProvider === tab;
  const isActiveText = selectedTextProvider === tab;
  const isActiveVision = selectedVisionProvider === tab;
  const videoCapFor = (m: string) =>
    videoInfo?.videoModelCapabilities?.[m] ??
    (m.trim() === videoInfo?.videoModel ? videoInfo?.videoModelCapability : undefined);
  const videoShowcases = (videoInfo?.videoModels ?? [])
    .map((m) => videoInfo?.videoModelShowcases?.[m])
    .filter((sample): sample is IVideoModelShowcase => !!sample?.sampleUrl);
  const selectedVideoCapability = videoCapFor(videoModel.trim()) ?? videoInfo?.videoModelCapability;
  const imageProviderLabel =
    PROVIDERS.find((provider) => provider.key === selectedImageProvider)?.label ?? selectedImageProvider;
  const videoProviderLabel =
    PROVIDERS.find((provider) => provider.key === selectedVideoProvider)?.label ?? selectedVideoProvider;
  const textProviderLabel =
    PROVIDERS.find((provider) => provider.key === selectedTextProvider)?.label ?? selectedTextProvider;
  const visionProviderLabel =
    PROVIDERS.find((provider) => provider.key === selectedVisionProvider)?.label ?? selectedVisionProvider;
  const activeRoleLabel = [
    isActiveImage ? t("image_model") : "",
    isActiveVideo ? t("video_model") : "",
    isActiveText ? t("text_rewrite_model") : "",
    isActiveVision ? t("vision_model") : "",
  ].filter(Boolean).join(" / ");

  useEffect(() => {
    setImageModel(settings.providers?.[selectedImageProvider]?.model ?? "");
  }, [selectedImageProvider, settings.providers]);

  useEffect(() => {
    setVideoModel(settings.providers?.[selectedVideoProvider]?.videoModel ?? "");
  }, [selectedVideoProvider, settings.providers]);

  useEffect(() => {
    setTextModel(settings.providers?.[selectedTextProvider]?.textModel ?? "");
  }, [selectedTextProvider, settings.providers]);

  useEffect(() => {
    setVisionModel(settings.providers?.[selectedVisionProvider]?.visionModel ?? "");
  }, [selectedVisionProvider, settings.providers]);

  const switchTab = (k: string) => {
    setTab(k);
    setKey("");
    setTextKey("");
    setError("");
  };

  const saveTextKey = async () => {
    if (!textKey.trim() || busy) return;
    setBusy(true);
    setError("");
    try {
      await SaveProviderTextKey(tab, textKey.trim());
      setTextKey("");
      onSaved(t("toast_text_key_saved", { provider: meta.label }), true);
    } catch (e) {
      setError(localizeBackendText(e, lang));
    } finally {
      setBusy(false);
    }
  };

  const reuseProviderKey = async () => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await SaveProviderTextKey(tab, "");
      await onSaved(t("toast_text_key_reused", { provider: meta.label }), true);
    } catch (e) {
      setError(localizeBackendText(e, lang));
    } finally {
      setBusy(false);
    }
  };

  const saveKey = async () => {
    if (!key.trim() || busy) return;
    setBusy(true);
    setError("");
    try {
      await SaveProviderKey(tab, key.trim());
      setKey("");
      onSaved(t("toast_key_saved", { provider: meta.label }));
    } catch (e) {
      setError(localizeBackendText(e, lang));
    } finally {
      setBusy(false);
    }
  };

  const saveImageModel = async (explicit?: string) => {
    if (busy) return;
    const next = (explicit ?? imageModel).trim();
    if (explicit !== undefined) setImageModel(next);
    setBusy(true);
    setError("");
    try {
      await SaveProviderModel(selectedImageProvider, next);
      await onSaved(t("toast_model_saved", {
        provider: PROVIDERS.find((item) => item.key === selectedImageProvider)?.label ?? selectedImageProvider,
      }), true);
    } catch (e) {
      setError(localizeBackendText(e, lang));
    } finally {
      setBusy(false);
    }
  };

  const saveSelectedVideoModel = async (explicit?: string) => {
    if (busy) return;
    const next = (explicit ?? videoModel).trim();
    if (explicit !== undefined) setVideoModel(next);
    setBusy(true);
    setError("");
    try {
      await SaveProviderVideoModel(selectedVideoProvider, next);
      await onSaved(t("toast_video_model_saved", {
        provider: PROVIDERS.find((item) => item.key === selectedVideoProvider)?.label ?? selectedVideoProvider,
      }), true);
    } catch (e) {
      setError(localizeBackendText(e, lang));
    } finally {
      setBusy(false);
    }
  };

  const saveSelectedTextModel = async () => {
    if (busy || !textModel.trim()) return;
    setBusy(true);
    setError("");
    try {
      await SaveProviderTextModel(selectedTextProvider, textModel.trim());
      await onSaved(t("toast_text_model_saved", { provider: textProviderLabel }), true);
    } catch (e) {
      setError(localizeBackendText(e, lang));
    } finally {
      setBusy(false);
    }
  };

  const saveSelectedVisionModel = async (explicit?: string) => {
    if (busy) return;
    const next = (explicit ?? visionModel).trim();
    if (!next) return;
    if (explicit !== undefined) setVisionModel(next);
    setBusy(true);
    setError("");
    try {
      await SaveProviderVisionModel(selectedVisionProvider, next);
      await onSaved(t("toast_vision_model_saved", { provider: visionProviderLabel }), true);
    } catch (e) {
      setError(localizeBackendText(e, lang));
    } finally {
      setBusy(false);
    }
  };

  const savePipeline = async (next: string) => {
    if (busy) return;
    setPipeline(next);
    setBusy(true);
    setError("");
    try {
      await SaveAnimationPipeline(next);
      onSaved(t("toast_animation_settings_saved"), true);
    } catch (e) {
      setError(localizeBackendText(e, lang));
    } finally {
      setBusy(false);
    }
  };

  const saveVideoDefaults = async () => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await SaveVideoDefaults(videoSeconds, videoFps);
      onSaved(t("toast_animation_settings_saved"), true);
    } catch (e) {
      setError(localizeBackendText(e, lang));
    } finally {
      setBusy(false);
    }
  };

  const selectImageProvider = async (provider: string) => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await SetImageProvider(provider);
      setImageModel(settings.providers?.[provider]?.model ?? "");
      await onSaved(t("toast_image_provider_active", { provider: PROVIDERS.find((item) => item.key === provider)?.label ?? provider }), true);
    } catch (e) {
      setError(localizeBackendText(e, lang));
    } finally {
      setBusy(false);
    }
  };

  const selectVideoProvider = async (provider: string) => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await SetVideoProvider(provider);
      setVideoModel(settings.providers?.[provider]?.videoModel ?? "");
      await onSaved(t("toast_video_provider_active", { provider: PROVIDERS.find((item) => item.key === provider)?.label ?? provider }), true);
    } catch (e) {
      setError(localizeBackendText(e, lang));
    } finally {
      setBusy(false);
    }
  };

  const selectTextProvider = async (provider: string) => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await SetTextProvider(provider);
      setTextModel(settings.providers?.[provider]?.textModel ?? "");
      await onSaved(t("toast_text_provider_active", {
        provider: PROVIDERS.find((item) => item.key === provider)?.label ?? provider,
      }), true);
    } catch (e) {
      setError(localizeBackendText(e, lang));
    } finally {
      setBusy(false);
    }
  };

  const selectVisionProvider = async (provider: string) => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await SetVisionProvider(provider);
      setVisionModel(settings.providers?.[provider]?.visionModel ?? "");
      await onSaved(t("toast_vision_provider_active", {
        provider: PROVIDERS.find((item) => item.key === provider)?.label ?? provider,
      }), true);
    } catch (e) {
      setError(localizeBackendText(e, lang));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open onOpenChange={(open) => !open && activeHasKey && onClose()}>
      <DialogContent
        hideClose={!activeHasKey}
        className="settings-dialog !w-[920px] !max-w-[calc(100vw-32px)] !gap-0 !overflow-hidden !p-0"
      >
        <header className="settings-header">
          <div>
            <DialogTitle>{t("settings")}</DialogTitle>
            <p>{t(`settings_${section}_desc`)}</p>
          </div>
        </header>

        <nav className="settings-nav" role="tablist" aria-label={t("settings")}>
          <button
            type="button"
            role="tab"
            aria-selected={section === "generation"}
            className={section === "generation" ? "settings-nav-button active" : "settings-nav-button"}
            onClick={() => setSection("generation")}
          >
            <SlidersHorizontal size={15} />
            {t("settings_generation")}
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={section === "credentials"}
            className={section === "credentials" ? "settings-nav-button active" : "settings-nav-button"}
            onClick={() => setSection("credentials")}
          >
            <KeyRound size={15} />
            {t("settings_credentials")}
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={section === "samples"}
            className={section === "samples" ? "settings-nav-button active" : "settings-nav-button"}
            onClick={() => setSection("samples")}
          >
            <Clapperboard size={15} />
            {t("settings_samples")}
            {videoShowcases.length > 0 && <span className="settings-nav-count">{videoShowcases.length}</span>}
          </button>
        </nav>

        <div className="settings-content">
          {section === "generation" && (
            <>
              <section className="settings-section">
                <div className="settings-section-head">
                  <div>
                    <h3>{t("settings_basics")}</h3>
                    <p>{t("settings_basics_desc")}</p>
                  </div>
                </div>
                <div className="settings-control-grid">
                  <div className="field">
                    <Label>{t("language")}</Label>
                    <Select value={lang} onValueChange={(value) => setLang(value as Lang)}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {LANGUAGES.map((language) => (
                          <SelectItem key={language.code} value={language.code}>
                            {language.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="field">
                    <Label>{t("animation_pipeline")}</Label>
                    <Select value={pipeline} onValueChange={savePipeline}>
                      <SelectTrigger disabled={busy}>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="video_to_frames">{t("pipeline_video")}</SelectItem>
                        <SelectItem value="image_strip">{t("pipeline_image")}</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                <p className="hint">{t("pipeline_hint")}</p>
              </section>

              <section className="settings-section settings-models-section">
                <div className="settings-section-head">
                  <div>
                    <h3>{t("settings_models")}</h3>
                    <p>{t("settings_models_desc")}</p>
                  </div>
                </div>

                <div className="settings-provider-row">
                  <div className="settings-provider-identity">
                    <span className="settings-provider-icon"><ImageIcon size={17} /></span>
                    <div>
                      <strong>{t("image_model")}</strong>
                      <span>{imageProviderLabel}</span>
                    </div>
                  </div>
                  <div className="settings-provider-controls">
                    <div className="field">
                      <Label>{t("image_provider")}</Label>
                      <Select value={selectedImageProvider} onValueChange={selectImageProvider}>
                        <SelectTrigger disabled={busy}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {PROVIDERS.filter((provider) => settings.providers?.[provider.key]?.supportsImage).map((provider) => (
                            <SelectItem key={provider.key} value={provider.key}>{provider.label}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="field">
                      <Label>{t("image_model")}</Label>
                      <Select
                        value={imageInfo?.models?.includes(imageModel.trim()) ? imageModel.trim() : undefined}
                        onValueChange={(value) => value !== imageModel.trim() && saveImageModel(value)}
                      >
                        <SelectTrigger disabled={busy || !imageInfo?.models?.length}>
                          <SelectValue placeholder={imageModel || t("custom_model_inuse")} />
                        </SelectTrigger>
                        <SelectContent>
                          {(imageInfo?.models ?? []).map((item, index) => (
                            <SelectItem key={item} value={item}>
                              {item}{index === 0 && t("latest")}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    {selectedImageProvider === "zetatechs" && (
                      <p
                        className={imageInfo?.modelListError ? "source-error settings-source-line" : "hint settings-source-line"}
                        title={imageInfo?.modelListError ? localizeBackendText(imageInfo.modelListError, lang) : undefined}
                      >
                        {imageInfo?.modelListError
                          ? t("model_list_refresh_failed")
                          : imageInfo?.modelListSource === "dynamic"
                            ? t("model_list_dynamic")
                            : t("model_list_static")}
                      </p>
                    )}
                  </div>
                </div>

                <div className="settings-provider-row">
                  <div className="settings-provider-identity">
                    <span className="settings-provider-icon video"><Video size={17} /></span>
                    <div>
                      <strong>{t("video_model")}</strong>
                      <span>{videoProviderLabel}</span>
                    </div>
                  </div>
                  <div className="settings-provider-controls">
                    <div className="field">
                      <Label>{t("video_provider")}</Label>
                      <Select value={selectedVideoProvider} onValueChange={selectVideoProvider}>
                        <SelectTrigger disabled={busy}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {PROVIDERS.filter((provider) => settings.providers?.[provider.key]?.supportsVideo).map((provider) => (
                            <SelectItem key={provider.key} value={provider.key}>{provider.label}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="field">
                      <Label>{t("video_model")}</Label>
                      <Select
                        value={videoInfo?.videoModels?.includes(videoModel.trim()) ? videoModel.trim() : undefined}
                        onValueChange={(value) => value !== videoModel.trim() && saveSelectedVideoModel(value)}
                      >
                        <SelectTrigger disabled={busy || !videoInfo?.videoModels?.length}>
                          <SelectValue placeholder={videoModel || t("custom_model_inuse")} />
                        </SelectTrigger>
                        <SelectContent>
                          {(videoInfo?.videoModels ?? []).map((item, index) => (
                            <SelectItem key={item} value={item}>
                              {item}{index === 0 && t("latest")}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="settings-source-line">
                      <CapabilityTags cap={selectedVideoCapability} t={t} />
                    </div>
                    {(selectedVideoProvider === "zetatechs" || selectedVideoProvider === "volcengine") && (
                      <p
                        className={videoInfo?.modelListError ? "source-error settings-source-line" : "hint settings-source-line"}
                        title={videoInfo?.modelListError ? localizeBackendText(videoInfo.modelListError, lang) : undefined}
                      >
                        {videoInfo?.modelListError
                          ? t("model_list_refresh_failed")
                          : videoInfo?.modelListSource === "dynamic"
                            ? t(selectedVideoProvider === "volcengine" ? "model_list_dynamic_volcengine" : "model_list_dynamic")
                            : t("model_list_static")}
                      </p>
                    )}
                  </div>
                </div>

                <div className="settings-provider-row">
                  <div className="settings-provider-identity">
                    <span className="settings-provider-icon text"><MessageSquareText size={17} /></span>
                    <div>
                      <strong>{t("text_rewrite_model")}</strong>
                      <span>{textProviderLabel}</span>
                    </div>
                  </div>
                  <div className="settings-provider-controls">
                    <div className="field">
                      <Label>{t("text_provider")}</Label>
                      <Select value={selectedTextProvider} onValueChange={selectTextProvider}>
                        <SelectTrigger disabled={busy}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {PROVIDERS.filter((provider) => settings.providers?.[provider.key]?.supportsText).map((provider) => (
                            <SelectItem key={provider.key} value={provider.key}>{provider.label}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="field">
                      <Label>{t("text_model_id")}</Label>
                      <div className="settings-inline-save">
                        <Input
                          value={textModel}
                          list={`text-models-${selectedTextProvider}`}
                          placeholder={t("text_model_placeholder")}
                          onChange={(event) => setTextModel(event.target.value)}
                          onKeyDown={(event) => event.key === "Enter" && saveSelectedTextModel()}
                        />
                        <datalist id={`text-models-${selectedTextProvider}`}>
                          {(textInfo?.textModels ?? []).map((model) => <option key={model} value={model} />)}
                        </datalist>
                        <Button variant="outline" onClick={saveSelectedTextModel} disabled={busy || !textModel.trim()}>
                          {t("apply")}
                        </Button>
                      </div>
                    </div>
                    <p className="hint settings-source-line">
                      {textInfo?.hasTextKey
                        ? textInfo.textUsesProviderKey
                          ? t("text_key_reusing_provider")
                          : t("text_key_dedicated")
                        : t("text_key_missing")}
                    </p>
                  </div>
                </div>

                <div className="settings-provider-row">
                  <div className="settings-provider-identity">
                    <span className="settings-provider-icon vision"><Eye size={17} /></span>
                    <div>
                      <strong>{t("vision_model")}</strong>
                      <span>{visionProviderLabel}</span>
                    </div>
                  </div>
                  <div className="settings-provider-controls">
                    <div className="field">
                      <Label>{t("vision_provider")}</Label>
                      <Select value={selectedVisionProvider} onValueChange={selectVisionProvider}>
                        <SelectTrigger disabled={busy}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {PROVIDERS.filter((provider) => settings.providers?.[provider.key]?.supportsVision).map((provider) => (
                            <SelectItem key={provider.key} value={provider.key}>{provider.label}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="field">
                      <Label>{t("vision_model")}</Label>
                      <Select
                        value={visionInfo?.visionModels?.includes(visionModel.trim()) ? visionModel.trim() : undefined}
                        onValueChange={(value) => value !== visionModel.trim() && saveSelectedVisionModel(value)}
                      >
                        <SelectTrigger disabled={busy || !visionInfo?.visionModels?.length}>
                          <SelectValue placeholder={visionModel || t("custom_model_inuse")} />
                        </SelectTrigger>
                        <SelectContent>
                          {(visionInfo?.visionModels ?? []).map((item, index) => (
                            <SelectItem key={item} value={item}>{item}{index === 0 && t("latest")}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <p className="hint settings-source-line">{t("vision_model_hint")}</p>
                  </div>
                </div>
              </section>

              <section className="settings-section">
                <div className="settings-section-head">
                  <div>
                    <h3>{t("settings_video_parameters")}</h3>
                    <p>{t("settings_video_parameters_desc")}</p>
                  </div>
                </div>
                <div className="settings-number-row">
                  <div className="field">
                    <Label>{t("video_seconds")}</Label>
                    <Input
                      type="number"
                      min={1}
                      max={15}
                      value={videoSeconds}
                      onChange={(event) => setVideoSeconds(Math.max(1, Math.min(15, Number(event.target.value) || 4)))}
                    />
                  </div>
                  <div className="field">
                    <Label>{t("video_fps")}</Label>
                    <Input
                      type="number"
                      min={1}
                      max={30}
                      value={videoFps}
                      onChange={(event) => setVideoFps(Math.max(1, Math.min(30, Number(event.target.value) || 12)))}
                    />
                  </div>
                  <Button variant="outline" onClick={saveVideoDefaults} disabled={busy}>
                    {t("apply")}
                  </Button>
                </div>
                <p className="hint">{t("video_defaults_hint", { seconds: videoSeconds, fps: videoFps })}</p>
              </section>
            </>
          )}

          {section === "credentials" && (
            <section className="settings-section credentials-section">
              <div className="settings-section-head">
                <div>
                  <h3>{t("settings_credentials")}</h3>
                  <p>{t("settings_credentials_desc")}</p>
                </div>
              </div>

              <div className="provider-tabs-scroll">
                <div className="provider-tabs-row" role="tablist" aria-label={t("settings_credentials")}>
                  {PROVIDERS.map((provider) => (
                    <button
                      key={provider.key}
                      type="button"
                      role="tab"
                      aria-selected={tab === provider.key}
                      className={tab === provider.key ? "provider-tab active" : "provider-tab"}
                      onClick={() => switchTab(provider.key)}
                    >
                      {provider.label}
                      {settings.providers?.[provider.key]?.hasKey && (
                        <CheckCircle2 className="provider-key-check" size={13} aria-label={t("credentials_saved")} />
                      )}
                    </button>
                  ))}
                </div>
              </div>

              <div className="credentials-body">
                <div className="credentials-title">
                  <div>
                    <strong>{meta.label}</strong>
                    <span className={info?.hasKey ? "credential-status saved" : "credential-status"}>
                      {info?.hasKey ? t("credentials_saved") : t("credentials_missing")}
                    </span>
                  </div>
                  {(isActiveImage || isActiveVideo || isActiveText || isActiveVision) && (
                    <span className="credential-role">
                      {activeRoleLabel}
                    </span>
                  )}
                </div>

                <div className="field">
                  <Label>{t("api_key", { provider: meta.label })}</Label>
                  <div className="credentials-input-row">
                    <Input
                      type="password"
                      placeholder={info?.hasKey ? t("saved_as", { preview: info.keyPreview }) : meta.placeholder}
                      value={key}
                      onChange={(event) => setKey(event.target.value)}
                      onKeyDown={(event) => event.key === "Enter" && saveKey()}
                    />
                    <Button onClick={saveKey} disabled={busy || !key.trim()}>
                      {busy && <Loader2 size={13} className="animate-spin" />}
                      {busy ? t("checking") : t("save_key")}
                    </Button>
                  </div>
                </div>

                <p className="hint">
                  {t(`help_${tab}`)}
                  <br />{t("key_local_note")}
                </p>

                {info?.supportsText && (
                  <div className="text-credential-block">
                    <div className="text-credential-heading">
                      <div>
                        <strong>{t("text_key_title")}</strong>
                        <p>{t("text_key_hint")}</p>
                      </div>
                      {info.hasTextKey && !info.textUsesProviderKey && (
                        <Button variant="ghost" size="sm" onClick={reuseProviderKey} disabled={busy}>
                          {t("reuse_provider_key")}
                        </Button>
                      )}
                    </div>
                    <div className="credentials-input-row">
                      <Input
                        type="password"
                        placeholder={info?.hasTextKey
                          ? t("saved_as", { preview: info.textKeyPreview || info.keyPreview })
                          : meta.placeholder}
                        value={textKey}
                        onChange={(event) => setTextKey(event.target.value)}
                        onKeyDown={(event) => event.key === "Enter" && saveTextKey()}
                      />
                      <Button variant="outline" onClick={saveTextKey} disabled={busy || !textKey.trim()}>
                        {busy && <Loader2 size={13} className="animate-spin" />}
                        {t("save_text_key")}
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            </section>
          )}

          {section === "samples" && (
            <section className="settings-section samples-section">
              <div className="settings-section-head settings-sample-heading">
                <div>
                  <h3>{t("verified_video_models")}</h3>
                  <p>{t("verified_video_models_hint")}</p>
                </div>
                <span>{t("tested_model_count", { n: videoShowcases.length })}</span>
              </div>

              <div className="settings-sample-current">
                <div>
                  <span>{t("video_provider")}</span>
                  <strong>{videoProviderLabel}</strong>
                </div>
                <div>
                  <span>{t("current_model")}</span>
                  <strong title={videoModel}>{videoModel}</strong>
                </div>
              </div>

              {videoShowcases.length > 0 ? (
                <div className="video-showcase-grid">
                  {videoShowcases.map((sample) => {
                    const active = sample.model === videoModel.trim();
                    const workflow = sample.workflow ? t(`video_cap_${sample.workflow}`) : "";
                    return (
                      <div key={sample.model} className={active ? "video-sample active" : "video-sample"}>
                        <video
                          src={sample.sampleUrl}
                          controls
                          muted
                          loop
                          playsInline
                          preload="metadata"
                          aria-label={t("sample_video_for", { model: sample.model })}
                        />
                        <div className="video-sample-meta">
                          <strong title={sample.model}>{sample.model}</strong>
                          <span>{workflow || sample.workflow || t("sample_video")}</span>
                          <small>
                            {formatBytes(sample.bytes)}
                            {sample.testedAt ? ` · ${formatDate(sample.testedAt)}` : ""}
                          </small>
                        </div>
                        <Button
                          variant={active ? "secondary" : "outline"}
                          size="sm"
                          onClick={() => !active && saveSelectedVideoModel(sample.model)}
                          disabled={busy || active}
                        >
                          {active ? t("current_model") : t("use_this_model")}
                        </Button>
                      </div>
                    );
                  })}
                </div>
              ) : (
                <div className="settings-empty">
                  <Clapperboard size={24} />
                  <strong>{t("samples_empty")}</strong>
                  <p>{t("verified_video_models_hint")}</p>
                </div>
              )}
            </section>
          )}

          {error && <p className="settings-error">{error}</p>}
        </div>

        <footer className="settings-footer">
          <span>{busy ? t("checking") : ""}</span>
          {activeHasKey && (
            <Button variant="ghost" onClick={onClose}>
              {t("close")}
            </Button>
          )}
        </footer>
      </DialogContent>
    </Dialog>
  );
}

function formatBytes(bytes?: number) {
  if (!bytes || bytes <= 0) return "";
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function formatDate(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function CapabilityTags({
  cap,
  t,
  compact = false,
}: {
  cap?: IVideoModelCapability;
  t: (key: string, vars?: Record<string, string | number>) => string;
  compact?: boolean;
}) {
  if (!cap) return null;
  const tags = cap.tags ?? [];
  if (tags.length === 0 && !cap.endpoint) return null;
  const visible = compact ? tags.slice(0, 3) : tags;
  return (
    <div className={compact ? "model-tags compact" : "model-tags"}>
      {cap.family && <span className="model-tag strong">{cap.family}</span>}
      {visible.map((tag) => (
        <span key={tag} className="model-tag">
          {t(`video_cap_${tag}`)}
        </span>
      ))}
    </div>
  );
}
