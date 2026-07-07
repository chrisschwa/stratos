import { useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Copy, Plus, RefreshCw, Tags, Trash2 } from "lucide-react"
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
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { fmtDate } from "@/lib/format"
import { useAdminList } from "@/lib/hooks"

type PricePlan = {
  id: string
  name?: string
  enabled?: boolean
  accessMode?: string
  serviceProviders?: unknown
  createdAt?: string
}

const LIST_PATH = "/admin/price-plan"

type CloneResponse = {
  clonedPricePlans?: Array<{ sourcePricePlanId?: string; newPricePlanId?: string; newPricePlanName?: string; rulesCloned?: number }>
}

export default function PricePlansPage() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { data, isLoading, error, refetch, isFetching } = useAdminList<PricePlan>(LIST_PATH)
  const plans = data?.data ?? []

  const invalidate = () => qc.invalidateQueries({ queryKey: ["admin-list", LIST_PATH] })

  // Create dialog state
  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState("")
  const [accessMode, setAccessMode] = useState("PUBLIC")
  const [enabled, setEnabled] = useState(true)

  const createPlan = useMutation({
    mutationFn: () => apiFetch(LIST_PATH, { method: "POST", body: { name, accessMode, enabled } }),
    onSuccess: () => {
      toast.success("Price plan created")
      setCreateOpen(false)
      setName("")
      setAccessMode("PUBLIC")
      setEnabled(true)
      void invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  // Enabled toggle — PUT overwrites name/enabled/serviceProviders, so send them all back.
  const togglePlan = useMutation({
    mutationFn: (p: PricePlan) =>
      apiFetch(`${LIST_PATH}/${p.id}`, {
        method: "PUT",
        body: { name: p.name, enabled: !p.enabled, accessMode: p.accessMode, serviceProviders: p.serviceProviders },
      }),
    onSuccess: () => void invalidate(),
    onError: (e: Error) => toast.error(e.message),
  })

  // Clone dialog state — POST /admin/price-plan/clone {pricePlans:[{pricePlanId,newName,includeRules}]}
  const [toClone, setToClone] = useState<PricePlan | null>(null)
  const [cloneName, setCloneName] = useState("")
  const clonePlan = useMutation({
    mutationFn: (p: PricePlan) =>
      apiFetch<CloneResponse>(`${LIST_PATH}/clone`, {
        method: "POST",
        body: { pricePlans: [{ pricePlanId: p.id, newName: cloneName.trim(), includeRules: true }] },
      }),
    onSuccess: (res) => {
      const cloned = res?.clonedPricePlans?.[0]
      toast.success(`Cloned to "${cloned?.newPricePlanName ?? "copy"}" (${cloned?.rulesCloned ?? 0} rules)`)
      setToClone(null)
      setCloneName("")
      void invalidate()
      if (cloned?.newPricePlanId) navigate(`/system/price-plans/${cloned.newPricePlanId}`)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  // Delete confirm state
  const [toDelete, setToDelete] = useState<PricePlan | null>(null)
  const deletePlan = useMutation({
    mutationFn: (id: string) => apiFetch(`${LIST_PATH}/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Price plan deleted")
      setToDelete(null)
      void invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <>
      <PageHeader
        title="Price plans"
        description="Rating plans applied to cloud usage."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
              <RefreshCw className={isFetching ? "size-4 animate-spin" : "size-4"} />
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create price plan
            </Button>
          </>
        }
      />

      {isLoading ? (
        <Skeleton className="h-64" />
      ) : error ? (
        <div className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground">{(error as Error).message}</div>
      ) : !plans.length ? (
        <EmptyState
          icon={Tags}
          title="No price plans yet"
          hint="Create a plan, then add per-resource pricing rules to it."
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" /> Create price plan
            </Button>
          }
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Access</TableHead>
                <TableHead>Enabled</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {plans.map((p) => (
                <TableRow key={p.id}>
                  <TableCell className="font-medium">
                    <Link className="hover:underline" to={`/system/price-plans/${p.id}`}>
                      {p.name ?? p.id}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <Badge variant={p.accessMode === "SCOPED" ? "secondary" : "outline"}>
                      {p.accessMode ?? "PUBLIC"}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <Switch
                      checked={!!p.enabled}
                      disabled={togglePlan.isPending}
                      onCheckedChange={() => togglePlan.mutate(p)}
                    />
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{fmtDate(p.createdAt)}</TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        title="Clone plan"
                        onClick={() => {
                          setToClone(p)
                          setCloneName(`${p.name ?? p.id} (Copy)`)
                        }}
                      >
                        <Copy className="size-4 text-muted-foreground" />
                      </Button>
                      <Button variant="ghost" size="icon" title="Delete plan" onClick={() => setToDelete(p)}>
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
            <DialogTitle>Create price plan</DialogTitle>
            <DialogDescription>Rules are added on the plan's detail page after creation.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="pp-name">Name</Label>
              <Input id="pp-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Standard pricing" />
            </div>
            <div className="grid gap-2">
              <Label>Access mode</Label>
              <Select value={accessMode} onValueChange={setAccessMode}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="PUBLIC">PUBLIC — applies to everyone</SelectItem>
                  <SelectItem value="SCOPED">SCOPED — assigned per profile</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center gap-3">
              <Switch id="pp-enabled" checked={enabled} onCheckedChange={setEnabled} />
              <Label htmlFor="pp-enabled">Enabled</Label>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => createPlan.mutate()} disabled={!name.trim() || createPlan.isPending}>
              Create price plan
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!toClone} onOpenChange={(o) => !o && setToClone(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Clone price plan</DialogTitle>
            <DialogDescription>
              Copies "{toClone?.name}" with all its rules. The clone starts disabled.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="pp-clone-name">New plan name</Label>
            <Input id="pp-clone-name" value={cloneName} onChange={(e) => setCloneName(e.target.value)} />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setToClone(null)}>
              Cancel
            </Button>
            <Button onClick={() => toClone && clonePlan.mutate(toClone)} disabled={clonePlan.isPending}>
              Clone plan
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!toDelete} onOpenChange={(o) => !o && setToDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete price plan</DialogTitle>
            <DialogDescription>
              Delete "{toDelete?.name}" and all its rules? This cannot be undone. Plans in use by services or
              projects cannot be deleted.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setToDelete(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => toDelete && deletePlan.mutate(toDelete.id)}
              disabled={deletePlan.isPending}
            >
              Delete price plan
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
