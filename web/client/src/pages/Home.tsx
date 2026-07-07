import { useEffect, useState } from "react"
import { useNavigate } from "react-router-dom"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { apiFetch } from "@/lib/api"
import { useProjects } from "@/lib/hooks"
import type { Organization, Project } from "@/lib/types"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

// "/" — route to the first project, or walk a brand-new user through
// organization + project creation.
export function HomePage() {
  const { data: projects, isLoading } = useProjects()
  const navigate = useNavigate()

  useEffect(() => {
    if (projects && projects.length > 0) navigate(`/p/${projects[0].id}/dashboard`, { replace: true })
  }, [projects, navigate])

  if (isLoading) return <div className="flex min-h-screen items-center justify-center text-muted-foreground">Loading…</div>
  if (projects && projects.length > 0) return null
  return <Onboarding />
}

function Onboarding() {
  const [orgName, setOrgName] = useState("")
  const [projectName, setProjectName] = useState("")
  const navigate = useNavigate()
  const qc = useQueryClient()

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
