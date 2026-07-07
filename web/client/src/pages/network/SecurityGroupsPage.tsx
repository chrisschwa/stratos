import { useState } from "react"
import { Link } from "react-router-dom"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Plus, RefreshCw, Shield, Trash2 } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
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

function sgName(r: CloudResource): string {
  return (r.data?.securityGroup?.name as string) ?? r.name ?? r.id
}
function sgDescription(r: CloudResource): string {
  return (r.data?.securityGroup?.description as string) ?? ""
}
function sgRuleCount(r: CloudResource): number | undefined {
  const rules = r.data?.securityGroup?.security_group_rules as unknown[] | undefined
  return rules?.length
}

export default function SecurityGroupsPage() {
  const pid = useProjectId()
  const scope = useCloudScope(pid)
  const qc = useQueryClient()
  const { data, isLoading, refetch, isFetching } = useCloudList(pid, "SECURITY_GROUP")

  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [toDelete, setToDelete] = useState<CloudResource | null>(null)

  const invalidate = () => void qc.invalidateQueries({ queryKey: ["cloud", pid, "SECURITY_GROUP"] })

  const create = useMutation({
    mutationFn: () =>
      apiFetch(`/project/${pid}/cloud`, {
        method: "POST",
        body: { type: "SECURITY_GROUP", data: { name, description } },
        cloud: scope,
      }),
    onSuccess: () => {
      toast.success(`Security group "${name}" created`)
      setCreateOpen(false)
      setName("")
      setDescription("")
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const del = useMutation({
    mutationFn: (id: string) =>
      apiFetch(`/project/${pid}/cloud/${id}`, { method: "DELETE", cloud: scope }),
    onSuccess: () => {
      toast.success("Security group deletion requested")
      setToDelete(null)
      setTimeout(invalidate, 1500)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <>
      <PageHeader
        title="Security groups"
        description="Firewall rule sets applied to servers and ports."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
              <RefreshCw className={isFetching ? "size-4 animate-spin" : "size-4"} />
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create security group
            </Button>
          </>
        }
      />

      {isLoading ? (
        <Skeleton className="h-64" />
      ) : !data?.length ? (
        <EmptyState
          icon={Shield}
          title="No security groups yet"
          hint="Create a security group and add ingress/egress rules to control traffic."
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create security group
            </Button>
          }
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Description</TableHead>
                <TableHead>Rules</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.map((r) => (
                <TableRow key={r.id}>
                  <TableCell className="font-medium">
                    <Link className="hover:underline" to={`/p/${pid}/security-groups/${r.id}`}>
                      {sgName(r)}
                    </Link>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{sgDescription(r) || "—"}</TableCell>
                  <TableCell className="text-sm">{sgRuleCount(r) ?? "—"}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {timeAgo(r.info?.createdAt ?? r.createdAt)}
                  </TableCell>
                  <TableCell>
                    <Button variant="ghost" size="sm" onClick={() => setToDelete(r)} aria-label="Delete security group">
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
            <DialogTitle>Create security group</DialogTitle>
            <DialogDescription>Rules can be added from the group's detail page after creation.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-2">
            <div className="grid gap-2">
              <Label htmlFor="sg-name">Name</Label>
              <Input id="sg-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="web-servers" />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="sg-desc">Description</Label>
              <Input
                id="sg-desc"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="Allow HTTP/HTTPS traffic"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => create.mutate()} disabled={!name.trim() || create.isPending}>
              {create.isPending ? "Creating…" : "Create security group"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!toDelete} onOpenChange={(o) => !o && setToDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete security group</DialogTitle>
            <DialogDescription>
              Delete "{toDelete ? sgName(toDelete) : ""}" and all of its rules? This cannot be undone.
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
