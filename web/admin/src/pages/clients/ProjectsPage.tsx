import { useState } from "react"
import { useNavigate } from "react-router-dom"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { FolderKanban, Pause, Play, RefreshCw } from "lucide-react"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { fmtDate } from "@/lib/format"
import { useAdminList } from "@/lib/hooks"

// GET /admin/project (clientarea_reads.go projectAdminList) — project doc shaped + joined
// `organization` + usedVcpus/usedRam/usedBlockStorage.
type Project = {
  id?: string
  name?: string
  status?: string
  organizationId?: string
  organization?: { name?: string }
  createdAt?: string
}

const LIST_PATH = "/admin/project"

// Confirmable row actions: POST /admin/project/{id}/{status} (ENABLED|DISABLED, projectmut.go
// projectUpdateStatus) and POST /admin/project/{id}/sync (projectmut.go projectSync).
type PendingAction = { project: Project; kind: "ENABLED" | "DISABLED" | "sync" }

const actionCopy: Record<PendingAction["kind"], { title: string; verb: string; hint: string }> = {
  ENABLED: {
    title: "Enable project",
    verb: "Enable",
    hint: "Unpauses the project's servers and re-enables it.",
  },
  DISABLED: {
    title: "Disable project",
    verb: "Disable",
    hint: "Pauses every server in the project before disabling it.",
  },
  sync: {
    title: "Sync project",
    verb: "Sync",
    hint: "Reconciles the cached cloud resources against OpenStack.",
  },
}

export default function ProjectsPage() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { data, isLoading, isFetching, error, refetch } = useAdminList<Project>(LIST_PATH)
  const [pending, setPending] = useState<PendingAction | null>(null)

  const projects = data?.data ?? []

  const runAction = useMutation({
    mutationFn: ({ project, kind }: PendingAction) =>
      apiFetch(
        kind === "sync" ? `/admin/project/${project.id}/sync` : `/admin/project/${project.id}/${kind}`,
        { method: "POST" },
      ),
    onSuccess: (_d, v) => {
      toast.success(v.kind === "sync" ? "Project synced" : `Project ${v.kind.toLowerCase()}`)
      setPending(null)
      qc.invalidateQueries({ queryKey: ["admin-list", LIST_PATH] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <>
      <PageHeader
        title="Projects"
        description="Client projects across every organization."
        actions={
          <Button variant="outline" size="sm" onClick={() => refetch()} disabled={isFetching}>
            <RefreshCw className={isFetching ? "animate-spin" : ""} />
            Refresh
          </Button>
        }
      />

      {isLoading ? (
        <Skeleton className="h-64" />
      ) : error ? (
        <div className="rounded-lg border bg-muted/40 p-6 text-sm text-muted-foreground">
          {(error as Error).message}
        </div>
      ) : projects.length === 0 ? (
        <EmptyState icon={FolderKanban} title="No projects yet" hint="Projects appear when clients create them." />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Organization</TableHead>
                <TableHead>ID</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-40 text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {projects.map((p) => {
                const enabled = (p.status ?? "").toUpperCase() === "ENABLED"
                return (
                  <TableRow
                    key={p.id}
                    className="cursor-pointer"
                    onClick={() => p.id && navigate(`/clients/projects/${p.id}`)}
                  >
                    <TableCell className="font-medium">{p.name ?? "—"}</TableCell>
                    <TableCell>
                      <StatusBadge status={p.status} />
                    </TableCell>
                    <TableCell>
                      {p.organization?.name ?? (
                        <span className="font-mono text-xs text-muted-foreground">{p.organizationId ?? "—"}</span>
                      )}
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{p.id ?? "—"}</TableCell>
                    <TableCell className="text-muted-foreground">{fmtDate(p.createdAt)}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={(e) => {
                            e.stopPropagation()
                            setPending({ project: p, kind: enabled ? "DISABLED" : "ENABLED" })
                          }}
                        >
                          {enabled ? <Pause /> : <Play />}
                          {enabled ? "Disable" : "Enable"}
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={(e) => {
                            e.stopPropagation()
                            setPending({ project: p, kind: "sync" })
                          }}
                        >
                          <RefreshCw />
                          Sync
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </Card>
      )}

      {/* Action confirm */}
      <Dialog open={!!pending} onOpenChange={(o) => !o && setPending(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{pending ? actionCopy[pending.kind].title : ""}</DialogTitle>
            <DialogDescription>
              {pending ? (
                <>
                  {actionCopy[pending.kind].hint} Project: <span className="font-medium">{pending.project.name}</span>
                </>
              ) : null}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPending(null)}>
              Cancel
            </Button>
            <Button
              variant={pending?.kind === "DISABLED" ? "destructive" : "default"}
              disabled={runAction.isPending}
              onClick={() => pending && runAction.mutate(pending)}
            >
              {runAction.isPending ? "Working…" : pending ? actionCopy[pending.kind].verb : ""}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
