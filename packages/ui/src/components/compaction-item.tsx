"use client"

import { memo } from "react"
import { Scissors } from "lucide-react"
import type { Message } from "@multica/store"

function formatTokens(n: number): string {
  if (n >= 1000) return `~${(n / 1000).toFixed(1)}k`
  return `${n}`
}

interface CompactionItemProps {
  message: Message
}

export const CompactionItem = memo(function CompactionItem({ message }: CompactionItemProps) {
  const info = message.compaction
  if (!info) return null

  const label = info.reason === "summary" ? "Context summarized" : "Context compacted"
  const removed = `${info.removed} messages removed`
  const tokens = info.tokensRemoved != null
    ? `, ${formatTokens(info.tokensRemoved)} tokens freed`
    : ""

  return (
    <div className="py-0.5 px-2.5 text-sm text-muted-foreground">
      <div className="flex items-center gap-1.5 px-2.5 py-1">
        {/* Status dot */}
        <span className="size-1.5 rounded-full shrink-0 bg-muted-foreground/40" />

        {/* Icon */}
        <Scissors className="size-3.5 shrink-0" />

        {/* Label */}
        <span className="font-medium shrink-0">{label}</span>

        {/* Stats */}
        <span className="ml-auto text-xs text-muted-foreground/60 shrink-0">
          {removed}{tokens}
        </span>
      </div>
    </div>
  )
})
