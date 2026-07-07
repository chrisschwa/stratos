import { useState } from "react"
import { Link } from "react-router-dom"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Globe, Plus, RefreshCw, Trash2 } from "lucide-react"
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
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { timeAgo } from "@/lib/format"
import { useCloudList, useCloudScope, useProjectId } from "@/lib/hooks"
import type { CloudResource } from "@/lib/types"

function zoneDomain(r: CloudResource): string {
  return (r.data?.zone?.name as string) ?? (r.data?.name as string) ?? r.name ?? r.id
}
function zoneStatus(r: CloudResource): string | undefined {
  return (r.data?.zone?.status as string) ?? r.status
}
function zoneEmail(r: CloudResource): string {
  return (r.data?.zone?.email as string) ?? "—"
}
function zoneTtl(r: CloudResource): string {
  const ttl = r.data?.zone?.ttl as number | undefined
  return ttl != null ? String(ttl) : "—"
}

export default function DnsZonesPage() {
  const pid = useProjectId()
  const scope = useCloudScope(pid)
  const qc = useQueryClient()
  const { data, isLoading, refetch, isFetching } = useCloudList(pid, "DNS_ZONE")

  const [createOpen, setCreateOpen] = useState(false)
  const [domain, setDomain] = useState("")
  const [email, setEmail] = useState("")
  const [ttl, setTtl] = useState("")
  const [description, setDescription] = useState("")
  const [toDelete, setToDelete] = useState<CloudResource | null>(null)

  const invalidate = () => void qc.invalidateQueries({ queryKey: ["cloud", pid, "DNS_ZONE"] })

  const create = useMutation({
    mutationFn: () => {
      const data: Record<string, unknown> = { domain: domain.trim(), email: email.trim() }
      if (ttl.trim()) data.ttl = Number(ttl)
      if (description.trim()) data.description = description.trim()
      return apiFetch(`/project/${pid}/cloud`, {
        method: "POST",
        body: { type: "DNS_ZONE", data },
        cloud: scope,
      })
    },
    onSuccess: () => {
      toast.success(`Zone "${domain}" created`)
      setCreateOpen(false)
      setDomain("")
      setEmail("")
      setTtl("")
      setDescription("")
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const del = useMutation({
    mutationFn: (id: string) =>
      apiFetch(`/project/${pid}/cloud/${id}`, { method: "DELETE", cloud: scope }),
    onSuccess: () => {
      toast.success("Zone deletion requested")
      setToDelete(null)
      setTimeout(invalidate, 1500)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <>
      <PageHeader
        title="DNS zones"
        description="Authoritative DNS zones managed in this project."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
              <RefreshCw className={isFetching ? "size-4 animate-spin" : "size-4"} />
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create zone
            </Button>
          </>
        }
      />

      {isLoading ? (
        <Skeleton className="h-64" />
      ) : !data?.length ? (
        <EmptyState
          icon={Globe}
          title="No DNS zones yet"
          hint="Create a zone for your domain, then add record sets from its detail page."
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create zone
            </Button>
          }
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Domain</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Email</TableHead>
                <TableHead>TTL</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.map((r) => (
                <TableRow key={r.id}>
                  <TableCell className="font-medium">
                    <Link className="hover:underline" to={`/p/${pid}/dns/${r.id}`}>
                      {zoneDomain(r)}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={zoneStatus(r)} />
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{zoneEmail(r)}</TableCell>
                  <TableCell className="font-mono text-sm">{zoneTtl(r)}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {timeAgo(r.info?.createdAt ?? r.createdAt)}
                  </TableCell>
                  <TableCell>
                    <Button variant="ghost" size="sm" onClick={() => setToDelete(r)} aria-label="Delete zone">
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
            <DialogTitle>Create DNS zone</DialogTitle>
            <DialogDescription>The domain is normalized to a fully-qualified name (trailing dot added).</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-2">
            <div className="grid gap-2">
              <Label htmlFor="zone-domain">Domain</Label>
              <Input
                id="zone-domain"
                value={domain}
                onChange={(e) => setDomain(e.target.value)}
                placeholder="example.com"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="zone-email">Contact email</Label>
              <Input
                id="zone-email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="hostmaster@example.com"
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <Label htmlFor="zone-ttl">TTL (seconds, optional)</Label>
                <Input
                  id="zone-ttl"
                  type="number"
                  value={ttl}
                  onChange={(e) => setTtl(e.target.value)}
                  placeholder="3600"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="zone-desc">Description (optional)</Label>
                <Input
                  id="zone-desc"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Production zone"
                />
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => create.mutate()} disabled={!domain.trim() || !email.trim() || create.isPending}>
              {create.isPending ? "Creating…" : "Create zone"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!toDelete} onOpenChange={(o) => !o && setToDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete zone</DialogTitle>
            <DialogDescription>
              Delete "{toDelete ? zoneDomain(toDelete) : ""}" and all of its record sets? This cannot be undone.
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
