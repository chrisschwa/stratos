// Keypairs: list KEYPAIR cloud resources, create (data{name, publicKey?} per
// internal/cloud/providers/write.go TypeKeypair; a nova-generated private key comes back
// once as ephemeralData.privateKey) and delete with confirm.
import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Copy, Download, KeyRound, Plus, RefreshCw, Trash2 } from "lucide-react"
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
import { Textarea } from "@/components/ui/textarea"
import { apiFetch } from "@/lib/api"
import { timeAgo } from "@/lib/format"
import { useCloudList, useCloudScope, useProjectId } from "@/lib/hooks"
import type { CloudResource } from "@/lib/types"

function keypairName(r: CloudResource): string {
  return (r.data?.keypair?.name as string) ?? r.name ?? r.id
}
function keypairFingerprint(r: CloudResource): string {
  return (r.data?.keypair?.fingerprint as string) ?? "—"
}

export default function KeypairsPage() {
  const pid = useProjectId()
  const scope = useCloudScope(pid)
  const qc = useQueryClient()
  const { data, isLoading, error, refetch, isFetching } = useCloudList(pid, "KEYPAIR")

  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState("")
  const [publicKey, setPublicKey] = useState("")
  const [toDelete, setToDelete] = useState<CloudResource | null>(null)
  // The generated private key is returned exactly once (ephemeralData) — shown here.
  const [privateKey, setPrivateKey] = useState<{ name: string; key: string } | null>(null)

  const invalidate = () => void qc.invalidateQueries({ queryKey: ["cloud", pid, "KEYPAIR"] })

  const create = useMutation({
    mutationFn: () =>
      apiFetch<CloudResource>(`/project/${pid}/cloud`, {
        method: "POST",
        cloud: scope,
        body: {
          type: "KEYPAIR",
          data: { name: name.trim(), ...(publicKey.trim() ? { publicKey: publicKey.trim() } : {}) },
        },
      }),
    onSuccess: (res) => {
      const kpName = name.trim()
      toast.success(`Keypair "${kpName}" created`)
      setCreateOpen(false)
      setName("")
      setPublicKey("")
      invalidate()
      const pk = (res?.ephemeralData as { privateKey?: string } | undefined)?.privateKey
      if (pk) setPrivateKey({ name: kpName, key: pk })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const del = useMutation({
    mutationFn: (r: CloudResource) =>
      apiFetch(`/project/${pid}/cloud/${r.id}`, { method: "DELETE", cloud: scope }),
    onSuccess: (_d, r) => {
      toast.success(`Keypair "${keypairName(r)}" deleted`)
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const copyKey = async () => {
    if (!privateKey) return
    await navigator.clipboard.writeText(privateKey.key)
    toast.success("Private key copied to clipboard")
  }
  const downloadKey = () => {
    if (!privateKey) return
    const url = URL.createObjectURL(new Blob([privateKey.key], { type: "application/x-pem-file" }))
    const a = document.createElement("a")
    a.href = url
    a.download = `${privateKey.name}.pem`
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <>
      <PageHeader
        title="Keypairs"
        description="SSH keypairs injected into servers at boot."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
              <RefreshCw className={isFetching ? "size-4 animate-spin" : "size-4"} />
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create keypair
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
          icon={KeyRound}
          title="No keypairs yet"
          hint="Create a keypair to SSH into your servers — import your public key or let the cloud generate one."
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create keypair
            </Button>
          }
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Fingerprint</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-16" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.map((r) => (
                <TableRow key={r.id}>
                  <TableCell className="font-medium">{keypairName(r)}</TableCell>
                  <TableCell className="font-mono text-sm text-muted-foreground">
                    {keypairFingerprint(r)}
                  </TableCell>
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
            <DialogTitle>Create keypair</DialogTitle>
            <DialogDescription>
              Leave the public key empty to have a new keypair generated — the private key is shown only once.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="kp-name">Name</Label>
              <Input id="kp-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="my-key" />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="kp-pub">Public key (optional)</Label>
              <Textarea
                id="kp-pub"
                value={publicKey}
                onChange={(e) => setPublicKey(e.target.value)}
                placeholder="ssh-ed25519 AAAA…"
                rows={4}
                className="font-mono text-xs"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => create.mutate()} disabled={!name.trim() || create.isPending}>
              {create.isPending ? "Creating…" : "Create keypair"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!privateKey} onOpenChange={(o) => !o && setPrivateKey(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Save your private key</DialogTitle>
            <DialogDescription>
              This is the only time the private key is shown — copy or download it now.
            </DialogDescription>
          </DialogHeader>
          <pre className="max-h-64 overflow-auto rounded-md bg-muted p-3 font-mono text-xs whitespace-pre-wrap break-all">
            {privateKey?.key}
          </pre>
          <DialogFooter>
            <Button variant="outline" onClick={() => void copyKey()}>
              <Copy className="size-4" /> Copy
            </Button>
            <Button onClick={downloadKey}>
              <Download className="size-4" /> Download .pem
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!toDelete} onOpenChange={(o) => !o && setToDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete keypair</DialogTitle>
            <DialogDescription>
              Delete keypair “{toDelete ? keypairName(toDelete) : ""}”? This cannot be undone.
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
