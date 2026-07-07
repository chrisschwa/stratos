import { useState } from "react"
import { useParams } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Plus, Trash2 } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
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
import { useCloudResource, useCloudScope, useProjectId } from "@/lib/hooks"

// Raw designate recordset (GET_RECORDSETS returns the zone's recordsets verbatim).
type Recordset = {
  id?: string
  name?: string
  type?: string
  ttl?: number | null
  records?: string[]
}

const RECORD_TYPES = ["A", "AAAA", "CNAME", "MX", "TXT", "NS", "SRV", "PTR"]

export default function DnsZoneDetailPage() {
  const pid = useProjectId()
  const { resourceId = "" } = useParams()
  const scope = useCloudScope(pid)
  const qc = useQueryClient()
  const { data: zone, isLoading } = useCloudResource(pid, resourceId)

  const recordsets = useQuery({
    queryKey: ["zone-recordsets", pid, resourceId],
    queryFn: () =>
      apiFetch<{ result?: Recordset[] }>(`/project/${pid}/cloud/${resourceId}/action`, {
        method: "POST",
        body: { action: "GET_RECORDSETS" },
        cloud: scope,
      }),
    enabled: !!pid && !!resourceId && !!scope,
  })

  const [addOpen, setAddOpen] = useState(false)
  const [recName, setRecName] = useState("")
  const [recType, setRecType] = useState("A")
  const [recTtl, setRecTtl] = useState("")
  const [recValues, setRecValues] = useState("")
  const [toDelete, setToDelete] = useState<Recordset | null>(null)

  const refresh = () => void qc.invalidateQueries({ queryKey: ["zone-recordsets", pid, resourceId] })

  const addRecord = useMutation({
    mutationFn: () => {
      const records = recValues
        .split(/[\n,]+/)
        .map((s) => s.trim())
        .filter(Boolean)
      const data: Record<string, unknown> = { name: recName.trim(), type: recType, records }
      if (recTtl.trim()) data.ttl = Number(recTtl)
      return apiFetch(`/project/${pid}/cloud/${resourceId}/action`, {
        method: "POST",
        body: { action: "CREATE_RECORDSET", data },
        cloud: scope,
      })
    },
    onSuccess: () => {
      toast.success("Record set created")
      setAddOpen(false)
      setRecName("")
      setRecValues("")
      setRecTtl("")
      refresh()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const deleteRecord = useMutation({
    mutationFn: (recordsetId: string) =>
      apiFetch(`/project/${pid}/cloud/${resourceId}/action`, {
        method: "POST",
        body: { action: "DELETE_RECORDSET", data: { recordsetId } },
        cloud: scope,
      }),
    onSuccess: () => {
      toast.success("Record set deleted")
      setToDelete(null)
      refresh()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  if (isLoading || !zone) {
    return (
      <>
        <PageHeader title="DNS zone" />
        <Skeleton className="h-72" />
      </>
    )
  }

  const z = zone.data?.zone ?? {}
  const domain = (z.name as string) ?? (zone.data?.name as string) ?? zone.name ?? zone.id

  return (
    <>
      <PageHeader
        title={domain}
        description={`Contact ${(z.email as string) ?? "—"} — record sets in this zone.`}
        actions={
          <Button size="sm" onClick={() => setAddOpen(true)}>
            <Plus className="size-4" /> Add record set
          </Button>
        }
      />

      <div className="mb-4 flex items-center gap-3">
        <StatusBadge status={(z.status as string) ?? zone.status} />
        <span className="font-mono text-xs text-muted-foreground">{zone.externalId}</span>
      </div>

      <Card className="overflow-hidden py-0">
        <CardHeader className="pt-6">
          <CardTitle className="text-base">Record sets</CardTitle>
        </CardHeader>
        <CardContent className="px-0 pb-0">
          {recordsets.isLoading ? (
            <Skeleton className="m-6 h-32" />
          ) : recordsets.isError ? (
            <p className="p-6 text-sm text-muted-foreground">{(recordsets.error as Error).message}</p>
          ) : !recordsets.data?.result?.length ? (
            <p className="p-6 text-center text-sm text-muted-foreground">No record sets in this zone.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>TTL</TableHead>
                  <TableHead>Records</TableHead>
                  <TableHead className="w-12" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {recordsets.data.result.map((rs, i) => (
                  <TableRow key={rs.id ?? i}>
                    <TableCell className="font-medium">{rs.name ?? "—"}</TableCell>
                    <TableCell>{rs.type ?? "—"}</TableCell>
                    <TableCell className="font-mono text-sm">{rs.ttl ?? "—"}</TableCell>
                    <TableCell className="max-w-md truncate font-mono text-sm">
                      {rs.records?.join(", ") || "—"}
                    </TableCell>
                    <TableCell>
                      {/* NS/SOA are zone-managed; deleting them fails server-side, so the button stays generic. */}
                      <Button variant="ghost" size="sm" onClick={() => setToDelete(rs)} aria-label="Delete record set">
                        <Trash2 className="size-4" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add record set</DialogTitle>
            <DialogDescription>Record names must end with the zone domain (e.g. www.{domain}).</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-2">
            <div className="grid gap-2">
              <Label htmlFor="rec-name">Name</Label>
              <Input
                id="rec-name"
                value={recName}
                onChange={(e) => setRecName(e.target.value)}
                placeholder={`www.${domain}`}
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <Label>Type</Label>
                <Select value={recType} onValueChange={setRecType}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {RECORD_TYPES.map((t) => (
                      <SelectItem key={t} value={t}>
                        {t}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="rec-ttl">TTL (seconds, optional)</Label>
                <Input
                  id="rec-ttl"
                  type="number"
                  value={recTtl}
                  onChange={(e) => setRecTtl(e.target.value)}
                  placeholder="3600"
                />
              </div>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="rec-values">Records (one per line)</Label>
              <Textarea
                id="rec-values"
                value={recValues}
                onChange={(e) => setRecValues(e.target.value)}
                placeholder={"192.0.2.10\n192.0.2.11"}
                rows={3}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAddOpen(false)}>
              Cancel
            </Button>
            <Button
              onClick={() => addRecord.mutate()}
              disabled={!recName.trim() || !recValues.trim() || addRecord.isPending}
            >
              {addRecord.isPending ? "Creating…" : "Add record set"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!toDelete} onOpenChange={(o) => !o && setToDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete record set</DialogTitle>
            <DialogDescription>
              Delete the {toDelete?.type ?? ""} record set "{toDelete?.name ?? ""}"? This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setToDelete(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => toDelete?.id && deleteRecord.mutate(toDelete.id)}
              disabled={deleteRecord.isPending}
            >
              {deleteRecord.isPending ? "Deleting…" : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
