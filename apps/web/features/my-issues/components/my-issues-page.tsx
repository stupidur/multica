"use client";

import { useMemo } from "react";
import Link from "next/link";
import { ChevronRight, User, Bot, SquarePen, ListTodo } from "lucide-react";
import { Accordion } from "@base-ui/react/accordion";
import type { Issue } from "@/shared/types";
import { useAuthStore } from "@/features/auth";
import { useWorkspaceStore } from "@/features/workspace";
import { useIssueStore } from "@/features/issues/store";
import { WorkspaceAvatar } from "@/features/workspace";
import { StatusIcon } from "@/features/issues/components/status-icon";
import { PriorityIcon } from "@/features/issues/components/priority-icon";
import { ActorAvatar } from "@/components/common/actor-avatar";
import { Skeleton } from "@/components/ui/skeleton";

interface GroupConfig {
  key: string;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
}

const GROUPS: GroupConfig[] = [
  { key: "assigned_to_me", label: "Assigned to me", icon: User },
  { key: "assigned_to_my_agents", label: "Assigned to my agents", icon: Bot },
  { key: "created_by_me", label: "Created by me", icon: SquarePen },
];

function formatDate(date: string): string {
  return new Date(date).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
  });
}

function IssueRow({ issue }: { issue: Issue }) {
  return (
    <Link
      href={`/issues/${issue.id}`}
      className="flex h-9 items-center gap-2 px-4 text-sm transition-colors hover:bg-accent/50"
    >
      <PriorityIcon priority={issue.priority} className="shrink-0" />
      <StatusIcon status={issue.status} className="h-3.5 w-3.5 shrink-0" />
      <span className="w-16 shrink-0 text-xs text-muted-foreground">
        {issue.identifier}
      </span>
      <span className="min-w-0 flex-1 truncate">{issue.title}</span>
      {issue.due_date && (
        <span className="shrink-0 text-xs text-muted-foreground">
          {formatDate(issue.due_date)}
        </span>
      )}
      {issue.assignee_type && issue.assignee_id && (
        <ActorAvatar
          actorType={issue.assignee_type}
          actorId={issue.assignee_id}
          size={20}
        />
      )}
    </Link>
  );
}

export function MyIssuesPage() {
  const user = useAuthStore((s) => s.user);
  const workspace = useWorkspaceStore((s) => s.workspace);
  const agents = useWorkspaceStore((s) => s.agents);
  const allIssues = useIssueStore((s) => s.issues);
  const loading = useIssueStore((s) => s.loading);

  const myAgentIds = useMemo(() => {
    if (!user) return new Set<string>();
    return new Set(agents.filter((a) => a.owner_id === user.id).map((a) => a.id));
  }, [agents, user]);

  const grouped = useMemo(() => {
    if (!user) return new Map<string, Issue[]>();

    const assignedToMe = allIssues.filter(
      (i) => i.assignee_type === "member" && i.assignee_id === user.id,
    );
    const assignedToMyAgents = allIssues.filter(
      (i) => i.assignee_type === "agent" && i.assignee_id && myAgentIds.has(i.assignee_id),
    );
    const createdByMe = allIssues.filter(
      (i) => i.creator_type === "member" && i.creator_id === user.id,
    );

    const map = new Map<string, Issue[]>();
    map.set("assigned_to_me", assignedToMe);
    map.set("assigned_to_my_agents", assignedToMyAgents);
    map.set("created_by_me", createdByMe);
    return map;
  }, [allIssues, user, myAgentIds]);

  if (loading) {
    return (
      <div className="flex flex-1 min-h-0 flex-col">
        <div className="flex h-12 shrink-0 items-center gap-2 border-b px-4">
          <Skeleton className="h-5 w-5 rounded" />
          <Skeleton className="h-4 w-32" />
        </div>
        <div className="flex-1 p-4 space-y-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="space-y-2">
              <Skeleton className="h-8 w-48" />
              <Skeleton className="h-9 w-full" />
              <Skeleton className="h-9 w-full" />
            </div>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-1 min-h-0 flex-col">
      {/* Header: Workspace breadcrumb */}
      <div className="flex h-12 shrink-0 items-center gap-1.5 border-b px-4">
        <WorkspaceAvatar name={workspace?.name ?? "W"} size="sm" />
        <span className="text-sm text-muted-foreground">
          {workspace?.name ?? "Workspace"}
        </span>
        <ChevronRight className="h-3 w-3 text-muted-foreground" />
        <span className="text-sm font-medium">My Issues</span>
      </div>

      {/* Content */}
      <div className="flex-1 min-h-0 overflow-y-auto p-2">
        <Accordion.Root
          multiple
          className="space-y-1"
          defaultValue={GROUPS.map((g) => g.key)}
        >
          {GROUPS.map((group) => {
            const issues = grouped.get(group.key) ?? [];
            const Icon = group.icon;

            return (
              <Accordion.Item key={group.key} value={group.key}>
                <Accordion.Header className="flex h-10 items-center rounded-lg bg-muted/40 transition-colors hover:bg-accent/30">
                  <Accordion.Trigger className="group/trigger flex flex-1 items-center gap-2 px-3 h-full text-left outline-none">
                    <ChevronRight className="size-3.5 shrink-0 text-muted-foreground transition-transform group-aria-expanded/trigger:rotate-90" />
                    <Icon className="size-3.5 text-muted-foreground" />
                    <span className="text-sm font-medium">
                      {group.label}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {issues.length}
                    </span>
                  </Accordion.Trigger>
                </Accordion.Header>
                <Accordion.Panel className="pt-1">
                  {issues.length > 0 ? (
                    issues.map((issue) => (
                      <IssueRow key={issue.id} issue={issue} />
                    ))
                  ) : (
                    <p className="py-6 text-center text-xs text-muted-foreground">
                      No issues
                    </p>
                  )}
                </Accordion.Panel>
              </Accordion.Item>
            );
          })}
        </Accordion.Root>
      </div>
    </div>
  );
}
