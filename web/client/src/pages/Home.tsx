import { useEffect, useState } from "react"
import { useNavigate } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { apiFetch } from "@/lib/api"
import { useProjects } from "@/lib/hooks"
import type { Organization, Project } from "@/lib/types"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

// "/" — route to the first project; if the user is in an organization but has no
// project yet (e.g. added to an org, awaiting a project assignment), show a waiting
// state; otherwise walk a brand-new user through organization + project creation.
type Invite = { token: string; projectId: string; projectName?: string; expiresAt?: string }

export function HomePage() {
  const { data: projects, isLoading } = useProjects()
  const { data: orgs, isLoading: orgsLoading } = useQuery({
    queryKey: ["organizations"],
    queryFn: () => apiFetch<Organization[]>("/organizations"),
  })
  const { data: invites, isLoading: invLoading } = useQuery({
    queryKey: ["my-invites"],
    queryFn: () => apiFetch<Invite[]>("/project-invites/mine"),
  })
  const navigate = useNavigate()

  useEffect(() => {
    if (projects && projects.length > 0) navigate(`/p/${projects[0].id}/dashboard`, { replace: true })
  }, [projects, navigate])

  if (isLoading || orgsLoading || invLoading) return <div className="flex min-h-screen items-center justify-center text-muted-foreground">Loading…</div>
  if (projects && projects.length > 0) return null
  // Pending invitations (logged in directly without the email link) → let them accept here.
  if (invites && invites.length > 0) return <PendingInvites invites={invites} />
  // Member of an organization but no project yet → NOT the create-org onboarding.
  if (orgs && orgs.length > 0) return <NoProjectYet orgs={orgs} />
  return <Onboarding />
}

// Accept a pending project invitation here (no email link needed). Accepting adds the user to
// the project + its organization, after which the projects query repopulates and routes them in.
function PendingInvites({ invites }: { invites: Invite[] }) {
  const qc = useQueryClient()
  const accept = useMutation({
    mutationFn: (token: string) => apiFetch(`/project-invites/accept/${token}`, { method: "POST" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["projects"] })
      void qc.invalidateQueries({ queryKey: ["organizations"] })
      void qc.invalidateQueries({ queryKey: ["my-invites"] })
      toast.success("Invitation accepted")
    },
    onError: (e: Error) => toast.error(e.message),
  })
  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle className="font-display text-xl">Pending invitations</CardTitle>
          <CardDescription>You've been invited to join the following project(s).</CardDescription>
          <div className="horizon mt-2" />
        </CardHeader>
        <CardContent className="space-y-3">
          {invites.map((inv) => (
            <div key={inv.token} className="flex items-center justify-between gap-3 rounded-lg border p-3">
              <span className="text-sm font-medium">{inv.projectName ?? "Project"}</span>
              <Button size="sm" disabled={accept.isPending} onClick={() => accept.mutate(inv.token)}>
                {accept.isPending ? "Accepting…" : "Accept"}
              </Button>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  )
}

// The user belongs to an organization but has no project assigned. The client is
// project-scoped, so there is nothing to land on yet — tell them to contact their admin.
function NoProjectYet({ orgs }: { orgs: Organization[] }) {
  const names = orgs.map((o) => o.name).filter(Boolean).join(", ")
  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle className="font-display text-xl">Welcome to Stratos</CardTitle>
          <CardDescription>
            You're a member of {names || "your organization"}, but no project has been assigned to
            you yet. Please contact your organization admin to be added to a project.
          </CardDescription>
          <div className="horizon mt-2" />
        </CardHeader>
        <CardContent>
          <Button variant="outline" className="w-full" onClick={() => window.location.reload()}>
            Refresh
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}

function Onboarding() {
  const [orgName, setOrgName] = useState("")
  const [projectName, setProjectName] = useState("")
  const navigate = useNavigate()
  const qc = useQueryClient()

  // Operator-only mode gate: when the platform org quota locks self-service creation
  // (limit 0 + enabled), show a contact note instead of a form that would only 400.
  const selfService = useQuery({
    queryKey: ["org-self-service"],
    queryFn: () => apiFetch<{ canCreateOrganization?: boolean }>("/organizations/self-service"),
  })

  const create = useMutation({
    mutationFn: async () => {
      const org = await apiFetch<Organization>("/organizations", { method: "POST", body: { name: orgName } })
      const project = await apiFetch<Project>("/project", {
        method: "POST",
        body: { name: projectName, organizationId: org.id },
      })
      return project
    },
    onSuccess: (p) => {
      void qc.invalidateQueries({ queryKey: ["projects"] })
      navigate(`/p/${p.id}/dashboard`)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  if (selfService.isLoading) {
    return <div className="flex min-h-screen items-center justify-center text-muted-foreground">Loading…</div>
  }
  if (selfService.data && selfService.data.canCreateOrganization === false) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background px-6">
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle className="font-display text-xl">Welcome to Stratos</CardTitle>
            <CardDescription>
              Your account isn't part of any project yet. Organizations on this platform are
              created by the operator — please contact support or wait for a project invitation.
            </CardDescription>
            <div className="horizon mt-2" />
          </CardHeader>
        </Card>
      </div>
    )
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle className="font-display text-xl">Welcome to Stratos</CardTitle>
          <CardDescription>Create your organization and first project to get started.</CardDescription>
          <div className="horizon mt-2" />
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="org">Organization name</Label>
            <Input id="org" value={orgName} onChange={(e) => setOrgName(e.target.value)} placeholder="Acme Inc" />
          </div>
          <div className="space-y-2">
            <Label htmlFor="proj">Project name</Label>
            <Input id="proj" value={projectName} onChange={(e) => setProjectName(e.target.value)} placeholder="production" />
          </div>
          <Button
            className="w-full"
            disabled={!orgName || !projectName || create.isPending}
            onClick={() => create.mutate()}
          >
            {create.isPending ? "Creating…" : "Create organization"}
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
