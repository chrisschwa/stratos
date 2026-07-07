import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { FolderTree, Plus, RefreshCw, Settings2, Trash2 } from "lucide-react"
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
import {
  Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle,
} from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { apiFetch, type CloudScope } from "@/lib/api"
import { timeAgo } from "@/lib/format"
import { useCloudList, useCloudScope, useProjectId } from "@/lib/hooks"
import type { CloudResource } from "@/lib/types"

function share(r: CloudResource): Record<string, any> {
  return (r.data?.share as Record<string, any>) ?? {}
}
function shareProtocol(r: CloudResource): string {
  return (share(r).share_proto as string) ?? (share(r).shareProto as string) ?? "—"
}
function shareName(r: CloudResource): string {
  return (share(r).name as string) || r.name || r.id
}

// Manila access rules come back verbatim (gophercloud JSON, snake_case).
type AccessRule = {
  id?: string
  access_type?: string
  access_to?: string
  access_level?: string
  state?: string
}

export default function SharesPage() {
  const pid = useProjectId()
  const scope = useCloudScope(pid)
  const qc = useQueryClient()
  const { data, isLoading, isError, error, refetch, isFetching } = useCloudList(pid, "SHARE")

  const [createOpen, setCreateOpen] = useState(false)
  const [form, setForm] = useState({ name: "", protocol: "NFS", size: "1" })
  const [deleteTarget, setDeleteTarget] = useState<CloudResource | null>(null)
  const [manageFor, setManageFor] = useState<CloudResource | null>(null)

  const invalidate = () => void qc.invalidateQueries({ queryKey: ["cloud", pid, "SHARE"] })

  const create = useMutation({
    mutationFn: () =>
      apiFetch(`/project/${pid}/cloud`, {
        method: "POST",
        cloud: scope,
        body: {
          type: "SHARE",
          data: { name: form.name, protocol: form.protocol, size: Number(form.size) },
        },
      }),
    onSuccess: () => {
      toast.success("Share created")
      setCreateOpen(false)
      setForm({ name: "", protocol: "NFS", size: "1" })
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const del = useMutation({
    mutationFn: (r: CloudResource) =>
      apiFetch(`/project/${pid}/cloud/${r.id}`, { method: "DELETE", cloud: scope }),
    onSuccess: () => {
      toast.success("Share deletion requested")
      setDeleteTarget(null)
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <>
      <PageHeader
        title="Shared file systems"
        description="Network file shares (NFS/CIFS) in this project."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
              <RefreshCw className={isFetching ? "size-4 animate-spin" : "size-4"} />
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create share
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
          icon={FolderTree}
          title="No shares yet"
          hint="Create a network file share your servers can mount."
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create share
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
                <TableHead>Protocol</TableHead>
                <TableHead>Size</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.map((r) => (
                <TableRow key={r.id}>
                  <TableCell className="font-medium">
                    <button className="hover:underline" onClick={() => setManageFor(r)}>
                      {shareName(r)}
                    </button>
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={(share(r).status as string) ?? r.status} />
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{shareProtocol(r)}</TableCell>
                  <TableCell className="text-sm">
                    {share(r).size != null ? `${share(r).size} GB` : "—"}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {timeAgo(r.info?.createdAt ?? r.createdAt)}
                  </TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-1">
                      <Button variant="ghost" size="sm" onClick={() => setManageFor(r)} aria-label="Manage share">
                        <Settings2 className="size-4" />
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => setDeleteTarget(r)} aria-label="Delete share">
                        <Trash2 className="size-4" />
                      </Button>
                    </div>
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
            <DialogTitle>Create share</DialogTitle>
            <DialogDescription>A new network file share in this project's region.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="share-name">Name</Label>
              <Input id="share-name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            </div>
            <div className="grid gap-2">
              <Label>Protocol</Label>
              <Select value={form.protocol} onValueChange={(v) => setForm({ ...form, protocol: v })}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="NFS">NFS</SelectItem>
                  <SelectItem value="CIFS">CIFS</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="share-size">Size (GB)</Label>
              <Input
                id="share-size"
                type="number"
                min={1}
                value={form.size}
                onChange={(e) => setForm({ ...form, size: e.target.value })}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => create.mutate()} disabled={!form.name || !Number(form.size) || create.isPending}>
              {create.isPending ? "Creating…" : "Create share"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!deleteTarget} onOpenChange={(o) => !o && setDeleteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete share</DialogTitle>
            <DialogDescription>
              This permanently deletes {deleteTarget ? shareName(deleteTarget) : ""} and its
              data. This cannot be undone.
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
              {del.isPending ? "Deleting…" : "Delete share"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {manageFor && (
        <ShareManageSheet
          pid={pid}
          scope={scope}
          res={manageFor}
          onClose={() => setManageFor(null)}
          onResized={invalidate}
        />
      )}
    </>
  )
}

// Per-share manage view: access rules (LIST_ACCESS/GRANT_ACCESS/REVOKE_ACCESS) + resize
// (EXTEND_SHARE/SHRINK_SHARE) — Go cloud_writes.go clusterAction TypeShare.
function ShareManageSheet({
  pid, scope, res, onClose, onResized,
}: {
  pid: string
  scope: CloudScope | undefined
  res: CloudResource
  onClose: () => void
  onResized: () => void
}) {
  const qc = useQueryClient()
  const shareId = res.id
  const currentSize = Number(share(res).size) || 0

  const act = (action: string, data?: Record<string, any>) =>
    apiFetch<{ result?: any }>(`/project/${pid}/cloud/${shareId}/action`, {
      method: "POST",
      body: data ? { action, data } : { action },
      cloud: scope,
    })

  const access = useQuery({
    queryKey: ["share-access", pid, shareId],
    queryFn: () => act("LIST_ACCESS"),
    enabled: !!scope,
  })
  const rules: AccessRule[] = access.data?.result ?? []

  const [grantOpen, setGrantOpen] = useState(false)
  const [grantForm, setGrantForm] = useState({ accessType: "ip", accessTo: "", accessLevel: "rw" })
  const [revokeTarget, setRevokeTarget] = useState<AccessRule | null>(null)
  const [extendSize, setExtendSize] = useState(String(currentSize + 1))
  const [shrinkSize, setShrinkSize] = useState(currentSize > 1 ? String(currentSize - 1) : "1")

  const invalidateAccess = () => void qc.invalidateQueries({ queryKey: ["share-access", pid, shareId] })

  const grant = useMutation({
    mutationFn: () =>
      act("GRANT_ACCESS", {
        accessType: grantForm.accessType,
        accessTo: grantForm.accessTo.trim(),
        accessLevel: grantForm.accessLevel,
      }),
    onSuccess: () => {
      toast.success("Access granted")
      setGrantOpen(false)
      setGrantForm({ accessType: "ip", accessTo: "", accessLevel: "rw" })
      invalidateAccess()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const revoke = useMutation({
    mutationFn: (rule: AccessRule) => act("REVOKE_ACCESS", { ruleId: rule.id }),
    onSuccess: () => {
      toast.success("Access revoked")
      setRevokeTarget(null)
      invalidateAccess()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const resize = useMutation({
    mutationFn: ({ action, size }: { action: "EXTEND_SHARE" | "SHRINK_SHARE"; size: number }) =>
      act(action, { size }),
    onSuccess: (_d, { action, size }) => {
      toast.success(
        `${action === "EXTEND_SHARE" ? "Extend" : "Shrink"} to ${size} GB requested — the size updates when Manila finishes`,
      )
      onResized()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const extendN = Number(extendSize)
  const extendValid = Number.isInteger(extendN) && extendN > currentSize
  const shrinkN = Number(shrinkSize)
  const shrinkValid = Number.isInteger(shrinkN) && shrinkN >= 1 && shrinkN < currentSize

  return (
    <Sheet open onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="overflow-y-auto sm:max-w-xl">
        <SheetHeader>
          <SheetTitle>{shareName(res)}</SheetTitle>
          <SheetDescription>
            {shareProtocol(res)} · {currentSize} GB — manage access rules and size.
          </SheetDescription>
        </SheetHeader>

        <div className="px-4 pb-6">
          <Tabs defaultValue="access">
            <TabsList>
              <TabsTrigger value="access">Access rules</TabsTrigger>
              <TabsTrigger value="resize">Resize</TabsTrigger>
            </TabsList>

            <TabsContent value="access" className="mt-4 space-y-3">
              <div className="flex justify-end">
                <Button size="sm" onClick={() => setGrantOpen(true)}>
                  <Plus className="size-4" /> Grant access
                </Button>
              </div>
              {access.isLoading ? (
                <Skeleton className="h-32" />
              ) : access.isError ? (
                <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">
                  {(access.error as Error).message}
                </p>
              ) : !rules.length ? (
                <p className="py-6 text-center text-sm text-muted-foreground">
                  No access rules — nothing can mount this share yet.
                </p>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Type</TableHead>
                      <TableHead>Access to</TableHead>
                      <TableHead>Level</TableHead>
                      <TableHead>State</TableHead>
                      <TableHead className="w-10" />
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {rules.map((rule, i) => (
                      <TableRow key={rule.id ?? i}>
                        <TableCell className="text-sm">{rule.access_type ?? "—"}</TableCell>
                        <TableCell className="font-mono text-sm">{rule.access_to ?? "—"}</TableCell>
                        <TableCell className="text-sm uppercase">{rule.access_level ?? "—"}</TableCell>
                        <TableCell>
                          <StatusBadge status={rule.state} />
                        </TableCell>
                        <TableCell>
                          <Button
                            variant="ghost"
                            size="sm"
                            aria-label="Revoke access"
                            onClick={() => setRevokeTarget(rule)}
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </TabsContent>

            <TabsContent value="resize" className="mt-4 space-y-6">
              <div className="space-y-2">
                <Label htmlFor="extend-size">Extend</Label>
                <div className="flex gap-2">
                  <Input
                    id="extend-size"
                    type="number"
                    min={currentSize + 1}
                    value={extendSize}
                    onChange={(e) => setExtendSize(e.target.value)}
                    className="max-w-32"
                  />
                  <Button
                    onClick={() => resize.mutate({ action: "EXTEND_SHARE", size: extendN })}
                    disabled={!extendValid || resize.isPending}
                  >
                    {resize.isPending ? "Working…" : "Extend share"}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  New size in GB — must be larger than the current {currentSize} GB.
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="shrink-size">Shrink</Label>
                <div className="flex gap-2">
                  <Input
                    id="shrink-size"
                    type="number"
                    min={1}
                    max={Math.max(currentSize - 1, 1)}
                    value={shrinkSize}
                    onChange={(e) => setShrinkSize(e.target.value)}
                    className="max-w-32"
                  />
                  <Button
                    variant="outline"
                    onClick={() => resize.mutate({ action: "SHRINK_SHARE", size: shrinkN })}
                    disabled={!shrinkValid || resize.isPending}
                  >
                    {resize.isPending ? "Working…" : "Shrink share"}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  New size in GB — must be smaller than the current {currentSize} GB and no smaller than the
                  space already used, or Manila rejects the shrink.
                </p>
              </div>
            </TabsContent>
          </Tabs>
        </div>

        {/* Grant access */}
        <Dialog open={grantOpen} onOpenChange={setGrantOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Grant access</DialogTitle>
              <DialogDescription>Allow a client to mount this share.</DialogDescription>
            </DialogHeader>
            <div className="grid gap-4 py-2">
              <div className="grid grid-cols-2 gap-4">
                <div className="grid gap-2">
                  <Label>Type</Label>
                  <Select
                    value={grantForm.accessType}
                    onValueChange={(v) => setGrantForm({ ...grantForm, accessType: v })}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="ip">IP</SelectItem>
                      <SelectItem value="user">User</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="grid gap-2">
                  <Label>Level</Label>
                  <Select
                    value={grantForm.accessLevel}
                    onValueChange={(v) => setGrantForm({ ...grantForm, accessLevel: v })}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="rw">Read-write</SelectItem>
                      <SelectItem value="ro">Read-only</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="grant-to">Access to</Label>
                <Input
                  id="grant-to"
                  value={grantForm.accessTo}
                  onChange={(e) => setGrantForm({ ...grantForm, accessTo: e.target.value })}
                  placeholder={grantForm.accessType === "ip" ? "10.0.0.0/24" : "username"}
                  className="font-mono"
                />
                <p className="text-xs text-muted-foreground">
                  {grantForm.accessType === "ip"
                    ? "An IP address or CIDR block allowed to mount the share."
                    : "The user or group name allowed to mount the share."}
                </p>
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setGrantOpen(false)}>
                Cancel
              </Button>
              <Button onClick={() => grant.mutate()} disabled={!grantForm.accessTo.trim() || grant.isPending}>
                {grant.isPending ? "Granting…" : "Grant access"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Revoke confirm */}
        <Dialog open={!!revokeTarget} onOpenChange={(o) => !o && setRevokeTarget(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Revoke access</DialogTitle>
              <DialogDescription>
                Revoke {revokeTarget?.access_type} access for "{revokeTarget?.access_to}"? Clients using this
                rule lose access to the share.
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setRevokeTarget(null)}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={() => revokeTarget && revoke.mutate(revokeTarget)}
                disabled={revoke.isPending}
              >
                {revoke.isPending ? "Revoking…" : "Revoke"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </SheetContent>
    </Sheet>
  )
}
