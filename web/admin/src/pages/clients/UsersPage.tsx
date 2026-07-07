import { useMemo, useState } from "react"
import { useNavigate } from "react-router-dom"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Plus, RefreshCw, Trash2, Users } from "lucide-react"
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
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { fmtDate } from "@/lib/format"
import { useAdminList } from "@/lib/hooks"

// GET /admin/user (handler.go listRaw "users") — raw user docs, shaped _id→id.
export type AdminUser = {
  id?: string
  sub?: string
  email?: string
  firstName?: string
  lastName?: string
  createdAt?: string
}

const LIST_PATH = "/admin/user"

export function userDisplayName(u: AdminUser): string {
  return [u.firstName, u.lastName].filter(Boolean).join(" ") || "—"
}

export default function UsersPage() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { data, isLoading, isFetching, error, refetch } = useAdminList<AdminUser>(LIST_PATH)
  const [q, setQ] = useState("")
  const [createOpen, setCreateOpen] = useState(false)
  const [form, setForm] = useState({ email: "", firstName: "", lastName: "" })
  const [projectIds, setProjectIds] = useState<string[]>([])
  const [toDelete, setToDelete] = useState<AdminUser | null>(null)

  // GET /admin/project (clientarea_reads.go projectAdminList) — for the create-dialog invite picker.
  const projects = useAdminList<{ id?: string; name?: string }>("/admin/project", createOpen)

  const users = data?.data ?? []
  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase()
    if (!needle) return users
    return users.filter((u) =>
      [u.email, u.firstName, u.lastName, u.sub].filter(Boolean).join(" ").toLowerCase().includes(needle),
    )
  }, [users, q])

  const invalidate = () => qc.invalidateQueries({ queryKey: ["admin-list", LIST_PATH] })

  // POST /admin/user (user.go userCreate) — body {firstName, lastName, email, projectIds?}.
  // projectIds fan out as best-effort project invites server-side.
  const createUser = useMutation({
    mutationFn: () =>
      apiFetch(LIST_PATH, {
        method: "POST",
        body: {
          email: form.email,
          firstName: form.firstName,
          lastName: form.lastName,
          ...(projectIds.length ? { projectIds } : {}),
        },
      }),
    onSuccess: () => {
      toast.success("User created")
      setCreateOpen(false)
      setForm({ email: "", firstName: "", lastName: "" })
      setProjectIds([])
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  // DELETE /admin/user/{id} (user.go userDelete).
  const deleteUser = useMutation({
    mutationFn: (id: string) => apiFetch(`/admin/user/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("User deleted")
      setToDelete(null)
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <>
      <PageHeader
        title="Users"
        description="Every registered client account on the platform."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => refetch()} disabled={isFetching}>
              <RefreshCw className={isFetching ? "animate-spin" : ""} />
              Refresh
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus />
              Create user
            </Button>
          </>
        }
      />

      <div className="mb-4 max-w-sm">
        <Input placeholder="Search by email, name or sub…" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>

      {isLoading ? (
        <Skeleton className="h-64" />
      ) : error ? (
        <div className="rounded-lg border bg-muted/40 p-6 text-sm text-muted-foreground">
          {(error as Error).message}
        </div>
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={Users}
          title={q ? "No users match the search" : "No users yet"}
          hint={q ? "Try a different query." : "Create the first user to get started."}
          action={
            q ? undefined : (
              <Button size="sm" onClick={() => setCreateOpen(true)}>
                <Plus />
                Create user
              </Button>
            )
          }
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Email</TableHead>
                <TableHead>Name</TableHead>
                <TableHead>Sub</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="w-10" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((u) => (
                <TableRow
                  key={u.id ?? u.sub}
                  className="cursor-pointer"
                  onClick={() => u.id && navigate(`/clients/users/${u.id}`)}
                >
                  <TableCell className="font-medium">{u.email ?? "—"}</TableCell>
                  <TableCell>{userDisplayName(u)}</TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{u.sub ?? "—"}</TableCell>
                  <TableCell className="text-muted-foreground">{fmtDate(u.createdAt)}</TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Delete user"
                      onClick={(e) => {
                        e.stopPropagation()
                        setToDelete(u)
                      }}
                    >
                      <Trash2 className="text-muted-foreground" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}

      {/* Create user */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create user</DialogTitle>
            <DialogDescription>The user is created without a password; set one from their detail page.</DialogDescription>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault()
              createUser.mutate()
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="user-email">Email</Label>
              <Input
                id="user-email"
                type="email"
                required
                value={form.email}
                onChange={(e) => setForm({ ...form, email: e.target.value })}
              />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-2">
                <Label htmlFor="user-first">First name</Label>
                <Input
                  id="user-first"
                  value={form.firstName}
                  onChange={(e) => setForm({ ...form, firstName: e.target.value })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="user-last">Last name</Label>
                <Input
                  id="user-last"
                  value={form.lastName}
                  onChange={(e) => setForm({ ...form, lastName: e.target.value })}
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label>Invite to projects (optional)</Label>
              {(projects.data?.data ?? []).length === 0 ? (
                <p className="text-xs text-muted-foreground">
                  {projects.isLoading ? "Loading projects…" : "No projects available."}
                </p>
              ) : (
                <div className="max-h-36 space-y-1.5 overflow-y-auto rounded-md border p-2">
                  {(projects.data?.data ?? []).map((p) =>
                    p.id ? (
                      <label key={p.id} className="flex cursor-pointer items-center gap-2 text-sm">
                        <Checkbox
                          checked={projectIds.includes(p.id)}
                          onCheckedChange={(c) =>
                            setProjectIds((prev) =>
                              c === true ? [...prev, p.id!] : prev.filter((x) => x !== p.id),
                            )
                          }
                        />
                        <span>{p.name ?? p.id}</span>
                      </label>
                    ) : null,
                  )}
                </div>
              )}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setCreateOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={createUser.isPending || !form.email}>
                {createUser.isPending ? "Creating…" : "Create user"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete confirm */}
      <Dialog open={!!toDelete} onOpenChange={(o) => !o && setToDelete(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete user</DialogTitle>
            <DialogDescription>
              This permanently deletes {toDelete?.email ?? "this user"}. Users still attached to projects cannot be
              deleted.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setToDelete(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={deleteUser.isPending}
              onClick={() => toDelete?.id && deleteUser.mutate(toDelete.id)}
            >
              {deleteUser.isPending ? "Deleting…" : "Delete user"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
