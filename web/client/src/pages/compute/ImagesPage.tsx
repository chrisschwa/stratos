// Images: "My images" = the project's own glance images (owner-filtered cache list,
// POST /project/{pid}/resource?type=IMAGE → CloudResource with data.image); "Public images" =
// the PUBLIC_IMAGES bulk action (flat glance image maps). Own images can be deleted.
// Upload: Go has NO plain IMAGE create (providers/write.go TypeImage = server snapshot only), so
// there is no "create then upload" flow — instead a per-row "Upload data" appears on images still
// in glance "queued" status and streams the raw file to POST /project/{pid}/image/{imageId}/upload.
import { useRef, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HardDrive, RefreshCw, Trash2, Upload } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { apiFetch } from "@/lib/api"
import { timeAgo } from "@/lib/format"
import { useCloudList, useCloudScope, useProjectId } from "@/lib/hooks"
import type { CloudResource } from "@/lib/types"

const gb = (bytes?: number) => (bytes ? (bytes / 1073741824).toFixed(2) : "0.00")

function imageName(r: CloudResource): string {
  return (r.data?.image?.name as string) ?? r.name ?? r.id
}

export default function ImagesPage() {
  const pid = useProjectId()
  const scope = useCloudScope(pid)
  const qc = useQueryClient()

  const mine = useCloudList(pid, "IMAGE")
  const pub = useQuery({
    queryKey: ["bulk-action", pid, "PUBLIC_IMAGES", scope?.serviceId, scope?.region],
    queryFn: () =>
      apiFetch<{ result?: Record<string, any>[] }>(`/project/${pid}/cloud/action`, {
        method: "POST",
        body: { action: "PUBLIC_IMAGES" },
        cloud: scope,
      }),
    enabled: !!pid && !!scope,
    select: (d) => d?.result ?? [],
  })

  const [toDelete, setToDelete] = useState<CloudResource | null>(null)
  const uploadTarget = useRef<CloudResource | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)

  const upload = useMutation({
    // Raw body to the glance image (10GB guard server-side). imageId = the glance externalId.
    mutationFn: ({ r, file }: { r: CloudResource; file: File }) =>
      apiFetch(`/project/${pid}/image/${r.externalId ?? (r.data?.image?.id as string)}/upload`, {
        method: "POST",
        cloud: scope,
        rawBody: file,
        headers: { "Content-Type": "application/octet-stream" },
      }),
    onSuccess: (_d, { file }) => {
      toast.success(`Uploaded ${file.name}`)
      void qc.invalidateQueries({ queryKey: ["cloud", pid, "IMAGE"] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const pickUpload = (r: CloudResource) => {
    uploadTarget.current = r
    fileInput.current?.click()
  }

  const del = useMutation({
    mutationFn: (r: CloudResource) =>
      apiFetch(`/project/${pid}/cloud/${r.id}`, { method: "DELETE", cloud: scope }),
    onSuccess: (_d, r) => {
      toast.success(`Image "${imageName(r)}" deleted`)
      void qc.invalidateQueries({ queryKey: ["cloud", pid, "IMAGE"] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <>
      <PageHeader
        title="Images"
        description="Your project's images and snapshots, plus the public OS catalog."
        actions={
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              void mine.refetch()
              void pub.refetch()
            }}
            disabled={mine.isFetching || pub.isFetching}
          >
            <RefreshCw className={mine.isFetching || pub.isFetching ? "size-4 animate-spin" : "size-4"} />
          </Button>
        }
      />

      <Tabs defaultValue="mine">
        <TabsList>
          <TabsTrigger value="mine">My images</TabsTrigger>
          <TabsTrigger value="public">Public images</TabsTrigger>
        </TabsList>

        <TabsContent value="mine" className="mt-4">
          {mine.isLoading ? (
            <Skeleton className="h-64" />
          ) : mine.error ? (
            <p className="rounded-md bg-muted p-4 text-sm text-muted-foreground">
              {(mine.error as Error).message}
            </p>
          ) : !mine.data?.length ? (
            <EmptyState
              icon={HardDrive}
              title="No images yet"
              hint="Server snapshots and images you upload will show up here."
            />
          ) : (
            <Card className="overflow-hidden py-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Size</TableHead>
                    <TableHead>Visibility</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead className="w-16" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {mine.data.map((r) => (
                    <TableRow key={r.id}>
                      <TableCell className="font-medium">{imageName(r)}</TableCell>
                      <TableCell>
                        <StatusBadge status={(r.data?.image?.status as string) ?? r.status} />
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {gb(r.data?.image?.size as number)} GB
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {(r.data?.image?.visibility as string) ?? "—"}
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {timeAgo(r.info?.createdAt ?? (r.data?.image?.created_at as string) ?? r.createdAt)}
                      </TableCell>
                      <TableCell>
                        <div className="flex justify-end gap-1">
                          {(r.data?.image?.status as string) === "queued" && (
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => pickUpload(r)}
                              disabled={upload.isPending}
                              aria-label="Upload image data"
                              title="Upload image data"
                            >
                              <Upload className="size-4" />
                            </Button>
                          )}
                          <Button variant="ghost" size="sm" onClick={() => setToDelete(r)} aria-label="Delete image">
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
        </TabsContent>

        <TabsContent value="public" className="mt-4">
          {pub.isLoading ? (
            <Skeleton className="h-64" />
          ) : pub.error ? (
            <p className="rounded-md bg-muted p-4 text-sm text-muted-foreground">
              {(pub.error as Error).message}
            </p>
          ) : !pub.data?.length ? (
            <EmptyState icon={HardDrive} title="No public images" hint="The region's public OS catalog is empty." />
          ) : (
            <Card className="overflow-hidden py-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>OS</TableHead>
                    <TableHead>Size</TableHead>
                    <TableHead>Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {pub.data.map((im) => (
                    <TableRow key={String(im.id)}>
                      <TableCell className="font-medium">{String(im.name ?? im.id)}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {[im.os_distro, im.os_version].filter(Boolean).join(" ") || "—"}
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">{gb(im.size as number)} GB</TableCell>
                      <TableCell>
                        <StatusBadge status={im.status as string} />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Card>
          )}
        </TabsContent>
      </Tabs>

      <input
        ref={fileInput}
        type="file"
        className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0]
          const r = uploadTarget.current
          e.target.value = ""
          if (!f || !r) return
          if (f.size > 10 * 1024 ** 3) {
            toast.error("Image is larger than 10 GB")
            return
          }
          upload.mutate({ r, file: f })
        }}
      />

      <Dialog open={!!toDelete} onOpenChange={(o) => !o && setToDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete image</DialogTitle>
            <DialogDescription>
              Delete image “{toDelete ? imageName(toDelete) : ""}”? This cannot be undone.
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
