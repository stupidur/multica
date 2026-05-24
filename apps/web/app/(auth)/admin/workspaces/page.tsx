"use client";

import { type FormEvent, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import { useCreateWorkspace } from "@multica/core/workspace";
import { paths } from "@multica/core/paths";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@multica/ui/components/ui/card";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";

export default function TenantWorkspaceAdminPage() {
  const router = useRouter();
  const user = useAuthStore((s) => s.user);
  const isLoading = useAuthStore((s) => s.isLoading);
  const createWorkspace = useCreateWorkspace();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [visibility, setVisibility] = useState<"private" | "tenant">("tenant");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!isLoading && !user) router.replace(paths.login());
  }, [isLoading, user, router]);

  const { data: workspaces = [], isLoading: isLoadingWorkspaces, refetch } = useQuery({
    queryKey: ["lark", "admin", "workspaces"],
    queryFn: () => api.listTenantAdminWorkspaces(),
    enabled: !!user,
  });

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    try {
      await createWorkspace.mutateAsync({ name, slug, visibility });
      setName("");
      setSlug("");
      void refetch();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create workspace");
    }
  };

  if (isLoading || !user) return null;

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6 p-6">
      <Card>
        <CardHeader>
          <CardTitle>Workspace Management</CardTitle>
          <CardDescription>Create tenant-shared or private workspaces for your Lark tenant.</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="grid gap-4 md:grid-cols-3" onSubmit={onSubmit}>
            <div className="grid gap-2">
              <Label htmlFor="workspace-name">Name</Label>
              <Input id="workspace-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Design Ops" required />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="workspace-slug">Slug</Label>
              <Input id="workspace-slug" value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="design-ops" required />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="workspace-visibility">Visibility</Label>
              <select
                id="workspace-visibility"
                className="h-10 rounded-md border border-input bg-background px-3 text-sm"
                value={visibility}
                onChange={(e) => setVisibility(e.target.value as "private" | "tenant")}
              >
                <option value="tenant">Tenant shared</option>
                <option value="private">Private</option>
              </select>
            </div>
            <div className="md:col-span-3 flex items-center gap-3">
              <Button type="submit" disabled={createWorkspace.isPending}>Create Workspace</Button>
              {error ? <p className="text-sm text-destructive">{error}</p> : null}
            </div>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Tenant Workspaces</CardTitle>
          <CardDescription>All workspaces owned by the current Lark tenant.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {isLoadingWorkspaces ? <p className="text-sm text-muted-foreground">Loading...</p> : null}
          {!isLoadingWorkspaces && workspaces.length === 0 ? (
            <p className="text-sm text-muted-foreground">No workspaces yet.</p>
          ) : null}
          {workspaces.map((ws) => (
            <div key={ws.id} className="flex items-center justify-between rounded-lg border p-4">
              <div>
                <p className="font-medium">{ws.name}</p>
                <p className="text-sm text-muted-foreground">/{ws.slug} · {ws.visibility === "tenant" ? "Tenant shared" : "Private"}</p>
              </div>
              <Button variant="outline" onClick={() => router.push(paths.workspace(ws.slug).issues())}>Open</Button>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  );
}
