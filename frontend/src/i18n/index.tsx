import { createContext, useContext, useEffect, useMemo, useState, ReactNode } from "react";
import en from "./ui.en";
import es from "./ui.es";
import ko from "./ui.ko";
import zh from "./ui.zh";

// 지원 언어 코드 (기본: 영어)
export type Lang = "en" | "es" | "ko" | "zh";

export const LANGUAGES: { code: Lang; label: string }[] = [
  { code: "en", label: "English" },
  { code: "es", label: "Español" },
  { code: "ko", label: "한국어" },
  { code: "zh", label: "中文" },
];

const DICTS: Record<Lang, Record<string, string>> = { en, es, ko, zh };
const STORAGE_KEY = "pp.lang";
const DEFAULT_LANG: Lang = "zh";

function readStoredLang(): Lang {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v && DICTS[v as Lang]) return v as Lang;
  } catch {
    // localStorage 접근 불가 시 기본값
  }
  return DEFAULT_LANG;
}

// {var} 플레이스홀더를 vars 값으로 치환
function interpolate(template: string, vars?: Record<string, string | number>): string {
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (_, k) => (k in vars ? String(vars[k]) : `{${k}}`));
}

export type TFunc = (key: string, vars?: Record<string, string | number>) => string;

interface I18nValue {
  lang: Lang;
  setLang: (l: Lang) => void;
  t: TFunc;
}

const I18nContext = createContext<I18nValue | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(readStoredLang);

  useEffect(() => {
    document.documentElement.lang = lang;
  }, [lang]);

  const value = useMemo<I18nValue>(() => {
    const setLang = (l: Lang) => {
      setLangState(l);
      try {
        localStorage.setItem(STORAGE_KEY, l);
      } catch {
        // 저장 실패는 무시 (메모리 상태는 유지됨)
      }
    };
    // 누락 키는 영어 → 키 순으로 폴백
    const t: TFunc = (key, vars) => {
      const raw = DICTS[lang]?.[key] ?? DICTS.en[key] ?? key;
      return interpolate(raw, vars);
    };
    return { lang, setLang, t };
  }, [lang]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nValue {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error("useI18n must be used within I18nProvider");
  return ctx;
}
