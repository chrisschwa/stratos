import { useState } from "react"
import { Link } from "react-router-dom"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { MoreVertical, Play, Plus, RefreshCw, RotateCw, Server, Square, Trash2 } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { useCloudList, useCloudScope, useProjectId } from "@/lib/hooks"
import { timeAgo } from "@/lib/format"
import type { CloudResource } from "@/lib/types"

export function serverName(r: CloudResource): string {
  return (r.data?.server?.name as string) ?? r.name ?? r.id
}
export function serverStatus(r: CloudResource): string | undefined {
  return (r.data?.server?.status as string) ?? r.status
}
export function serverIPs(r: CloudResource): string[] {
  const addrs = r.data?.server?.addresses as Record<string, Array<{ addr?: string }>> | undefined
  if (!addrs) return []
  return Object.values(addrs)
    .flat()
    .map((a) => a?.addr)
    .filter((x): x is string => !!x)
}
export function serverFlavor(r: CloudResource): string {
  const f = r.data?.server?.flavor as Record<string, unknown> | undefined
  if (!f) return "—"
  const name = (f.original_name as string) ?? (f.name as string)
  const vcpus = f.vcpus as number | undefined
  const ram = f.ram as number | undefined
  if (name && vcpus && ram) return `${name} · ${vcpus} vCPU · ${Math.round(ram / 1024)} GB`
  return name ?? "—"
}

// Quick actions verified against internal/cloud/providers/write.go (TypeServer Action switch):
// START / STOP / SOFTREBOOT take no data; DELETE = DELETE /project/{pid}/cloud/{id} → 202.
type PendingRow = {
  id: string
  name: string
  action: "START" | "STOP" | "SOFTREBOOT" | "DELETE"
  label: string
}

const ACTION_VERB: Record<PendingRow["action"], string> = {
  START: "Start",
  STOP: "Stop",
  SOFTREBOOT: "Reboot",
  DELETE: "Delete",
}

export function ServersPage() {
  const pid = useProjectId()
  const scope = useCloudScope(pid)
  const qc = useQueryClient()
  const { data, isLoading, refetch, isFetching } = useCloudList(pid, "SERVER")
  const [pending, setPending] = useState<PendingRow | null>(null)

  const invalidate = () =>
    setTimeout(() => void qc.invalidateQueries({ queryKey: ["cloud", pid, "SERVER"] }), 1500)

  const act = useMutation({
    mutationFn: (p: { id: string; action: string }) =>
      apiFetch(`/project/${pid}/cloud/${p.id}/action`, {
        method: "POST",
        body: { action: p.action },
        cloud: scope,
      }),
    onSuccess: (_d, p) => {
      toast.success(`${ACTION_VERB[p.action as PendingRow["action"]] ?? p.action} requested`)
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const del = useMutation({
    mutationFn: (id: string) => apiFetch(`/project/${pid}/cloud/${id}`, { method: "DELETE", cloud: scope }),
    onSuccess: () => {
      toast.success("Server deletion requested")
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const runPending = () => {
    if (!pending) return
    if (pending.action === "DELETE") del.mutate(pending.id)
    else act.mutate({ id: pending.id, action: pending.action })
    setPending(null)
  }

  return (
    <>
      <PageHeader
        title="Servers"
        description="Virtual machines running in this project."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
              <RefreshCw className={isFetching ? "size-4 animate-spin" : "size-4"} />
            </Button>
            <Button size="sm" asChild>
              <Link to={`/p/${pid}/servers/new`}>
                <Plus className="size-4" /> Create server
              </Link>
            </Button>
          </>
        }
      />

      {isLoading ? (
        <Skeleton className="h-64" />
      ) : !data?.length ? (
        <EmptyState
          icon={Server}
          title="No servers yet"
          hint="Create your first virtual machine — it will boot in this project's region."
          action={
            <Button asChild>
              <Link to={`/p/${pid}/servers/new`}>
                <Plus className="size-4" /> Create server
              </Link>
            </Button>
          }
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Flavor</TableHead>
                <TableHead>IP addresses</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.map((r) => {
                const name = serverName(r)
                const status = serverStatus(r)
                return (
                  <TableRow key={r.id}>
                    <TableCell className="font-medium">
                      <Link className="hover:underline" to={`/p/${pid}/servers/${r.id}`}>
                        {name}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={status} />
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">{serverFlavor(r)}</TableCell>
                    <TableCell className="font-mono text-sm">{serverIPs(r).join(", ") || "—"}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {timeAgo(r.info?.createdAt ?? r.createdAt)}
                    </TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon" className="size-8" aria-label={`Actions for ${name}`}>
                            <MoreVertical className="size-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem
                            disabled={status === "ACTIVE"}
                            onClick={() => setPending({ id: r.id, name, action: "START", label: `start "${name}"` })}
                          >
                            <Play className="size-4" /> Start
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            disabled={status === "SHUTOFF"}
                            onClick={() => setPending({ id: r.id, name, action: "STOP", label: `stop "${name}"` })}
                          >
                            <Square className="size-4" /> Stop
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={() =>
                              setPending({ id: r.id, name, action: "SOFTREBOOT", label: `reboot "${name}"` })
                            }
                          >
                            <RotateCw className="size-4" /> Reboot
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            variant="destructive"
                            onClick={() =>
                              setPending({
                                id: r.id,
                                name,
                                action: "DELETE",
                                label: `delete "${name}" — this cannot be undone`,
                              })
                            }
                          >
                            <Trash2 className="size-4" /> Delete
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </Card>
      )}

      <Dialog open={!!pending} onOpenChange={(o) => !o && setPending(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{pending ? ACTION_VERB[pending.action] : ""} server</DialogTitle>
            <DialogDescription>Are you sure you want to {pending?.label}?</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPending(null)}>
              Cancel
            </Button>
            <Button
              variant={pending?.action === "DELETE" ? "destructive" : "default"}
              onClick={runPending}
            >
              {pending ? ACTION_VERB[pending.action] : "Confirm"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
