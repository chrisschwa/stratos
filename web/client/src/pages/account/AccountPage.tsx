import { useState } from "react"
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { ExternalLink, KeyRound, Pencil, Plus, ScrollText, Trash2 } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch, apiFetchEnvelope, apiFetchRaw } from "@/lib/api"
import { config } from "@/lib/config"
import { fmtDate, fmtDateTime } from "@/lib/format"

// GET /account/details — RAW endpoint (AccountDetailsDTO, no envelope).
type AccountDetails = {
  id?: string
  sub: string
  createdAt?: string
  customInfo?: Record<string, unknown>
  firstName?: string
  lastName?: string
  email?: string
  language?: string
  gravatarUrl?: string
}

type AuditEvent = {
  id?: string
  timestamp?: string
  action?: string
  resourceType?: string
  resourceDisplayName?: string
  outcome?: string
  actor?: { displayName?: string; ipAddress?: string }
}

const AUDIT_LIMIT = 25

export default function AccountPage() {
  const qc = useQueryClient()

  const { data: details, isLoading, error } = useQuery({
    queryKey: ["account-details"],
    queryFn: () => apiFetchRaw<AccountDetails>("/account/details"),
  })

  // Security log: GET /account/audit — cursor-paged CursorList envelope.
  const audit = useInfiniteQuery({
    queryKey: ["account-audit"],
    queryFn: ({ pageParam }) =>
      apiFetchEnvelope<AuditEvent[]>(
        `/account/audit?limit=${AUDIT_LIMIT}${pageParam ? `&after=${encodeURIComponent(pageParam)}` : ""}`,
      ),
    initialPageParam: "",
    getNextPageParam: (last) => (last.paging as { nextMarker?: string } | undefined)?.nextMarker ?? undefined,
  })
  const events = audit.data?.pages.flatMap((p) => p.data ?? []) ?? []

  // Edit name dialog — POST /account/name {firstName, lastName} (raw response).
  const [editOpen, setEditOpen] = useState(false)
  const [firstName, setFirstName] = useState("")
  const [lastName, setLastName] = useState("")
  const saveName = useMutation({
    mutationFn: () =>
      apiFetchRaw("/account/name", { method: "POST", body: { firstName: firstName.trim(), lastName: lastName.trim() } }),
    onSuccess: () => {
      toast.success("Name updated")
      setEditOpen(false)
      void qc.invalidateQueries({ queryKey: ["account-details"] })
      void qc.invalidateQueries({ queryKey: ["account-audit"] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  // Custom info — POST /user/custom-info/{key}?value=… / DELETE /user/custom-info/{key} (enveloped).
  const [addOpen, setAddOpen] = useState(false)
  const [ciKey, setCiKey] = useState("")
  const [ciValue, setCiValue] = useState("")
  const [ciDeleting, setCiDeleting] = useState<string | null>(null)
  const addInfo = useMutation({
    mutationFn: () =>
      apiFetch(`/user/custom-info/${encodeURIComponent(ciKey.trim())}?value=${encodeURIComponent(ciValue)}`, {
        method: "POST",
      }),
    onSuccess: () => {
      toast.success("Custom info saved")
      setAddOpen(false)
      setCiKey("")
      setCiValue("")
      void qc.invalidateQueries({ queryKey: ["account-details"] })
    },
    onError: (e: Error) => toast.error(e.message),
  })
  const deleteInfo = useMutation({
    mutationFn: (key: string) => apiFetch(`/user/custom-info/${encodeURIComponent(key)}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Custom info removed")
      setCiDeleting(null)
      void qc.invalidateQueries({ queryKey: ["account-details"] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const customInfo = Object.entries(details?.customInfo ?? {})
  const fullName = [details?.firstName, details?.lastName].filter(Boolean).join(" ")
  // config.authIssuer = <keycloak origin>/realms/clients → the Keycloak account console.
  const keycloakAccountUrl = `${config.authIssuer.replace(/\/$/, "")}/account`

  return (
    <>
      <PageHeader title="Account" description="Your profile, security log and custom info." />

      {isLoading ? (
        <Skeleton className="h-64" />
      ) : error ? (
        <div className="rounded-md border bg-muted/40 p-4 text-sm text-muted-foreground">
          {(error as Error).message}
        </div>
      ) : (
        <div className="space-y-6">
          <Card>
            <CardHeader className="flex-row items-center justify-between">
              <CardTitle>Profile</CardTitle>
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setFirstName(details?.firstName ?? "")
                  setLastName(details?.lastName ?? "")
                  setEditOpen(true)
                }}
              >
                <Pencil className="size-4" /> Edit name
              </Button>
            </CardHeader>
            <CardContent className="flex items-start gap-4">
              {details?.gravatarUrl ? (
                <img
                  src={details.gravatarUrl}
                  alt=""
                  className="size-16 shrink-0 rounded-full border"
                />
              ) : null}
              <dl className="grid flex-1 gap-x-8 gap-y-3 text-sm sm:grid-cols-2">
                <div>
                  <dt className="text-muted-foreground">Name</dt>
                  <dd className="font-medium">{fullName || "—"}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Email</dt>
                  <dd className="font-medium">{details?.email ?? "—"}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Subject</dt>
                  <dd className="font-mono text-xs">{details?.sub}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Member since</dt>
                  <dd>{fmtDate(details?.createdAt)}</dd>
                </div>
              </dl>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex-row items-center justify-between">
              <CardTitle>Password & two-factor auth</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-wrap items-center justify-between gap-3">
              <p className="flex items-center gap-2 text-sm text-muted-foreground">
                <KeyRound className="size-4 shrink-0" />
                Password and two-factor authentication are managed by the identity provider.
              </p>
              <Button size="sm" variant="outline" asChild>
                <a href={keycloakAccountUrl} target="_blank" rel="noreferrer">
                  Open account console <ExternalLink className="size-4" />
                </a>
              </Button>
            </CardContent>
          </Card>

          <Card className="gap-0 overflow-hidden pb-0">
            <CardHeader className="flex-row items-center justify-between pb-4">
              <CardTitle>Custom info</CardTitle>
              <Button size="sm" variant="outline" onClick={() => setAddOpen(true)}>
                <Plus className="size-4" /> Add entry
              </Button>
            </CardHeader>
            {!customInfo.length ? (
              <CardContent className="pb-6 text-sm text-muted-foreground">
                No custom info entries. Key/value pairs stored on your user record.
              </CardContent>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Key</TableHead>
                    <TableHead>Value</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {customInfo.map(([k, v]) => (
                    <TableRow key={k}>
                      <TableCell className="font-mono text-xs">{k}</TableCell>
                      <TableCell className="text-sm">{String(v)}</TableCell>
                      <TableCell className="text-right">
                        <Button variant="ghost" size="sm" onClick={() => setCiDeleting(k)}>
                          <Trash2 className="size-4" /> Delete
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Card>

          <div>
            <h2 className="mb-3 font-display text-lg font-semibold">Security log</h2>
            {audit.isLoading ? (
              <Skeleton className="h-40" />
            ) : audit.error ? (
              <div className="rounded-md border bg-muted/40 p-4 text-sm text-muted-foreground">
                {(audit.error as Error).message}
              </div>
            ) : !events.length ? (
              <EmptyState icon={ScrollText} title="No security events" hint="Account activity will show up here." />
            ) : (
              <>
                <Card className="overflow-hidden py-0">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Time</TableHead>
                        <TableHead>Event</TableHead>
                        <TableHead>Outcome</TableHead>
                        <TableHead>IP address</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {events.map((e, i) => (
                        <TableRow key={e.id ?? i}>
                          <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
                            {fmtDateTime(e.timestamp)}
                          </TableCell>
                          <TableCell>
                            <Badge variant="secondary">{e.action ?? "—"}</Badge>
                            {e.resourceType ? (
                              <span className="ml-2 text-xs text-muted-foreground">{e.resourceType}</span>
                            ) : null}
                          </TableCell>
                          <TableCell className="text-sm text-muted-foreground">{e.outcome ?? "—"}</TableCell>
                          <TableCell className="font-mono text-xs text-muted-foreground">
                            {e.actor?.ipAddress ?? "—"}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </Card>
                {audit.hasNextPage ? (
                  <div className="mt-4 text-center">
                    <Button
                      variant="outline"
                      onClick={() => void audit.fetchNextPage()}
                      disabled={audit.isFetchingNextPage}
                    >
                      {audit.isFetchingNextPage ? "Loading…" : "Load more"}
                    </Button>
                  </div>
                ) : null}
              </>
            )}
          </div>
        </div>
      )}

      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit name</DialogTitle>
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <Label className="mb-1.5 block">First name</Label>
              <Input value={firstName} onChange={(e) => setFirstName(e.target.value)} maxLength={100} />
            </div>
            <div>
              <Label className="mb-1.5 block">Last name</Label>
              <Input value={lastName} onChange={(e) => setLastName(e.target.value)} maxLength={100} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => saveName.mutate()} disabled={saveName.isPending}>
              {saveName.isPending ? "Saving…" : "Save name"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add custom info</DialogTitle>
            <DialogDescription>A key/value pair stored on your user record.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4">
            <div>
              <Label className="mb-1.5 block">Key</Label>
              <Input value={ciKey} onChange={(e) => setCiKey(e.target.value)} placeholder="lang" />
            </div>
            <div>
              <Label className="mb-1.5 block">Value</Label>
              <Input value={ciValue} onChange={(e) => setCiValue(e.target.value)} placeholder="en-us" />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAddOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => addInfo.mutate()} disabled={!ciKey.trim() || addInfo.isPending}>
              {addInfo.isPending ? "Saving…" : "Save entry"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!ciDeleting} onOpenChange={(o) => !o && setCiDeleting(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete custom info</DialogTitle>
            <DialogDescription>
              Remove the entry <span className="font-mono">{ciDeleting}</span> from your user record?
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCiDeleting(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => ciDeleting && deleteInfo.mutate(ciDeleting)}
              disabled={deleteInfo.isPending}
            >
              {deleteInfo.isPending ? "Deleting…" : "Delete entry"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
