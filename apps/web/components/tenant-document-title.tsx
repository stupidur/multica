"use client";

import { useEffect, useState } from "react";
import { usePathname } from "next/navigation";
import { api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";

const DEFAULT_TITLE = "Multica — Project Management for Human + Agent Teams";
const TITLE_UPDATED_EVENT = "multica:tenant-title-updated";

export function notifyTenantTitleUpdated(title: string) {
  if (typeof window === "undefined") return;
  window.dispatchEvent(
    new CustomEvent(TITLE_UPDATED_EVENT, { detail: { title } }),
  );
}

export function TenantDocumentTitle() {
  const user = useAuthStore((s) => s.user);
  const pathname = usePathname();
  const [title, setTitle] = useState<string | null>(null);

  useEffect(() => {
    if (!user) {
      setTitle(null);
      return;
    }

    let cancelled = false;
    api
      .getTenantSettings()
      .then((settings) => {
        if (!cancelled) setTitle(settings.title?.trim() || null);
      })
      .catch(() => {
        if (!cancelled) setTitle(null);
      });

    return () => {
      cancelled = true;
    };
  }, [user?.id]);

  useEffect(() => {
    if (typeof window === "undefined") return;

    const onTitleUpdated = (event: Event) => {
      const nextTitle = (event as CustomEvent<{ title?: string }>).detail?.title;
      setTitle(nextTitle?.trim() || null);
    };
    window.addEventListener(TITLE_UPDATED_EVENT, onTitleUpdated);
    return () => window.removeEventListener(TITLE_UPDATED_EVENT, onTitleUpdated);
  }, []);

  useEffect(() => {
    document.title = title || DEFAULT_TITLE;
  }, [pathname, title]);

  return null;
}
