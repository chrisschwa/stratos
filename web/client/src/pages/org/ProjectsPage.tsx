import { useState } from "react"
import { useNavigate } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { CreditCard, FolderKanban, MoreHorizontal, Pencil, Plus, Trash2, Undo2 } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { useProjectId, useProjects } from "@/lib/hooks"
import type { BillingSummary, Project } from "@/lib/types"
import { useOrg } from "./MembersPage"

export default function ProjectsPage() {
  const pid = useProjectId()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { data: projects, isLoading, error } = useProjects()
  const { org } = useOrg(pid)

  const [createOpen, setCreateOpen] = useState(false)
  const [newName, setNewName] = useState("")
  const [renaming, setRenaming] = useState<Project | null>(null)
  const [renameName, setRenameName] = useState("")
  const [deleting, setDeleting] = useState<Project | null>(null)
  const [changingBilling, setChangingBilling] = useState<Project | null>(null)
  const [targetBp, setTargetBp] = useState("")

  // Billing profiles the caller can read (GET /billing-profile → billing.Summary list).
  const billingProfiles = useQuery({
    queryKey: ["billing-profiles"],
    queryFn: () => apiFetch<BillingSummary[]>("/billing-profile"),
    enabled: !!changingBilling,
  })

  const invalidate = () => void qc.invalidateQueries({ queryKey: ["projects"] })

  const create = useMutation({
    mutationFn: () =>
      apiFetch<Project>(`/project`, { method: "POST", body: { name: newName.trim(), organizationId: org?.id } }),
    onSuccess: (p) => {
      toast.success(`Project ${p.name} created`)
      setCreateOpen(false)
      setNewName("")
      invalidate()
      if (p?.id) navigate(`/p/${p.id}/dashboard`)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const rename = useMutation({
    mutationFn: (p: Project) =>
      apiFetch(`/project/${p.id}/rename`, { method: "POST", body: { name: renameName.trim() } }),
    onSuccess: () => {
      toast.success("Project renamed")
      setRenaming(null)
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const remove = useMutation({
    // DELETE /project/{id} schedules deletion (cancellable for ~5 minutes via /cancel).
    mutationFn: (p: Project) => apiFetch(`/project/${p.id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Project scheduled for deletion — you can cancel it for the next 5 minutes")
      setDeleting(null)
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const cancelDeletion = useMutation({
    mutationFn: (p: Project) => apiFetch(`/project/${p.id}/cancel`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Deletion cancelled")
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  // DELETE /project/{id}/now — immediate delete, no 5-minute grace window.
  const removeNow = useMutation({
    mutationFn: (p: Project) => apiFetch(`/project/${p.id}/now`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Project deleted")
      setDeleting(null)
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  // POST /project/{id}/billing/{bpId} — reassign the project's billing profile.
  const changeBilling = useMutation({
    mutationFn: ({ p, bpId }: { p: Project; bpId: string }) =>
      apiFetch(`/project/${p.id}/billing/${bpId}`, { method: "POST" }),
    onSuccess: () => {
      toast.success("Billing profile changed")
      setChangingBilling(null)
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const bpLabel = (bp: BillingSummary) =>
    `${bp.fullName || bp.id || "Unnamed"}${bp.currency ? ` · ${bp.currency}` : ""}${bp.status ? ` · ${bp.status}` : ""}`

  return (
    <>
      <PageHeader
        title="Projects"
        description="All projects in this organization."
        actions={
          <Button size="sm" onClick={() => setCreateOpen(true)} disabled={!org}>
            <Plus className="size-4" /> Create project
          </Button>
        }
      />

      {isLoading ? (
        <Skeleton className="h-64" />
      ) : error ? (
        <div className="rounded-md border bg-muted/40 p-4 text-sm text-muted-foreground">
          {(error as Error).message}
        </div>
      ) : !projects?.length ? (
        <EmptyState
          icon={FolderKanban}
          title="No projects yet"
          hint="Create a project to start provisioning cloud resources."
          action={
            <Button onClick={() => setCreateOpen(true)} disabled={!org}>
              <Plus className="size-4" /> Create project
            </Button>
          }
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>ID</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {projects.map((p) => (
                <TableRow key={p.id}>
                  <TableCell className="font-medium">
                    <button className="hover:underline" onClick={() => navigate(`/p/${p.id}/dashboard`)}>
                      {p.name}
                    </button>
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={p.status} />
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{p.id}</TableCell>
                  <TableCell className="text-right">
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="sm">
                          <MoreHorizontal className="size-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem
                          onClick={() => {
                            setRenaming(p)
                            setRenameName(p.name)
                          }}
                        >
                          <Pencil className="size-4" /> Rename
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          onClick={() => {
                            setTargetBp("")
                            setChangingBilling(p)
                          }}
                        >
                          <CreditCard className="size-4" /> Change billing profile
                        </DropdownMenuItem>
                        {p.status === "SCHEDULED_FOR_DELETION" ? (
                          <DropdownMenuItem onClick={() => cancelDeletion.mutate(p)}>
                            <Undo2 className="size-4" /> Cancel deletion
                          </DropdownMenuItem>
                        ) : (
                          <DropdownMenuItem variant="destructive" onClick={() => setDeleting(p)}>
                            <Trash2 className="size-4" /> Delete
                          </DropdownMenuItem>
                        )}
                      </DropdownMenuContent>
                    </DropdownMenu>
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
            <DialogTitle>Create project</DialogTitle>
            <DialogDescription>A new project in {org?.name ?? "your organization"}.</DialogDescription>
          </DialogHeader>
          <div>
            <Label className="mb-1.5 block">Project name</Label>
            <Input value={newName} onChange={(e) => setNewName(e.target.value)} placeholder="my-project" />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => create.mutate()} disabled={!newName.trim() || !org || create.isPending}>
              {create.isPending ? "Creating…" : "Create project"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!renaming} onOpenChange={(o) => !o && setRenaming(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Rename project</DialogTitle>
          </DialogHeader>
          <div>
            <Label className="mb-1.5 block">New name</Label>
            <Input value={renameName} onChange={(e) => setRenameName(e.target.value)} />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRenaming(null)}>
              Cancel
            </Button>
            <Button
              onClick={() => renaming && rename.mutate(renaming)}
              disabled={!renameName.trim() || rename.isPending}
            >
              {rename.isPending ? "Renaming…" : "Rename"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete project</DialogTitle>
            <DialogDescription>
              Delete {deleting?.name}? Scheduling gives you 5 minutes to cancel before its cloud resources are
              removed. "Delete now" skips the grace window and removes everything immediately.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            <Button
              variant="outline"
              className="border-destructive/40 text-destructive hover:text-destructive"
              onClick={() => deleting && removeNow.mutate(deleting)}
              disabled={removeNow.isPending || remove.isPending}
            >
              {removeNow.isPending ? "Deleting…" : "Delete now"}
            </Button>
            <Button
              variant="destructive"
              onClick={() => deleting && remove.mutate(deleting)}
              disabled={remove.isPending || removeNow.isPending}
            >
              {remove.isPending ? "Deleting…" : "Schedule deletion"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!changingBilling} onOpenChange={(o) => !o && setChangingBilling(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Change billing profile</DialogTitle>
            <DialogDescription>
              Pick the billing profile that {changingBilling?.name} should be charged against.
            </DialogDescription>
          </DialogHeader>
          <div>
            <Label className="mb-1.5 block">Billing profile</Label>
            {billingProfiles.isLoading ? (
              <Skeleton className="h-9" />
            ) : billingProfiles.error ? (
              <p className="text-sm text-muted-foreground">{(billingProfiles.error as Error).message}</p>
            ) : !billingProfiles.data?.length ? (
              <p className="text-sm text-muted-foreground">No billing profiles available.</p>
            ) : (
              <Select value={targetBp} onValueChange={setTargetBp}>
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="Pick a billing profile" />
                </SelectTrigger>
                <SelectContent>
                  {billingProfiles.data.map((bp) => (
                    <SelectItem key={String(bp.id)} value={String(bp.id)}>
                      {bpLabel(bp)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setChangingBilling(null)}>
              Cancel
            </Button>
            <Button
              onClick={() => changingBilling && changeBilling.mutate({ p: changingBilling, bpId: targetBp })}
              disabled={!targetBp || changeBilling.isPending}
            >
              {changeBilling.isPending ? "Changing…" : "Change billing profile"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
