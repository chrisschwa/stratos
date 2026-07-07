import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Pencil, Percent, Plus, RefreshCw, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { useAdminList } from "@/lib/hooks"

type RateLevel = { level: number; percentage: number }
type TaxRate = {
  id: string
  name?: string
  country?: string
  state?: string
  level?: string
  accessMode?: string
  rateLevels?: RateLevel[]
}

const LIST_PATH = "/admin/tax"
const LEVELS = ["ALL", "BUSINESS_ONLY", "CONSUMERS_ONLY"]

type FormState = {
  name: string
  percentage: string
  country: string
  level: string
  accessMode: string
}

const emptyForm: FormState = { name: "", percentage: "", country: "", level: "ALL", accessMode: "PUBLIC" }

function formToBody(f: FormState) {
  const body: Record<string, unknown> = {
    name: f.name,
    rateLevels: [{ level: 1, percentage: parseInt(f.percentage, 10) }],
    level: f.level,
    accessMode: f.accessMode,
  }
  if (f.country.trim()) body.country = f.country.trim()
  return body
}

function TaxForm({ form, setForm }: { form: FormState; setForm: (f: FormState) => void }) {
  return (
    <div className="grid gap-4">
      <div className="grid gap-2">
        <Label htmlFor="tax-name">Name</Label>
        <Input id="tax-name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="VAT" />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="grid gap-2">
          <Label htmlFor="tax-pct">Percentage</Label>
          <Input
            id="tax-pct"
            type="number"
            min="0"
            step="1"
            value={form.percentage}
            onChange={(e) => setForm({ ...form, percentage: e.target.value })}
            placeholder="19"
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="tax-country">Country (optional)</Label>
          <Input
            id="tax-country"
            value={form.country}
            onChange={(e) => setForm({ ...form, country: e.target.value })}
            placeholder="All countries when empty"
          />
        </div>
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="grid gap-2">
          <Label>Applies to</Label>
          <Select value={form.level} onValueChange={(v) => setForm({ ...form, level: v })}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {LEVELS.map((l) => (
                <SelectItem key={l} value={l}>
                  {l}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="grid gap-2">
          <Label>Access mode</Label>
          <Select value={form.accessMode} onValueChange={(v) => setForm({ ...form, accessMode: v })}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="PUBLIC">PUBLIC</SelectItem>
              <SelectItem value="SCOPED">SCOPED</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
    </div>
  )
}

export default function TaxesPage() {
  const qc = useQueryClient()
  const { data, isLoading, error, refetch, isFetching } = useAdminList<TaxRate>(LIST_PATH)
  const rates = data?.data ?? []

  const invalidate = () => qc.invalidateQueries({ queryKey: ["admin-list", LIST_PATH] })

  const [createOpen, setCreateOpen] = useState(false)
  const [createForm, setCreateForm] = useState<FormState>(emptyForm)

  const [editing, setEditing] = useState<TaxRate | null>(null)
  const [editForm, setEditForm] = useState<FormState>(emptyForm)

  const [toDelete, setToDelete] = useState<TaxRate | null>(null)

  const createTax = useMutation({
    mutationFn: () => apiFetch(LIST_PATH, { method: "POST", body: formToBody(createForm) }),
    onSuccess: () => {
      toast.success("Tax rate created")
      setCreateOpen(false)
      setCreateForm(emptyForm)
      void invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const updateTax = useMutation({
    mutationFn: (id: string) => apiFetch(`${LIST_PATH}/${id}`, { method: "PUT", body: formToBody(editForm) }),
    onSuccess: () => {
      toast.success("Tax rate updated")
      setEditing(null)
      void invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const deleteTax = useMutation({
    mutationFn: (id: string) => apiFetch(`${LIST_PATH}/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Tax rate deleted")
      setToDelete(null)
      void invalidate()
    },
    // A SCOPED rate referenced by billing profiles refuses deletion — surface the API message.
    onError: (e: Error) => toast.error(e.message),
  })

  const openEdit = (t: TaxRate) => {
    setEditForm({
      name: t.name ?? "",
      percentage: String(t.rateLevels?.[0]?.percentage ?? ""),
      country: t.country ?? "",
      level: t.level ?? "ALL",
      accessMode: t.accessMode ?? "PUBLIC",
    })
    setEditing(t)
  }

  const formValid = (f: FormState) => f.name.trim() !== "" && f.percentage !== "" && !Number.isNaN(parseInt(f.percentage, 10))

  return (
    <>
      <PageHeader
        title="Taxes"
        description="Tax rates applied when bills and payments are generated."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
              <RefreshCw className={isFetching ? "size-4 animate-spin" : "size-4"} />
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create tax rate
            </Button>
          </>
        }
      />

      {isLoading ? (
        <Skeleton className="h-64" />
      ) : error ? (
        <div className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground">{(error as Error).message}</div>
      ) : !rates.length ? (
        <EmptyState
          icon={Percent}
          title="No tax rates yet"
          hint="Create a rate — it is applied to gross amounts at billing time."
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create tax rate
            </Button>
          }
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Country</TableHead>
                <TableHead>Applies to</TableHead>
                <TableHead>Rate levels</TableHead>
                <TableHead>Access</TableHead>
                <TableHead className="w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {rates.map((t) => (
                <TableRow key={t.id}>
                  <TableCell className="font-medium">{t.name ?? t.id}</TableCell>
                  <TableCell className="text-sm">{t.country || "All countries"}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{t.level ?? "—"}</TableCell>
                  <TableCell className="font-mono text-sm tabular-nums">
                    {t.rateLevels?.length ? t.rateLevels.map((l) => `${l.level}: ${l.percentage}%`).join(", ") : "—"}
                  </TableCell>
                  <TableCell>
                    <Badge variant={t.accessMode === "SCOPED" ? "secondary" : "outline"}>{t.accessMode ?? "PUBLIC"}</Badge>
                  </TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-1">
                      <Button variant="ghost" size="icon" title="Edit" onClick={() => openEdit(t)}>
                        <Pencil className="size-4 text-muted-foreground" />
                      </Button>
                      <Button variant="ghost" size="icon" title="Delete" onClick={() => setToDelete(t)}>
                        <Trash2 className="size-4 text-muted-foreground" />
                      </Button>
                    </div>
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
            <DialogTitle>Create tax rate</DialogTitle>
            <DialogDescription>The percentage is stored as level 1 of the rate.</DialogDescription>
          </DialogHeader>
          <TaxForm form={createForm} setForm={setCreateForm} />
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => createTax.mutate()} disabled={!formValid(createForm) || createTax.isPending}>
              Create tax rate
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!editing} onOpenChange={(o) => !o && setEditing(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Update tax rate</DialogTitle>
            <DialogDescription>Changes apply to future billing runs.</DialogDescription>
          </DialogHeader>
          <TaxForm form={editForm} setForm={setEditForm} />
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditing(null)}>
              Cancel
            </Button>
            <Button
              onClick={() => editing && updateTax.mutate(editing.id)}
              disabled={!formValid(editForm) || updateTax.isPending}
            >
              Save changes
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!toDelete} onOpenChange={(o) => !o && setToDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete tax rate</DialogTitle>
            <DialogDescription>
              Delete "{toDelete?.name}"? A SCOPED rate still referenced by billing profiles will refuse to delete.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setToDelete(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => toDelete && deleteTax.mutate(toDelete.id)}
              disabled={deleteTax.isPending}
            >
              Delete tax rate
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
