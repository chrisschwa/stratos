// Server groups: list SERVER_GROUP cloud resources, create (data{name, policy} per
// internal/cloud/providers/write.go TypeServerGroup) and delete with confirm.
import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Boxes, Plus, RefreshCw, Trash2 } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
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

const POLICIES = ["affinity", "anti-affinity", "soft-affinity", "soft-anti-affinity"]

function groupName(r: CloudResource): string {
  return (r.data?.serverGroup?.name as string) ?? r.name ?? r.id
}
function groupPolicy(r: CloudResource): string {
  const sg = r.data?.serverGroup as Record<string, any> | undefined
  const policies = sg?.policies as string[] | undefined
  if (policies?.length) return policies.join(", ")
  return (sg?.policy as string) ?? "—"
}

export default function ServerGroupsPage() {
  const pid = useProjectId()
  const scope = useCloudScope(pid)
  const qc = useQueryClient()
  const { data, isLoading, error, refetch, isFetching } = useCloudList(pid, "SERVER_GROUP")

  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState("")
  const [policy, setPolicy] = useState(POLICIES[0])
  const [toDelete, setToDelete] = useState<CloudResource | null>(null)

  const invalidate = () => void qc.invalidateQueries({ queryKey: ["cloud", pid, "SERVER_GROUP"] })

  const create = useMutation({
    mutationFn: () =>
      apiFetch<CloudResource>(`/project/${pid}/cloud`, {
        method: "POST",
        cloud: scope,
        body: { type: "SERVER_GROUP", data: { name: name.trim(), policy } },
      }),
    onSuccess: () => {
      toast.success(`Server group "${name.trim()}" created`)
      setCreateOpen(false)
      setName("")
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const del = useMutation({
    mutationFn: (r: CloudResource) =>
      apiFetch(`/project/${pid}/cloud/${r.id}`, { method: "DELETE", cloud: scope }),
    onSuccess: (_d, r) => {
      toast.success(`Server group "${groupName(r)}" deleted`)
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <>
      <PageHeader
        title="Server groups"
        description="Scheduling policies that keep servers together or apart across hosts."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
              <RefreshCw className={isFetching ? "size-4 animate-spin" : "size-4"} />
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create server group
            </Button>
          </>
        }
      />

      {isLoading ? (
        <Skeleton className="h-64" />
      ) : error ? (
        <p className="rounded-md bg-muted p-4 text-sm text-muted-foreground">{(error as Error).message}</p>
      ) : !data?.length ? (
        <EmptyState
          icon={Boxes}
          title="No server groups yet"
          hint="Create a group to control how its member servers are placed across hosts."
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create server group
            </Button>
          }
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Policy</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-16" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.map((r) => (
                <TableRow key={r.id}>
                  <TableCell className="font-medium">{groupName(r)}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{groupPolicy(r)}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {timeAgo(r.info?.createdAt ?? r.createdAt)}
                  </TableCell>
                  <TableCell>
                    <Button variant="ghost" size="sm" onClick={() => setToDelete(r)}>
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
            <DialogTitle>Create server group</DialogTitle>
            <DialogDescription>Servers added to the group follow its placement policy.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="sg-name">Name</Label>
              <Input id="sg-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="my-group" />
            </div>
            <div className="grid gap-2">
              <Label>Policy</Label>
              <Select value={policy} onValueChange={setPolicy}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {POLICIES.map((p) => (
                    <SelectItem key={p} value={p}>
                      {p}
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
            <Button onClick={() => create.mutate()} disabled={!name.trim() || create.isPending}>
              {create.isPending ? "Creating…" : "Create server group"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!toDelete} onOpenChange={(o) => !o && setToDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete server group</DialogTitle>
            <DialogDescription>
              Delete server group “{toDelete ? groupName(toDelete) : ""}”? This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setToDelete(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                if (toDelete) del.mutate(toDelete)
                setToDelete(null)
              }}
            >
              <Trash2 className="size-4" /> Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
