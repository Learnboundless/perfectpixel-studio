import { useState } from "react";
import { ImagePlus, Loader2, ShieldAlert, Sparkles, Wand2 } from "lucide-react";
import { PickImage, GenerateCharacter, OptimizeCharacterPrompt, RewriteRiskyPrompt } from "../../wailsjs/go/main/App";
import { CharacterDef, FrameFitMode, STYLE_OPTIONS, CELL_SIZES } from "../types";
import { useI18n } from "../i18n";
import { styleLabel } from "../i18n/catalog";
import { findIPRiskTerms, neutralizeIPDescription } from "../ipRisk";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select";
import { Tabs, TabsList, TabsTrigger } from "./ui/tabs";
import { Textarea } from "./ui/textarea";

interface IProps {
  character: CharacterDef;
  cellSize: number;
  frameFitMode: FrameFitMode;
  busy: boolean;
  onChange: (c: CharacterDef) => void;
  onCellSize: (n: number) => void;
  onFrameFitMode: (mode: FrameFitMode) => void;
  onError: (msg: string) => void;
}

// 1단계: 캐릭터 입력 패널 (이미지 업로드 또는 AI 생성)
export default function CharacterPanel({
  character,
  cellSize,
  frameFitMode,
  busy,
  onChange,
  onCellSize,
  onFrameFitMode,
  onError,
}: IProps) {
  const { t, lang } = useI18n();
  const [mode, setMode] = useState<"upload" | "ai">("upload");
  const [dragOver, setDragOver] = useState(false);
  const [genBusy, setGenBusy] = useState(false);
  const [optimizeBusy, setOptimizeBusy] = useState(false);
  const [rewriteBusy, setRewriteBusy] = useState(false);
  const ipRiskMatches = findIPRiskTerms(character.description, lang === "zh");

  const set = (patch: Partial<CharacterDef>) => onChange({ ...character, ...patch });

  const pickFile = async () => {
    try {
      const dataURL = await PickImage();
      if (dataURL) set({ image: dataURL });
    } catch (e) {
      onError(String(e));
    }
  };

  const readDropped = (file: File) => {
    if (!file.type.startsWith("image/")) {
      onError(t("err_image_only"));
      return;
    }
    const reader = new FileReader();
    reader.onload = () => set({ image: String(reader.result) });
    reader.readAsDataURL(file);
  };

  const generateBase = async () => {
    if (!character.description.trim() || genBusy) return;
    setGenBusy(true);
    try {
      const dataURL = await GenerateCharacter({
        description: character.description,
        styleKey: character.styleKey,
        styleCustom: character.styleCustom,
      } as any);
      if (dataURL) set({ image: dataURL });
    } catch (e) {
      onError(String(e));
    } finally {
      setGenBusy(false);
    }
  };

  const rewriteWithAI = async () => {
    if (!character.description.trim() || rewriteBusy) return;
    setRewriteBusy(true);
    try {
      const neutralInput = neutralizeIPDescription(character.description, lang === "zh");
      const rewritten = await RewriteRiskyPrompt({ text: neutralInput } as any);
      if (rewritten) set({ description: rewritten });
    } catch (e) {
      onError(String(e));
    } finally {
      setRewriteBusy(false);
    }
  };

  const optimizeWithAI = async () => {
    if (!character.description.trim() || optimizeBusy) return;
    setOptimizeBusy(true);
    try {
      const optimized = await OptimizeCharacterPrompt({ text: character.description } as any);
      if (optimized) set({ description: optimized });
    } catch (e) {
      onError(String(e));
    } finally {
      setOptimizeBusy(false);
    }
  };

  return (
    <>
      <div className="panel-head">
        <span className="step-badge">1</span>
        <span className="panel-title">{t("char_style")}</span>
      </div>

      <div className="panel-body">
      <Tabs value={mode} onValueChange={(v) => setMode(v as "upload" | "ai")}>
        <TabsList>
          <TabsTrigger value="upload">{t("upload_image")}</TabsTrigger>
          <TabsTrigger value="ai">{t("ai_generate")}</TabsTrigger>
        </TabsList>
      </Tabs>

      <div
        className={`dropzone checker ${dragOver ? "over" : ""}`}
        onClick={pickFile}
        onDragOver={(e) => {
          e.preventDefault();
          setDragOver(true);
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragOver(false);
          const f = e.dataTransfer.files?.[0];
          if (f) readDropped(f);
        }}
      >
        {character.image ? (
          <>
            <img src={character.image} alt={t("base_char_alt")} className="pixelated" />
            <Button
              variant="outline"
              size="sm"
              className="dz-replace"
              onClick={(e) => {
                e.stopPropagation();
                pickFile();
              }}
            >
              {t("replace")}
            </Button>
          </>
        ) : (
          <>
            <ImagePlus size={28} className="text-muted-foreground mb-2" />
            <div className="dz-text">
              {t("dropzone_line1")}
              <br />
              {t("dropzone_line2")}
            </div>
          </>
        )}
      </div>

      <div className="field">
        <Label>{t("char_name")}</Label>
        <Input
          type="text"
          placeholder={t("char_name_ph")}
          value={character.name}
          onChange={(e) => set({ name: e.target.value })}
        />
      </div>

      <div className="field">
        <div className="field-label-row">
          <Label>{t("char_desc")} {mode === "ai" ? t("char_desc_gen") : t("char_desc_opt")}</Label>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={optimizeWithAI}
            disabled={optimizeBusy || rewriteBusy || busy || !character.description.trim()}
          >
            {optimizeBusy ? <Loader2 size={12} className="animate-spin" /> : <Sparkles size={12} />}
            {optimizeBusy ? t("prompt_optimizing") : t("prompt_optimize")}
          </Button>
        </div>
        <Textarea
          placeholder={t("char_desc_ph")}
          value={character.description}
          onChange={(e) => set({ description: e.target.value })}
        />
      </div>

      {ipRiskMatches.length > 0 && (
        <div className="ip-risk-note">
          <div className="ip-risk-head">
            <ShieldAlert size={14} />
            <strong>{t("ip_risk_title")}</strong>
          </div>
          <p>{t("ip_risk_hint", { terms: ipRiskMatches.map((match) => match.term).join("、") })}</p>
          <div className="ip-risk-mapping">
            {ipRiskMatches.map((match) => (
              <span key={match.group}>{match.term} → {match.replacement}</span>
            ))}
          </div>
          <div className="ip-risk-actions">
            <small>{t("ip_risk_disclaimer")}</small>
            <div>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={() => set({ description: neutralizeIPDescription(character.description, lang === "zh") })}
              >
                <Wand2 size={12} /> {t("ip_risk_apply_neutral")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={rewriteWithAI}
                disabled={rewriteBusy || optimizeBusy || busy}
              >
                {rewriteBusy ? <Loader2 size={12} className="animate-spin" /> : <Sparkles size={12} />}
                {rewriteBusy ? t("ip_risk_ai_rewriting") : t("ip_risk_ai_rewrite")}
              </Button>
            </div>
          </div>
        </div>
      )}

      {mode === "ai" && (
        <Button onClick={generateBase} disabled={genBusy || busy || !character.description.trim()}>
          {genBusy ? <Loader2 size={13} className="animate-spin" /> : <Sparkles size={13} />}
          {genBusy ? t("generating_char") : t("generate_base")}
        </Button>
      )}

      <div className="sub-title">{t("style")}</div>

      <div className="field">
        <Label>{t("art_style")}</Label>
        <Select value={character.styleKey} onValueChange={(v) => set({ styleKey: v })}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {STYLE_OPTIONS.map((o) => (
              <SelectItem key={o.key} value={o.key}>
                {styleLabel(o.key, lang, o.label)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {character.styleKey === "custom" && (
        <div className="field">
          <Label>{t("custom_style_label")}</Label>
          <Textarea
            placeholder={t("custom_style_ph")}
            value={character.styleCustom}
            onChange={(e) => set({ styleCustom: e.target.value })}
          />
        </div>
      )}

      <div className="field">
        <Label>{t("cell_size")}</Label>
        <Select value={String(cellSize)} onValueChange={(v) => onCellSize(Number(v))}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {CELL_SIZES.map((n) => (
              <SelectItem key={n} value={String(n)}>
                {n} × {n} px
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="field">
        <Label>{t("frame_fit_mode")}</Label>
        <Select value={frameFitMode} onValueChange={(v) => onFrameFitMode(v as FrameFitMode)}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="contain">{t("frame_fit_contain")}</SelectItem>
            <SelectItem value="auto_expand">{t("frame_fit_auto_expand")}</SelectItem>
            <SelectItem value="crop">{t("frame_fit_crop")}</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <p className="hint">{t("char_hint")}</p>
      </div>
    </>
  );
}
