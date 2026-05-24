"use client";

import { useEffect, useState } from "react";
import { usePathname } from "next/navigation";
import { api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";

const DEFAULT_TITLE = "Multica — Project Management for Human + Agent Teams";
const TITLE_UPDATED_EVENT = "multica:tenant-title-updated";
const TITLE_CACHE_KEY = "multica_tenant_document_title";

function cachedTitle() {
  if (typeof window === "undefined") return null;
  return window.localStorage.getItem(TITLE_CACHE_KEY)?.trim() || null;
}

function persistTitle(title: string | null) {
  if (typeof window === "undefined") return;
  if (title) {
    window.localStorage.setItem(TITLE_CACHE_KEY, title);
  } else {
    window.localStorage.removeItem(TITLE_CACHE_KEY);
  }
}

export function notifyTenantTitleUpdated(title: string) {
  if (typeof window === "undefined") return;
  window.dispatchEvent(
    new CustomEvent(TITLE_UPDATED_EVENT, { detail: { title } }),
  );
}

export function TenantDocumentTitle() {
  const user = useAuthStore((s) => s.user);
  const pathname = usePathname();
  const [title, setTitle] = useState<string | null>(() => cachedTitle());

  useEffect(() => {
    if (!user) {
      setTitle(null);
      return;
    }

    let cancelled = false;
    api
      .getTenantSettings()
      .then((settings) => {
        if (cancelled) return;
        const nextTitle = settings.title?.trim() || null;
        persistTitle(nextTitle);
        setTitle(nextTitle);
      })
      .catch(() => {
        if (!cancelled) setTitle(cachedTitle());
      });

    return () => {
      cancelled = true;
    };
  }, [user?.id]);

  useEffect(() => {
    if (typeof window === "undefined") return;

    const onTitleUpdated = (event: Event) => {
      const nextTitle = (event as CustomEvent<{ title?: string }>).detail?.title;
      const trimmed = nextTitle?.trim() || null;
      persistTitle(trimmed);
      setTitle(trimmed);
    };
    window.addEventListener(TITLE_UPDATED_EVENT, onTitleUpdated);
    return () => window.removeEventListener(TITLE_UPDATED_EVENT, onTitleUpdated);
  }, []);

  useEffect(() => {
    const desiredTitle = title || DEFAULT_TITLE;
    const applyTitle = () => {
      if (document.title !== desiredTitle) document.title = desiredTitle;
    };

    applyTitle();

    const observer = new MutationObserver(applyTitle);
    const titleElement = document.querySelector("title");
    observer.observe(document.head, { childList: true, subtree: true });
    if (titleElement) observer.observe(titleElement, { childList: true });

    return () => observer.disconnect();
  }, [pathname, title]);

  return null;
}
