"use client";

import { useState } from "react";
import { Save } from "lucide-react";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Button } from "@multica/ui/components/ui/button";
import { api } from "@multica/core/api";
import { toast } from "sonner";
import { useT } from "../../i18n";

export function PasswordTab() {
  const { t } = useT("settings");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [saving, setSaving] = useState(false);

  const mismatch =
    confirmPassword.length > 0 && newPassword !== confirmPassword;

  const handleSave = async () => {
    if (!newPassword || mismatch) return;
    setSaving(true);
    try {
      await api.updatePassword({
        current_password: currentPassword || undefined,
        new_password: newPassword,
      });
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      toast.success(t(($) => $.password.toast_updated));
    } catch (e) {
      toast.error(
        e instanceof Error ? e.message : t(($) => $.password.toast_failed),
      );
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-8">
      <section className="space-y-4">
        <h2 className="text-sm font-semibold">
          {t(($) => $.password.section_title)}
        </h2>
        <Card>
          <CardContent className="space-y-4">
            <p className="text-sm text-muted-foreground">
              {t(($) => $.password.description)}
            </p>
            <div>
              <Label className="text-xs text-muted-foreground">
                {t(($) => $.password.current_label)}
              </Label>
              <Input
                type="password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                className="mt-1"
                placeholder={t(($) => $.password.current_placeholder)}
              />
              <p className="mt-1 text-xs text-muted-foreground">
                {t(($) => $.password.current_hint)}
              </p>
            </div>
            <div>
              <Label className="text-xs text-muted-foreground">
                {t(($) => $.password.new_label)}
              </Label>
              <Input
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                className="mt-1"
                placeholder={t(($) => $.password.new_placeholder)}
              />
            </div>
            <div>
              <Label className="text-xs text-muted-foreground">
                {t(($) => $.password.confirm_label)}
              </Label>
              <Input
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                className="mt-1"
                placeholder={t(($) => $.password.confirm_placeholder)}
              />
              {mismatch ? (
                <p className="mt-1 text-xs text-destructive">
                  {t(($) => $.password.confirm_mismatch)}
                </p>
              ) : null}
            </div>
            <div className="flex items-center justify-end gap-2 pt-1">
              <Button
                size="sm"
                onClick={handleSave}
                disabled={saving || !newPassword || mismatch}
              >
                <Save className="h-3 w-3" />
                {saving
                  ? t(($) => $.password.saving)
                  : t(($) => $.password.save)}
              </Button>
            </div>
          </CardContent>
        </Card>
      </section>
    </div>
  );
}
