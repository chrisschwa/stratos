import { useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Database, FolderOpen, MoreHorizontal, Plus, RefreshCw } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { timeAgo } from "@/lib/format"
import { useCloudList, useCloudScope, useProjectId } from "@/lib/hooks"
import type { CloudResource } from "@/lib/types"

export function bucketName(r: CloudResource): string {
  return (r.data?.bucketName as string) || r.externalId || r.name || r.id
}

// sizeInGb may arrive as a number or a decimal string — tolerate both (and a legacy {$numberDecimal} wrapper).
export function bucketGb(r: CloudResource): string {
  const v = r.data?.sizeInGb
  if (v == null) return "—"
  if (typeof v === "object") {
    const d = (v as Record<string, unknown>).$numberDecimal
    return d != null ? `${d} GB` : "—"
  }
  return `${v} GB`
}

export default function BucketsPage() {
  const pid = useProjectId()
  const scope = useCloudScope(pid)
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { data, isLoading, isError, error, refetch, isFetching } = useCloudList(pid, "BUCKET")

  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState("")
  const [deleteTarget, setDeleteTarget] = useState<CloudResource | null>(null)

  const invalidate = () => void qc.invalidateQueries({ queryKey: ["cloud", pid, "BUCKET"] })

  const create = useMutation({
    mutationFn: () =>
      apiFetch(`/project/${pid}/cloud`, {
        method: "POST",
        cloud: scope,
        body: { type: "BUCKET", data: { bucketName: name } },
      }),
    onSuccess: () => {
      toast.success("Bucket created")
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
      toast.success("Bucket deletion requested")
      setDeleteTarget(null)
      invalidate()
    },
    // Swift rejects deleting a non-empty container — surface the API error.
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <>
      <PageHeader
        title="Object storage"
        description="Buckets for storing objects and files."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
              <RefreshCw className={isFetching ? "size-4 animate-spin" : "size-4"} />
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create bucket
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
          icon={Database}
          title="No buckets yet"
          hint="Create a bucket to store objects and files."
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create bucket
            </Button>
          }
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Objects</TableHead>
                <TableHead>Size</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-10" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.map((r) => (
                <TableRow key={r.id}>
                  <TableCell className="font-medium">
                    <Link className="hover:underline" to={`/p/${pid}/object-storage/${r.id}`}>
                      {bucketName(r)}
                    </Link>
                  </TableCell>
                  <TableCell className="text-sm">{(r.data?.objectCount as number) ?? 0}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{bucketGb(r)}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {timeAgo(r.info?.createdAt ?? r.createdAt)}
                  </TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="sm">
                          <MoreHorizontal className="size-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={() => navigate(`/p/${pid}/object-storage/${r.id}`)}>
                          <FolderOpen className="size-4" /> Browse
                        </DropdownMenuItem>
                        <DropdownMenuItem className="text-destructive" onClick={() => setDeleteTarget(r)}>
                          Delete
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
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
            <DialogTitle>Create bucket</DialogTitle>
            <DialogDescription>Bucket names must be unique within this project.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="bucket-name">Bucket name</Label>
            <Input id="bucket-name" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => create.mutate()} disabled={!name.trim() || create.isPending}>
              {create.isPending ? "Creating…" : "Create bucket"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!deleteTarget} onOpenChange={(o) => !o && setDeleteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete bucket</DialogTitle>
            <DialogDescription>
              This deletes {deleteTarget ? bucketName(deleteTarget) : ""}. The bucket must be empty — delete its
              objects first.
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
              {del.isPending ? "Deleting…" : "Delete bucket"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
