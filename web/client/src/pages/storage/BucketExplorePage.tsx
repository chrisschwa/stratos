import { useRef, useState } from "react"
import { Link, useParams } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { ChevronRight, Download, File, Folder, FolderPlus, RefreshCw, Trash2, Upload } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { fmtDateTime } from "@/lib/format"
import { useCloudResource, useCloudScope, useProjectId } from "@/lib/hooks"

type BucketObject = {
  name: string
  displayName?: string
  sizeInBytes?: number
  mimeType?: string
  directory?: boolean
  lastModified?: string
}

function fmtBytes(n?: number): string {
  if (n == null || Number.isNaN(n)) return "—"
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

export default function BucketExplorePage() {
  const pid = useProjectId()
  const { resourceId = "" } = useParams()
  const scope = useCloudScope(pid)
  const qc = useQueryClient()
  const { data: bucket } = useCloudResource(pid, resourceId)
  const [prefix, setPrefix] = useState("") // "" = root; folders carry a trailing "/"
  const [folderOpen, setFolderOpen] = useState(false)
  const [folderName, setFolderName] = useState("")
  const [deleteTarget, setDeleteTarget] = useState<BucketObject | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)

  const bucketLabel = (bucket?.data?.bucketName as string) || bucket?.externalId || "Bucket"

  const action = (name: string, data?: Record<string, unknown>) =>
    apiFetch<{ result?: unknown }>(`/project/${pid}/cloud/${resourceId}/action`, {
      method: "POST",
      cloud: scope,
      body: { action: name, ...(data ? { data } : {}) },
    })

  const objects = useQuery({
    queryKey: ["bucket-objects", pid, resourceId, prefix],
    queryFn: async () => {
      const res = await action("LIST_OBJECTS", prefix ? { folderName: prefix } : undefined)
      return (res.result as BucketObject[]) ?? []
    },
    enabled: !!pid && !!resourceId && !!scope,
  })

  const visibility = useQuery({
    queryKey: ["bucket-public", pid, resourceId],
    queryFn: async () => {
      const res = await action("IS_BUCKET_PUBLIC")
      return res.result === true
    },
    enabled: !!pid && !!resourceId && !!scope,
  })

  const invalidateObjects = () => {
    void qc.invalidateQueries({ queryKey: ["bucket-objects", pid, resourceId] })
    void qc.invalidateQueries({ queryKey: ["cloud-resource", pid, resourceId] })
  }

  const setPublic = useMutation({
    mutationFn: (pub: boolean) => action(pub ? "MAKE_BUCKET_PUBLIC" : "MAKE_BUCKET_PRIVATE"),
    onSuccess: (_d, pub) => {
      toast.success(pub ? "Bucket is now public" : "Bucket is now private")
      void qc.invalidateQueries({ queryKey: ["bucket-public", pid, resourceId] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const createFolder = useMutation({
    mutationFn: () => action("CREATE_FOLDER", { folderName: prefix + folderName.trim() }),
    onSuccess: () => {
      toast.success("Folder created")
      setFolderOpen(false)
      setFolderName("")
      invalidateObjects()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const upload = useMutation({
    mutationFn: (file: File) =>
      apiFetch(
        `/project/${pid}/cloud/${resourceId}/upload-bucket-file?objectName=${encodeURIComponent(prefix + file.name)}`,
        {
          method: "POST",
          cloud: scope,
          rawBody: file,
          headers: { "Content-Type": file.type || "application/octet-stream" },
        }
      ),
    onSuccess: (_d, file) => {
      toast.success(`Uploaded ${file.name}`)
      invalidateObjects()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  // Go DOWNLOAD action (cloud_writes.go) mints a short-lived token and returns the FULL public
  // download URL in result.url (GET /api/v1/download/{token} is whitelisted — the token is the auth).
  const download = useMutation({
    mutationFn: async (obj: BucketObject) => {
      const res = await action("DOWNLOAD", { objectName: obj.name })
      return (res.result as { url?: string } | undefined)?.url
    },
    onSuccess: (url) => {
      if (url) window.open(url, "_blank", "noopener")
      else toast.error("No download URL returned")
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const deleteObject = useMutation({
    mutationFn: (obj: BucketObject) => action("DELETE_OBJECT", { objectName: obj.name }),
    onSuccess: () => {
      toast.success("Object deleted")
      setDeleteTarget(null)
      invalidateObjects()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  // Breadcrumb segments: "a/b/" → ["a", "b"]
  const segments = prefix.split("/").filter(Boolean)

  return (
    <>
      <PageHeader
        title={bucketLabel}
        description="Browse, upload and manage the objects in this bucket."
        actions={
          <>
            <div className="mr-2 flex items-center gap-2">
              {visibility.data !== undefined && (
                <Badge variant={visibility.data ? "default" : "secondary"}>
                  {visibility.data ? "Public" : "Private"}
                </Badge>
              )}
              <Switch
                checked={visibility.data === true}
                disabled={visibility.isLoading || setPublic.isPending}
                onCheckedChange={(v) => setPublic.mutate(v)}
                aria-label="Toggle public access"
              />
            </div>
            <Button variant="outline" size="sm" onClick={() => void objects.refetch()} disabled={objects.isFetching}>
              <RefreshCw className={objects.isFetching ? "size-4 animate-spin" : "size-4"} />
            </Button>
            <Button variant="outline" size="sm" onClick={() => setFolderOpen(true)}>
              <FolderPlus className="size-4" /> New folder
            </Button>
            <Button size="sm" onClick={() => fileInput.current?.click()} disabled={upload.isPending}>
              <Upload className="size-4" /> {upload.isPending ? "Uploading…" : "Upload file"}
            </Button>
            <input
              ref={fileInput}
              type="file"
              className="hidden"
              onChange={(e) => {
                const f = e.target.files?.[0]
                if (f) upload.mutate(f)
                e.target.value = ""
              }}
            />
          </>
        }
      />

      <nav className="mb-4 flex items-center gap-1 text-sm">
        <Link className="text-muted-foreground hover:underline" to={`/p/${pid}/object-storage`}>
          Buckets
        </Link>
        <ChevronRight className="size-4 text-muted-foreground" />
        <button
          className={segments.length ? "text-muted-foreground hover:underline" : "font-medium"}
          onClick={() => setPrefix("")}
        >
          {bucketLabel}
        </button>
        {segments.map((seg, i) => (
          <span key={i} className="flex items-center gap-1">
            <ChevronRight className="size-4 text-muted-foreground" />
            <button
              className={i === segments.length - 1 ? "font-medium" : "text-muted-foreground hover:underline"}
              onClick={() => setPrefix(segments.slice(0, i + 1).join("/") + "/")}
            >
              {seg}
            </button>
          </span>
        ))}
      </nav>

      {objects.isLoading ? (
        <Skeleton className="h-64" />
      ) : objects.isError ? (
        <p className="rounded-md bg-muted p-4 text-sm text-muted-foreground">{(objects.error as Error).message}</p>
      ) : !objects.data?.length ? (
        <EmptyState
          icon={Folder}
          title={prefix ? "This folder is empty" : "This bucket is empty"}
          hint="Upload a file or create a folder to get started."
          action={
            <Button onClick={() => fileInput.current?.click()}>
              <Upload className="size-4" /> Upload file
            </Button>
          }
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Size</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Last modified</TableHead>
                <TableHead className="w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {objects.data.map((o) => (
                <TableRow key={o.name}>
                  <TableCell className="font-medium">
                    {o.directory ? (
                      <button className="flex items-center gap-2 hover:underline" onClick={() => setPrefix(o.name)}>
                        <Folder className="size-4 text-muted-foreground" /> {o.displayName || o.name}
                      </button>
                    ) : (
                      <span className="flex items-center gap-2">
                        <File className="size-4 text-muted-foreground" /> {o.displayName || o.name}
                      </span>
                    )}
                  </TableCell>
                  <TableCell className="text-sm">{o.directory ? "—" : fmtBytes(o.sizeInBytes)}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {o.directory ? "Folder" : o.mimeType || "—"}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {o.lastModified ? fmtDateTime(o.lastModified) : "—"}
                  </TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-1">
                      {!o.directory && (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => download.mutate(o)}
                          disabled={download.isPending}
                          aria-label="Download object"
                        >
                          <Download className="size-4" />
                        </Button>
                      )}
                      <Button variant="ghost" size="sm" onClick={() => setDeleteTarget(o)} aria-label="Delete object">
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

      <Dialog open={folderOpen} onOpenChange={setFolderOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New folder</DialogTitle>
            <DialogDescription>
              Create a folder {prefix ? `inside ${prefix}` : "at the bucket root"}.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="folder-name">Folder name</Label>
            <Input id="folder-name" value={folderName} onChange={(e) => setFolderName(e.target.value)} />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setFolderOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => createFolder.mutate()} disabled={!folderName.trim() || createFolder.isPending}>
              {createFolder.isPending ? "Creating…" : "Create folder"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!deleteTarget} onOpenChange={(o) => !o && setDeleteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete {deleteTarget?.directory ? "folder" : "object"}</DialogTitle>
            <DialogDescription>
              This permanently deletes {deleteTarget?.displayName || deleteTarget?.name}
              {deleteTarget?.directory ? " and everything inside it" : ""}. This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => deleteTarget && deleteObject.mutate(deleteTarget)}
              disabled={deleteObject.isPending}
            >
              {deleteObject.isPending ? "Deleting…" : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
