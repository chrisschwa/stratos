import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Camera, Plus, RefreshCw, Trash2 } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { timeAgo } from "@/lib/format"
import { useCloudList, useCloudScope, useProjectId } from "@/lib/hooks"
import type { CloudResource } from "@/lib/types"

// The API stores the live snapshot under data.volumeSnapshot (data.snapshot is not used).
function snap(r: CloudResource): Record<string, any> {
  return (r.data?.volumeSnapshot as Record<string, any>) ?? (r.data?.snapshot as Record<string, any>) ?? {}
}

export default function SnapshotsPage() {
  const pid = useProjectId()
  const scope = useCloudScope(pid)
  const qc = useQueryClient()
  const { data, isLoading, isError, error, refetch, isFetching } = useCloudList(pid, "VOLUME_SNAPSHOT")
  const volumes = useCloudList(pid, "VOLUME")

  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState("")
  const [volumeExtId, setVolumeExtId] = useState("")
  const [deleteTarget, setDeleteTarget] = useState<CloudResource | null>(null)

  const invalidate = () => void qc.invalidateQueries({ queryKey: ["cloud", pid, "VOLUME_SNAPSHOT"] })

  const create = useMutation({
    mutationFn: () =>
      apiFetch(`/project/${pid}/cloud`, {
        method: "POST",
        cloud: scope,
        body: { type: "VOLUME_SNAPSHOT", data: { name, externalVolumeId: volumeExtId } },
      }),
    onSuccess: () => {
      toast.success("Snapshot created")
      setCreateOpen(false)
      setName("")
      setVolumeExtId("")
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const del = useMutation({
    mutationFn: (r: CloudResource) =>
      apiFetch(`/project/${pid}/cloud/${r.id}`, { method: "DELETE", cloud: scope }),
    onSuccess: () => {
      toast.success("Snapshot deletion requested")
      setDeleteTarget(null)
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const volumeName = (extId?: string) => {
    if (!extId) return "—"
    const v = volumes.data?.find((r) => r.externalId === extId)
    return (v?.data?.volume?.name as string) ?? v?.name ?? extId
  }

  return (
    <>
      <PageHeader
        title="Volume snapshots"
        description="Point-in-time copies of your block-storage volumes."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
              <RefreshCw className={isFetching ? "size-4 animate-spin" : "size-4"} />
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create snapshot
            </Button>
          </>
        }
      />

      {isLoading ? (
        <Skeleton className="h-64" />
      ) : isError ? (
        <p className="rounded-md bg-muted p-4 text-sm text-muted-foreground">{(error as Error).message}</p>
      ) : !data?.length ? (
        <EmptyState
          icon={Camera}
          title="No snapshots yet"
          hint="Snapshot a volume to capture its state at a point in time."
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create snapshot
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
                <TableHead>Size</TableHead>
                <TableHead>Volume</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-10" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.map((r) => (
                <TableRow key={r.id}>
                  <TableCell className="font-medium">{snap(r).name || r.name || r.id}</TableCell>
                  <TableCell>
                    <StatusBadge status={(snap(r).status as string) ?? r.status} />
                  </TableCell>
                  <TableCell className="text-sm">{snap(r).size != null ? `${snap(r).size} GB` : "—"}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {volumeName(snap(r).volume_id as string)}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {timeAgo(r.info?.createdAt ?? r.createdAt)}
                  </TableCell>
                  <TableCell>
                    <Button variant="ghost" size="sm" onClick={() => setDeleteTarget(r)}>
                      <Trash2 className="size-4" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create snapshot</DialogTitle>
            <DialogDescription>Capture the current state of a volume.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="snap-name">Name</Label>
              <Input id="snap-name" value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <div className="grid gap-2">
              <Label>Volume</Label>
              <Select value={volumeExtId} onValueChange={setVolumeExtId}>
                <SelectTrigger>
                  <SelectValue placeholder={volumes.data?.length ? "Select a volume" : "No volumes available"} />
                </SelectTrigger>
                <SelectContent>
                  {(volumes.data ?? [])
                    .filter((v) => !!v.externalId)
                    .map((v) => (
                      <SelectItem key={v.id} value={v.externalId as string}>
                        {(v.data?.volume?.name as string) ?? v.name ?? v.externalId}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => create.mutate()} disabled={!name || !volumeExtId || create.isPending}>
              {create.isPending ? "Creating…" : "Create snapshot"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!deleteTarget} onOpenChange={(o) => !o && setDeleteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete snapshot</DialogTitle>
            <DialogDescription>
              This permanently deletes {deleteTarget ? snap(deleteTarget).name || deleteTarget.id : ""}. This cannot
              be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => deleteTarget && del.mutate(deleteTarget)}
              disabled={del.isPending}
            >
              {del.isPending ? "Deleting…" : "Delete snapshot"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
