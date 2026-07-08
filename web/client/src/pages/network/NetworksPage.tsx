import { useState } from "react"
import { Link } from "react-router-dom"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Network, Plus, RefreshCw, Trash2 } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { timeAgo } from "@/lib/format"
import { useCloudList, useCloudScope, useProject, useProjectId } from "@/lib/hooks"
import type { CloudResource } from "@/lib/types"

export function networkName(r: CloudResource): string {
  return (r.data?.networkName as string) || (r.data?.network?.name as string) || r.name || r.id
}
export function networkStatus(r: CloudResource): string | undefined {
  return (r.data?.network?.status as string) ?? r.status
}
// A private (own) network — not a shared or router:external cloud network. When the external-network
// picker is hidden (publicNetworksVisible=false), the Networks page + create-server network step show
// only these; external/shared pools stay in the FIP/router pickers.
export function isPrivateNetwork(r: CloudResource): boolean {
  const net = (r.data?.network ?? {}) as Record<string, unknown>
  return !net.shared && !net["router:external"]
}

export default function NetworksPage() {
  const pid = useProjectId()
  const scope = useCloudScope(pid)
  const qc = useQueryClient()
  const { data, isLoading, refetch, isFetching, error } = useCloudList(pid, "NETWORK")
  // Hidden external picker → show only the project's own private networks (drop shared/external infra).
  const netsVisible = useProject(pid).project?.publicNetworksVisible === true
  const rows = netsVisible ? (data ?? []) : (data ?? []).filter(isPrivateNetwork)
  const [createOpen, setCreateOpen] = useState(false)
  const [toDelete, setToDelete] = useState<CloudResource | null>(null)

  // create form
  const [name, setName] = useState("")
  const [withSubnet, setWithSubnet] = useState(true)
  const [cidr, setCidr] = useState("10.0.0.0/24")

  const invalidate = () => void qc.invalidateQueries({ queryKey: ["cloud", pid, "NETWORK"] })

  const create = useMutation({
    mutationFn: () =>
      apiFetch(`/project/${pid}/cloud`, {
        method: "POST",
        cloud: scope,
        body: {
          type: "NETWORK",
          data: withSubnet
            ? { name, defaultSubnet: true, cidr, enableDhcp: true, gateway: true }
            : { name },
        },
      }),
    onSuccess: () => {
      toast.success(`Network "${name}" created`)
      setCreateOpen(false)
      setName("")
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const del = useMutation({
    mutationFn: (r: CloudResource) =>
      apiFetch(`/project/${pid}/cloud/${r.id}`, { method: "DELETE", cloud: scope }),
    onSuccess: () => {
      toast.success("Network deletion requested")
      setToDelete(null)
      setTimeout(invalidate, 1500)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <>
      <PageHeader
        title="Networks"
        description="Private networks in this project."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
              <RefreshCw className={isFetching ? "size-4 animate-spin" : "size-4"} />
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create network
            </Button>
          </>
        }
      />

      {isLoading ? (
        <Skeleton className="h-64" />
      ) : error ? (
        <div className="rounded-lg border bg-muted/40 p-6 text-sm text-muted-foreground">{(error as Error).message}</div>
      ) : !rows.length ? (
        <EmptyState
          icon={Network}
          title="No networks yet"
          hint="Create a private network to connect your servers."
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create network
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
                <TableHead>Subnets</TableHead>
                <TableHead>Flags</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-10" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((r) => {
                const net = (r.data?.network ?? {}) as Record<string, unknown>
                const subnets = (net.subnets as string[] | undefined) ?? []
                return (
                  <TableRow key={r.id}>
                    <TableCell className="font-medium">
                      <Link className="hover:underline" to={`/p/${pid}/networks/${r.id}`}>
                        {networkName(r)}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={networkStatus(r)} />
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">{subnets.length}</TableCell>
                    <TableCell>
                      <div className="flex gap-1">
                        {net.shared ? <Badge variant="secondary">shared</Badge> : null}
                        {net["router:external"] ? <Badge variant="secondary">external</Badge> : null}
                      </div>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {timeAgo(r.info?.createdAt ?? r.createdAt)}
                    </TableCell>
                    <TableCell>
                      <Button variant="ghost" size="icon" onClick={() => setToDelete(r)} aria-label="Delete network">
                        <Trash2 className="size-4 text-muted-foreground" />
                      </Button>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </Card>
      )}

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create network</DialogTitle>
            <DialogDescription>A private network, optionally with an initial subnet.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-2">
            <div className="grid gap-2">
              <Label htmlFor="net-name">Name</Label>
              <Input id="net-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="my-network" />
            </div>
            <div className="flex items-center gap-2">
              <Checkbox id="net-subnet" checked={withSubnet} onCheckedChange={(v) => setWithSubnet(v === true)} />
              <Label htmlFor="net-subnet">Create a subnet</Label>
            </div>
            {withSubnet ? (
              <div className="grid gap-2">
                <Label htmlFor="net-cidr">Subnet CIDR</Label>
                <Input id="net-cidr" className="font-mono" value={cidr} onChange={(e) => setCidr(e.target.value)} />
              </div>
            ) : null}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => create.mutate()} disabled={!name || (withSubnet && !cidr) || create.isPending}>
              {create.isPending ? "Creating…" : "Create network"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!toDelete} onOpenChange={(o) => !o && setToDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete network</DialogTitle>
            <DialogDescription>
              Delete network "{toDelete ? networkName(toDelete) : ""}"? This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setToDelete(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={() => toDelete && del.mutate(toDelete)} disabled={del.isPending}>
              {del.isPending ? "Deleting…" : "Delete network"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
