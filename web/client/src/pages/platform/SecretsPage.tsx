import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { KeyRound, Plus, RefreshCw, Trash2 } from "lucide-react"
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
import { Textarea } from "@/components/ui/textarea"
import { apiFetch } from "@/lib/api"
import { fmtDateTime, timeAgo } from "@/lib/format"
import { useCloudList, useCloudScope, useProjectId } from "@/lib/hooks"
import type { CloudResource } from "@/lib/types"

function secretName(r: CloudResource): string {
  return (r.data?.secret?.name as string) ?? r.name ?? r.id
}
function secretType(r: CloudResource): string {
  return (r.data?.secret?.secret_type as string) ?? "—"
}
function secretStatus(r: CloudResource): string | undefined {
  return (r.data?.secret?.status as string) ?? r.status
}
function secretExpiration(r: CloudResource): string | undefined {
  return r.data?.secret?.expiration as string | undefined
}

export default function SecretsPage() {
  const pid = useProjectId()
  const scope = useCloudScope(pid)
  const qc = useQueryClient()
  const { data, isLoading, refetch, isFetching } = useCloudList(pid, "BARBICAN_SECRET")

  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState("")
  const [type, setType] = useState("opaque")
  const [payload, setPayload] = useState("")
  const [toDelete, setToDelete] = useState<CloudResource | null>(null)

  const invalidate = () => void qc.invalidateQueries({ queryKey: ["cloud", pid, "BARBICAN_SECRET"] })

  const create = useMutation({
    mutationFn: () =>
      apiFetch(`/project/${pid}/cloud`, {
        method: "POST",
        body: {
          type: "BARBICAN_SECRET",
          data: { name, secretType: type, payload, payloadContentType: "text/plain" },
        },
        cloud: scope,
      }),
    onSuccess: () => {
      toast.success(`Secret "${name}" created`)
      setCreateOpen(false)
      setName("")
      setPayload("")
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const del = useMutation({
    mutationFn: (id: string) =>
      apiFetch(`/project/${pid}/cloud/${id}`, { method: "DELETE", cloud: scope }),
    onSuccess: () => {
      toast.success("Secret deletion requested")
      setToDelete(null)
      setTimeout(invalidate, 1500)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <>
      <PageHeader
        title="Secrets"
        description="Payloads stored in the key manager (Barbican)."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
              <RefreshCw className={isFetching ? "size-4 animate-spin" : "size-4"} />
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create secret
            </Button>
          </>
        }
      />

      {isLoading ? (
        <Skeleton className="h-64" />
      ) : !data?.length ? (
        <EmptyState
          icon={KeyRound}
          title="No secrets yet"
          hint="Store passphrases and other sensitive payloads in the key manager."
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create secret
            </Button>
          }
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Expiration</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.map((r) => (
                <TableRow key={r.id}>
                  <TableCell className="font-medium">{secretName(r)}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{secretType(r)}</TableCell>
                  <TableCell>
                    <StatusBadge status={secretStatus(r)} />
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {secretExpiration(r) ? fmtDateTime(secretExpiration(r)) : "Never"}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {timeAgo(r.info?.createdAt ?? r.createdAt)}
                  </TableCell>
                  <TableCell>
                    <Button variant="ghost" size="sm" onClick={() => setToDelete(r)} aria-label="Delete secret">
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
            <DialogTitle>Create secret</DialogTitle>
            <DialogDescription>The payload is stored encrypted in the key manager.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-2">
            <div className="grid gap-2">
              <Label htmlFor="secret-name">Name</Label>
              <Input id="secret-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="db-password" />
            </div>
            <div className="grid gap-2">
              <Label>Secret type</Label>
              <Select value={type} onValueChange={setType}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="opaque">Opaque</SelectItem>
                  <SelectItem value="passphrase">Passphrase</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="secret-payload">Payload</Label>
              <Textarea
                id="secret-payload"
                value={payload}
                onChange={(e) => setPayload(e.target.value)}
                placeholder="secret value"
                rows={4}
                className="font-mono text-xs"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => create.mutate()} disabled={!name.trim() || !payload || create.isPending}>
              {create.isPending ? "Creating…" : "Create secret"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!toDelete} onOpenChange={(o) => !o && setToDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete secret</DialogTitle>
            <DialogDescription>
              Delete "{toDelete ? secretName(toDelete) : ""}"? The stored payload is destroyed. This cannot be
              undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setToDelete(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => toDelete && del.mutate(toDelete.id)}
              disabled={del.isPending}
            >
              {del.isPending ? "Deleting…" : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
