import { statusKind } from "@/lib/format"
import { cn } from "@/lib/utils"

export function StatusBadge({ status, className }: { status?: string; className?: string }) {
  const kind = statusKind(status)
  return (
    <span className={cn("inline-flex items-center gap-1.5 text-sm", className)}>
      <span className={cn("status-dot", `status-dot-${kind}`)} />
      <span className="capitalize">{(status ?? "unknown").toLowerCase().replace(/_/g, " ")}</span>
    </span>
  )
}
