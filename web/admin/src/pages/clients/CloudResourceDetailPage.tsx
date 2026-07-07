import { useState } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Cloud, RefreshCw, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { apiFetch } from "@/lib/api"
import { useAdminGet } from "@/lib/hooks"
import { fmtDateTime } from "@/lib/format"
import {
  resourceCreatedAt, resourceName, resourceStatus, type CloudResourceRow,
} from "./CloudResourcesPage"

// GET /admin/cloud-resource/{id} — the typed CloudResource doc, or {} when absent (httpx.Empty).
// GET /admin/cloud-resource/{id}/sync — live re-fetch + cache upsert, returns the refreshed doc
//   (cloudresourcemut.go registers sync as GET).
// DELETE /admin/cloud-resource/{id} — deletes the REAL OpenStack resource + archives the cache → 202.
export default function CloudResourceDetailPage() {
  const { id = "" } = useParams()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const path = `/admin/cloud-resource/${id}`
  const { data: res, isLoading, isError, error } = useAdminGet<CloudResourceRow>(path)
  const [confirmDelete, setConfirmDelete] = useState(false)

  const sync = useMutation({
    mutationFn: () => apiFetch<CloudResourceRow>(`${path}/sync`),
    onSuccess: () => {
      toast.success("Resource synced from the cloud")
      void qc.invalidateQueries({ queryKey: ["admin-get", path] })
      void qc.invalidateQueries({ queryKey: ["admin-list", "/admin/cloud-resource"] })
    },
    // Surface the exact API message (404, or the 501 seam for unsynced types).
    onError: (e: Error) => toast.error(e.message),
  })

  const del = useMutation({
    mutationFn: () => apiFetch(path, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Delete accepted — the cloud resource is being removed")
      void qc.invalidateQueries({ queryKey: ["admin-list", "/admin/cloud-resource"] })
      navigate("/clients/cloud-resources")
    },
    onError: (e: Error) => toast.error(e.message),
  })

  if (isLoading) {
    return (
      <>
        <PageHeader title="Cloud resource" description="Loading…" />
        <Skeleton className="h-64" />
      </>
    )
  }
  if (isError) {
    return (
      <>
        <PageHeader title="Cloud resource" description="Failed to load." />
        <div className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground">{(error as Error).message}</div>
      </>
    )
  }
  // Absent resource → the API returns an empty {} envelope.
  if (!res?.id) {
    return (
      <>
        <PageHeader title="Cloud resource" description="This resource does not exist." />
        <EmptyState
          icon={Cloud}
          title="Resource not found"
          hint="It may have been deleted or archived."
          action={
            <Button variant="outline" asChild>
              <Link to="/clients/cloud-resources">Back to cloud resources</Link>
            </Button>
          }
        />
      </>
    )
  }

  const name = resourceName(res)
  const status = resourceStatus(res)
  const type = (res.type as string | undefined)?.toLowerCase().replace(/_/g, " ") ?? "resource"

  return (
    <>
      <PageHeader
        title={name}
        description={`Cloud ${type} — cached copy of the live OpenStack object.`}
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => sync.mutate()} disabled={sync.isPending}>
              <RefreshCw className={sync.isPending ? "size-4 animate-spin" : "size-4"} /> Sync
            </Button>
            <Button variant="destructive" size="sm" onClick={() => setConfirmDelete(true)} disabled={del.isPending}>
              <Trash2 className="size-4" /> Delete
            </Button>
          </>
        }
      />

      <div className="grid gap-4">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Details</CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="grid gap-x-8 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
              <div>
                <dt className="text-muted-foreground">Status</dt>
                <dd className="mt-0.5"><StatusBadge status={status} /></dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Type</dt>
                <dd className="mt-0.5 capitalize">{type}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">External ID</dt>
                <dd className="mt-0.5 break-all font-mono text-xs">{res.externalId ?? "—"}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Project</dt>
                <dd className="mt-0.5">
                  {res.projectId ? (
                    <Link className="break-all font-mono text-xs hover:underline" to={`/clients/projects/${res.projectId}`}>
                      {res.projectId}
                    </Link>
                  ) : (
                    "—"
                  )}
                </dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Service</dt>
                <dd className="mt-0.5 break-all font-mono text-xs">{res.serviceId ?? "—"}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Region</dt>
                <dd className="mt-0.5">{res.region ?? "—"}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Created</dt>
                <dd className="mt-0.5">{fmtDateTime(resourceCreatedAt(res))}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Updated</dt>
                <dd className="mt-0.5">{fmtDateTime(res.updatedAt)}</dd>
              </div>
            </dl>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Raw data</CardTitle>
          </CardHeader>
          <CardContent>
            {res.data && Object.keys(res.data).length ? (
              <pre className="max-h-[32rem] overflow-auto rounded-md bg-muted/40 p-4 font-mono text-xs leading-relaxed">
                {JSON.stringify(res.data, null, 2)}
              </pre>
            ) : (
              <p className="text-sm text-muted-foreground">No cached data for this resource.</p>
            )}
          </CardContent>
        </Card>
      </div>

      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete cloud resource</DialogTitle>
            <DialogDescription>
              This deletes the <span className="font-medium">real {type}</span> "{name}" on the cloud provider,
              not just the cached record. This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDelete(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                del.mutate()
                setConfirmDelete(false)
              }}
            >
              <Trash2 className="size-4" /> Delete resource
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
