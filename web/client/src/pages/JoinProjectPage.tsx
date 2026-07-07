import { useNavigate, useParams, useSearchParams } from "react-router-dom"
import { useMutation, useQuery } from "@tanstack/react-query"
import { toast } from "sonner"
import { CheckCircle2, MailX, UserPlus } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { apiFetch } from "@/lib/api"
import { fmtDateTime } from "@/lib/format"

// GET /project-invites/{token} → the invite doc for the CALLER's email + token,
// or {} when there is no matching (or an expired) invite.
type Invite = {
  email?: string
  projectId?: string
  expiresAt?: string
}

export default function JoinProjectPage() {
  const { token: tokenParam = "" } = useParams()
  const [searchParams] = useSearchParams()
  // email deep-link = /join-project?invite-token=…; direct route = /join/:token
  const token = tokenParam || searchParams.get("invite-token") || ""
  const navigate = useNavigate()

  const { data: invite, isLoading, error } = useQuery({
    queryKey: ["project-invite", token],
    queryFn: () => apiFetch<Invite>(`/project-invites/${encodeURIComponent(token)}`),
    enabled: !!token,
  })

  const accept = useMutation({
    mutationFn: () => apiFetch(`/project-invites/accept/${encodeURIComponent(token)}`, { method: "POST" }),
    onSuccess: () => {
      toast.success("Invite accepted — welcome to the project")
      navigate("/")
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const decline = useMutation({
    mutationFn: () => apiFetch(`/project-invites/decline/${encodeURIComponent(token)}`, { method: "POST" }),
    onSuccess: () => {
      toast.success("Invite declined")
      navigate("/")
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const valid = !!invite?.projectId

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-4">
      <Card className="w-full max-w-md">
        {isLoading ? (
          <CardContent className="py-8">
            <Skeleton className="h-32" />
          </CardContent>
        ) : error ? (
          <CardContent className="py-8 text-center text-sm text-muted-foreground">
            {(error as Error).message}
          </CardContent>
        ) : !valid ? (
          <>
            <CardHeader className="items-center text-center">
              <MailX className="mx-auto mb-2 size-10 text-muted-foreground/60" strokeWidth={1.5} />
              <CardTitle>Invite not found</CardTitle>
            </CardHeader>
            <CardContent className="text-center">
              <p className="text-sm text-muted-foreground">
                This invitation is invalid, has expired, or was sent to a different email address than the
                account you are signed in with.
              </p>
              <Button className="mt-6" onClick={() => navigate("/")}>
                Go to console
              </Button>
            </CardContent>
          </>
        ) : (
          <>
            <CardHeader className="items-center text-center">
              <UserPlus className="mx-auto mb-2 size-10 text-primary" strokeWidth={1.5} />
              <CardTitle>Project invitation</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-center text-sm text-muted-foreground">
                You have been invited to join a project. Accepting adds you to the project and its
                organization.
              </p>
              <dl className="mt-6 space-y-2 rounded-md border bg-muted/30 p-4 text-sm">
                <div className="flex justify-between gap-4">
                  <dt className="text-muted-foreground">Invited email</dt>
                  <dd className="font-medium">{invite?.email ?? "—"}</dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt className="text-muted-foreground">Project</dt>
                  <dd className="font-mono text-xs">{invite?.projectId}</dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt className="text-muted-foreground">Expires</dt>
                  <dd>{fmtDateTime(invite?.expiresAt)}</dd>
                </div>
              </dl>
              <div className="mt-6 flex justify-center gap-3">
                <Button
                  variant="outline"
                  onClick={() => decline.mutate()}
                  disabled={decline.isPending || accept.isPending}
                >
                  {decline.isPending ? "Declining…" : "Decline"}
                </Button>
                <Button onClick={() => accept.mutate()} disabled={accept.isPending || decline.isPending}>
                  <CheckCircle2 className="size-4" />
                  {accept.isPending ? "Accepting…" : "Accept invite"}
                </Button>
              </div>
            </CardContent>
          </>
        )}
      </Card>
    </div>
  )
}
