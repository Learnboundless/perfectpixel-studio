import { SlidersHorizontal, Sparkles } from "lucide-react";
import { useEffect, useState } from "react";
import { useI18n } from "../../i18n";
import { StudioMode } from "../../types";

export function useStudioMode(key: string): [StudioMode, (mode: StudioMode) => void] {
  const storageKey = `pp.toolchain.mode.${key}`;
  const [mode, setModeState] = useState<StudioMode>(() => {
    try {
      return localStorage.getItem(storageKey) === "expert" ? "expert" : "simple";
    } catch {
      return "simple";
    }
  });

  useEffect(() => {
    try {
      localStorage.setItem(storageKey, mode);
    } catch {
      // The in-memory preference still works when storage is unavailable.
    }
  }, [mode, storageKey]);

  return [mode, setModeState];
}

export function StudioModeToggle({ value, onChange }: { value: StudioMode; onChange: (value: StudioMode) => void }) {
  const { t } = useI18n();
  return (
    <div className="studio-mode-toggle" role="group" aria-label={t("tool_mode_label")}>
      <button type="button" className={value === "simple" ? "active" : ""} onClick={() => onChange("simple")}>
        <Sparkles size={12} aria-hidden="true" />
        {t("tool_mode_simple")}
      </button>
      <button type="button" className={value === "expert" ? "active" : ""} onClick={() => onChange("expert")}>
        <SlidersHorizontal size={12} aria-hidden="true" />
        {t("tool_mode_expert")}
      </button>
    </div>
  );
}

export function FieldLabel({ children, optional }: { children: React.ReactNode; optional?: boolean }) {
  const { t } = useI18n();
  return (
    <label className="tool-field-label">
      <span>{children}</span>
      {optional && <small>{t("tool_optional")}</small>}
    </label>
  );
}
