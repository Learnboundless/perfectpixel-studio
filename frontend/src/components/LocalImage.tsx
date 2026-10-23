import { ImgHTMLAttributes, useEffect, useRef, useState } from "react";
import { LoadAnimationVersionResource, LoadImageFull, LoadImageThumb } from "../../wailsjs/go/main/App";

const imageCache = new Map<string, string>();

const isBrowserImageURL = (value: string) =>
  value.startsWith("data:image/") || value.startsWith("blob:") || value.startsWith("http://") || value.startsWith("https://");

export async function loadImageReference(reference: string, maxDim = 0): Promise<string> {
  const value = String(reference ?? "").trim();
  if (!value || isBrowserImageURL(value)) return value;
  const key = `${maxDim}:${value}`;
  const cached = imageCache.get(key);
  if (cached) return cached;
  const loaded = value.startsWith("version-resource:")
    ? await LoadAnimationVersionResource(value, maxDim)
    : maxDim > 0
      ? await LoadImageThumb(value, maxDim)
      : await LoadImageFull(value);
  imageCache.set(key, loaded);
  return loaded;
}

type LocalImageProps = Omit<ImgHTMLAttributes<HTMLImageElement>, "src"> & {
  reference?: string;
  maxDim?: number;
};

export default function LocalImage({ reference = "", maxDim = 0, alt = "", ...props }: LocalImageProps) {
  const [src, setSrc] = useState(() => (isBrowserImageURL(reference) ? reference : ""));
  const imageRef = useRef<HTMLImageElement | null>(null);

  useEffect(() => {
    let cancelled = false;
    let observer: IntersectionObserver | null = null;
    setSrc(isBrowserImageURL(reference) ? reference : "");
    if (!reference || isBrowserImageURL(reference)) return;
    const load = () => {
      loadImageReference(reference, maxDim)
        .then((value) => {
          if (!cancelled) setSrc(value);
        })
        .catch(() => {
          if (!cancelled) setSrc("");
        });
    };
    if (typeof IntersectionObserver === "undefined" || !imageRef.current) {
      load();
    } else {
      observer = new IntersectionObserver((entries) => {
        if (!entries.some((entry) => entry.isIntersecting)) return;
        observer?.disconnect();
        observer = null;
        load();
      }, { rootMargin: "240px" });
      observer.observe(imageRef.current);
    }
    return () => {
      cancelled = true;
      observer?.disconnect();
    };
  }, [reference, maxDim]);

  return <img {...props} ref={imageRef} src={src} alt={alt} />;
}
