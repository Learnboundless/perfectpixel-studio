import { Clapperboard, Home, ImagePlus, PanelsTopLeft, Video } from "lucide-react";
import { AppModule } from "../../types";
import { useI18n } from "../../i18n";

interface Props {
  value: AppModule;
  onChange: (module: AppModule) => void;
}

const MODULES = [
  { id: "home" as const, icon: Home, label: "tool_nav_home" },
  { id: "animation" as const, icon: Clapperboard, label: "tool_nav_animation" },
  { id: "image" as const, icon: ImagePlus, label: "tool_nav_image" },
  { id: "video" as const, icon: Video, label: "tool_nav_video" },
  { id: "ui" as const, icon: PanelsTopLeft, label: "tool_nav_ui" },
];

export default function ModuleNav({ value, onChange }: Props) {
  const { t } = useI18n();

  return (
    <nav className="module-nav" aria-label={t("tool_nav_label")}>
      {MODULES.map(({ id, icon: Icon, label }) => (
        <button
          key={id}
          type="button"
          className={`module-nav-button${value === id ? " active" : ""}`}
          aria-current={value === id ? "page" : undefined}
          onClick={() => onChange(id)}
          title={t(label)}
        >
          <Icon size={14} aria-hidden="true" />
          <span>{t(label)}</span>
        </button>
      ))}
    </nav>
  );
}
