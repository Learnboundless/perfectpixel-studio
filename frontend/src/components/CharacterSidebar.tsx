import { useState } from "react";
import { ChevronLeft, ChevronRight, Plus, Trash2, UserRound, UsersRound } from "lucide-react";
import { CharacterLibraryItem } from "../types";
import { useI18n } from "../i18n";
import { Button } from "./ui/button";

interface IProps {
  items: CharacterLibraryItem[];
  activeId: string | null;
  busy: boolean;
  onSelect: (id: string) => void;
  onAdd: () => void;
  onDelete: (id: string) => void;
}

export default function CharacterSidebar({
  items,
  activeId,
  busy,
  onSelect,
  onAdd,
  onDelete,
}: IProps) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);

  return (
    <aside className={`character-sidebar${open ? " open" : ""}`} aria-label={t("character_library")}>
      <header className="character-sidebar-head">
        <span className="character-sidebar-title">
          <UsersRound size={16} />
          {open && <strong>{t("character_library")}</strong>}
        </span>
        <Button
          variant="ghost"
          size="icon"
          onClick={() => setOpen((value) => !value)}
          aria-expanded={open}
          title={open ? t("character_library_collapse") : t("character_library_expand")}
        >
          {open ? <ChevronLeft size={15} /> : <ChevronRight size={15} />}
        </Button>
      </header>

      <div className="character-sidebar-list" role="list">
        {items.map((item) => {
          const active = item.id === activeId;
          return (
            <button
              type="button"
              role="listitem"
              key={item.id}
              className={`character-sidebar-item${active ? " active" : ""}`}
              onClick={() => {
                if (busy) return;
                onSelect(item.id);
                setOpen(false);
              }}
              disabled={busy}
              title={item.name || t("unnamed_character")}
            >
              <span className="character-sidebar-thumb checker">
                {item.image ? <img src={item.image} alt="" className="pixelated" /> : <UserRound size={17} />}
              </span>
              {open && (
                <span className="character-sidebar-meta">
                  <strong>{item.name || t("unnamed_character")}</strong>
                  <small>{item.description || t("character_no_description")}</small>
                </span>
              )}
            </button>
          );
        })}
      </div>

      <footer className="character-sidebar-foot">
        <Button
          variant="ghost"
          size={open ? "sm" : "icon"}
          onClick={onAdd}
          disabled={busy}
          title={t("character_add")}
        >
          <Plus size={15} />
          {open && <span>{t("character_add")}</span>}
        </Button>
        {open && activeId && items.length > 1 && (
          <Button
            variant="ghost"
            size="icon"
            className="character-sidebar-delete"
            onClick={() => onDelete(activeId)}
            disabled={busy}
            title={t("character_delete")}
          >
            <Trash2 size={14} />
          </Button>
        )}
      </footer>
    </aside>
  );
}
