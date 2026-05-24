"use client";

import { useEffect, useState } from "react";
import { Save } from "lucide-react";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import { useT } from "../../i18n";

const TITLE_UPDATED_EVENT = "multica:tenant-title-updated";

function notifyTenantTitleUpdated(title: string) {
  if (typeof window === "undefined") return;
  window.dispatchEvent(
    new CustomEvent(TITLE_UPDATED_EVENT, { detail: { title } }),
  );
}

export function SystemTab() {
  const { t } = useT("settings");

  const [title, setTitle] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [role, setRole] = useState<string | null>(null);

  useEffect(() => {
    Promise.all([api.getTenantSettings(), api.getTenantRole()])
      .then(([settings, roleResp]) => {
        setTitle(settings.title ?? "");
        setRole(roleResp.role);
      })
      .catch(() => {
        setRole(null);
      })
      .finally(() => setLoading(false));
  }, []);

  const handleSave = async () => {
    try {
      setSaving(true);
      const result = await api.patchTenantSettings(title);
      setTitle(result.title ?? "");
      notifyTenantTitleUpdated(result.title ?? "");
      toast.success(t(($) => $.page.system_saved ?? "System settings saved"));
    } catch {
      toast.error(t(($) => $.page.system_save_failed ?? "Failed to save"));
    } finally {
      setSaving(false);
    }
  };

  const isAdmin = role === "owner" || role === "admin";

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h2 className="text-lg font-semibold">
          {t(($) => $.page.tabs.system)}
        </h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t(
            ($) =>
              $.page.system_description ??
              "Configure system-level settings for this workspace.",
          )}
        </p>
      </div>

      <Card>
        <CardContent className="pt-6">
          <div className="flex flex-col gap-4 max-w-md">
            <div className="flex flex-col gap-2">
              <Label htmlFor="system-title">
                {t(($) => $.page.system_title_label ?? "Page Title")}
              </Label>
              <Input
                id="system-title"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder={t(
                  ($) => $.page.system_title_placeholder ?? "Enter page title",
                )}
                disabled={loading || !isAdmin}
              />
              <p className="text-xs text-muted-foreground">
                {t(
                  ($) =>
                    $.page.system_title_hint ??
                    "This title will be displayed in the browser tab.",
                )}
              </p>
            </div>

            {isAdmin ? (
              <div className="flex justify-end">
                <Button onClick={handleSave} disabled={saving}>
                  <Save className="h-4 w-4" />
                  {t(($) => $.page.save ?? "Save")}
                </Button>
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                {t(
                  ($) =>
                    $.page.system_admin_only ??
                    "Only admins can modify system settings.",
                )}
              </p>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
