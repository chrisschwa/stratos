import { useState } from "react"
import { useNavigate } from "react-router-dom"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Building2, Plus, RefreshCw } from "lucide-react"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
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
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { fmtDate } from "@/lib/format"
import { useAdminList } from "@/lib/hooks"

// GET /admin/organizations (organization.go orgToDto) — the org doc shaped (_id→id) +
// memberCount / projectCount / billingProfile?.
type Org = {
  id?: string
  name?: string
  billingProfileId?: string
  memberCount?: number
  projectCount?: number
  createdAt?: string
}

const LIST_PATH = "/admin/organizations"

export default function OrganizationsPage() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { data, isLoading, isFetching, error, refetch } = useAdminList<Org>(LIST_PATH)
  const [createOpen, setCreateOpen] = useState(false)
  const [form, setForm] = useState({ name: "", description: "" })

  const orgs = data?.data ?? []

  // POST /admin/organizations (organization.go organizationCreate) — plain branch: {name, description}.
  const createOrg = useMutation({
    mutationFn: () =>
      apiFetch(LIST_PATH, {
        method: "POST",
        body: { name: form.name, ...(form.description ? { description: form.description } : {}) },
      }),
    onSuccess: () => {
      toast.success("Organization created")
      setCreateOpen(false)
      setForm({ name: "", description: "" })
      qc.invalidateQueries({ queryKey: ["admin-list", LIST_PATH] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <>
      <PageHeader
        title="Organizations"
        description="Client organizations and their memberships."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => refetch()} disabled={isFetching}>
              <RefreshCw className={isFetching ? "animate-spin" : ""} />
              Refresh
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus />
              Create organization
            </Button>
          </>
        }
      />

      {isLoading ? (
        <Skeleton className="h-64" />
      ) : error ? (
        <div className="rounded-lg border bg-muted/40 p-6 text-sm text-muted-foreground">
          {(error as Error).message}
        </div>
      ) : orgs.length === 0 ? (
        <EmptyState
          icon={Building2}
          title="No organizations yet"
          hint="Organizations appear when clients sign up."
          action={
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus />
              Create organization
            </Button>
          }
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>ID</TableHead>
                <TableHead>Members</TableHead>
                <TableHead>Projects</TableHead>
                <TableHead>Billing profile</TableHead>
                <TableHead>Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {orgs.map((o) => (
                <TableRow
                  key={o.id}
                  className="cursor-pointer"
                  onClick={() => o.id && navigate(`/clients/organizations/${o.id}`)}
                >
                  <TableCell className="font-medium">{o.name ?? "—"}</TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{o.id ?? "—"}</TableCell>
                  <TableCell className="tabular-nums">{o.memberCount ?? 0}</TableCell>
                  <TableCell className="tabular-nums">{o.projectCount ?? 0}</TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {o.billingProfileId ?? "—"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">{fmtDate(o.createdAt)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}

      {/* Create organization */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create organization</DialogTitle>
            <DialogDescription>
              Creates an empty organization. Add members and a billing profile from its detail page.
            </DialogDescription>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault()
              createOrg.mutate()
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="org-name">Name</Label>
              <Input
                id="org-name"
                required
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="org-desc">Description</Label>
              <Input
                id="org-desc"
                value={form.description}
                onChange={(e) => setForm({ ...form, description: e.target.value })}
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setCreateOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={createOrg.isPending || !form.name}>
                {createOrg.isPending ? "Creating…" : "Create organization"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}
