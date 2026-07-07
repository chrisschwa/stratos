import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Pencil, Plus, ShieldCheck, Trash2 } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { useProjectId } from "@/lib/hooks"
import { useOrg } from "./MembersPage"

// GET /organizations/{id}/roles → RoleDto (built-ins OWNER/ADMIN/MEMBER + custom roleDefinitions).
export type OrgRole = {
  id: string
  name: string
  description?: string
  permissions: string[]
  expandedPermissions: string[]
  builtIn: boolean
}

// GET /organizations/{id}/roles/permissions → rbac.PermissionMeta.
type PermissionMeta = { key: string; description?: string; resourceType?: string }

export default function RolesPage() {
  const pid = useProjectId()
  const qc = useQueryClient()
  const { org, isLoading: orgLoading, error: orgError } = useOrg(pid)

  const { data: roles, isLoading, error } = useQuery({
    queryKey: ["org-roles", org?.id],
    queryFn: () => apiFetch<OrgRole[]>(`/organizations/${org?.id}/roles`),
    enabled: !!org?.id,
  })

  const { data: permissions } = useQuery({
    queryKey: ["org-role-permissions", org?.id],
    queryFn: () => apiFetch<PermissionMeta[]>(`/organizations/${org?.id}/roles/permissions`),
    enabled: !!org?.id,
  })

  // Group the permission catalog by resourceType for the dialog checkboxes.
  const permGroups = useMemo(() => {
    const groups = new Map<string, PermissionMeta[]>()
    for (const p of permissions ?? []) {
      const g = p.resourceType || "other"
      if (!groups.has(g)) groups.set(g, [])
      groups.get(g)!.push(p)
    }
    return [...groups.entries()]
  }, [permissions])

  // Create/edit dialog state (editing == null → create).
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<OrgRole | null>(null)
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [deleting, setDeleting] = useState<OrgRole | null>(null)

  const openCreate = () => {
    setEditing(null)
    setName("")
    setDescription("")
    setSelected(new Set())
    setDialogOpen(true)
  }
  const openEdit = (role: OrgRole) => {
    setEditing(role)
    setName(role.name)
    setDescription(role.description ?? "")
    setSelected(new Set(role.permissions))
    setDialogOpen(true)
  }
  const togglePerm = (key: string, checked: boolean) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (checked) next.add(key)
      else next.delete(key)
      return next
    })
  }

  const invalidate = () => void qc.invalidateQueries({ queryKey: ["org-roles", org?.id] })

  const save = useMutation({
    // Create: POST {name, description, permissions}; edit: PUT /{roleId} {description, permissions}.
    mutationFn: () =>
      editing
        ? apiFetch(`/organizations/${org?.id}/roles/${editing.id}`, {
            method: "PUT",
            body: { description, permissions: [...selected] },
          })
        : apiFetch(`/organizations/${org?.id}/roles`, {
            method: "POST",
            body: { name: name.trim(), description, permissions: [...selected] },
          }),
    onSuccess: () => {
      toast.success(editing ? "Role updated" : "Role created")
      setDialogOpen(false)
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const remove = useMutation({
    mutationFn: (role: OrgRole) =>
      apiFetch(`/organizations/${org?.id}/roles/${role.id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Role deleted")
      setDeleting(null)
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const err = (orgError ?? error) as Error | null

  return (
    <>
      <PageHeader
        title="Roles"
        description="Built-in and custom roles that control what members can do in this organization."
        actions={
          <Button size="sm" onClick={openCreate} disabled={!org}>
            <Plus className="size-4" /> Create role
          </Button>
        }
      />

      {orgLoading || isLoading ? (
        <Skeleton className="h-64" />
      ) : err ? (
        <div className="rounded-md border bg-muted/40 p-4 text-sm text-muted-foreground">{err.message}</div>
      ) : !roles?.length ? (
        <EmptyState icon={ShieldCheck} title="No roles" hint="Create a custom role to grant fine-grained access." />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Description</TableHead>
                <TableHead>Permissions</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {roles.map((role) => (
                <TableRow key={role.id}>
                  <TableCell className="font-medium">
                    {role.name}
                    {role.builtIn ? (
                      <Badge variant="secondary" className="ml-2">
                        Built-in
                      </Badge>
                    ) : null}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{role.description ?? "—"}</TableCell>
                  <TableCell className="text-sm">
                    {role.permissions.length}
                    <span className="ml-1 text-xs text-muted-foreground">
                      ({role.expandedPermissions.length} expanded)
                    </span>
                  </TableCell>
                  <TableCell className="text-right">
                    {role.builtIn ? (
                      <span className="text-xs text-muted-foreground">Not editable</span>
                    ) : (
                      <>
                        <Button variant="ghost" size="sm" onClick={() => openEdit(role)}>
                          <Pencil className="size-4" /> Edit
                        </Button>
                        <Button variant="ghost" size="sm" onClick={() => setDeleting(role)}>
                          <Trash2 className="size-4" /> Delete
                        </Button>
                      </>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{editing ? `Edit role ${editing.name}` : "Create role"}</DialogTitle>
            <DialogDescription>
              {editing
                ? "Change the description and permission set of this custom role."
                : "A custom role you can assign to organization members."}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4">
            {!editing ? (
              <div>
                <Label className="mb-1.5 block">Name</Label>
                <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="billing-viewer" />
              </div>
            ) : null}
            <div>
              <Label className="mb-1.5 block">Description</Label>
              <Input
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="What this role is for"
              />
            </div>
            <div>
              <Label className="mb-1.5 block">Permissions</Label>
              {!permissions?.length ? (
                <p className="text-sm text-muted-foreground">Loading permission catalog…</p>
              ) : (
                <div className="max-h-64 space-y-3 overflow-y-auto rounded-md border p-3">
                  {permGroups.map(([group, perms]) => (
                    <div key={group}>
                      <p className="mb-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                        {group}
                      </p>
                      <div className="space-y-1.5">
                        {perms.map((p) => (
                          <label key={p.key} className="flex items-start gap-2 text-sm">
                            <Checkbox
                              className="mt-0.5"
                              checked={selected.has(p.key)}
                              onCheckedChange={(c) => togglePerm(p.key, c === true)}
                            />
                            <span>
                              <span className="font-mono text-xs">{p.key}</span>
                              {p.description ? (
                                <span className="block text-xs text-muted-foreground">{p.description}</span>
                              ) : null}
                            </span>
                          </label>
                        ))}
                      </div>
                    </div>
                  ))}
                </div>
              )}
              <p className="mt-1.5 text-xs text-muted-foreground">{selected.size} selected</p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              onClick={() => save.mutate()}
              disabled={(!editing && !name.trim()) || save.isPending}
            >
              {save.isPending ? "Saving…" : editing ? "Save role" : "Create role"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete role</DialogTitle>
            <DialogDescription>
              Delete the role {deleting?.name}? Members assigned to it lose its permissions.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => deleting && remove.mutate(deleting)}
              disabled={remove.isPending}
            >
              {remove.isPending ? "Deleting…" : "Delete role"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
