import { useState } from "react"
import { Link } from "react-router-dom"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { BadgeCheck, Check, RefreshCw, X } from "lucide-react"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { useAdminList } from "@/lib/hooks"
import { fmtDateTime } from "@/lib/format"
import { profileName } from "./BillingProfilesPage"

// GET /admin/billing-profile/validations — PENDING billingProfileValidation docs, each joined
// with its shaped billing profile under `billingProfile` (billingprofile.go billingProfileValidations).
// POST /admin/billing-profile/validations/{validationId}/status/{APPROVED|REJECTED} flips the doc;
// the APPROVED branch activates the profile + sends the validation email server-side.
// Document upload/download is a vendor seam server-side, so no document link is rendered.
type ValidationRow = Record<string, any>

const LIST_PATH = "/admin/billing-profile/validations"

type PendingDecision = { id: string; status: "APPROVED" | "REJECTED"; who: string } | null

export default function ValidationsPage() {
  const qc = useQueryClient()
  const { data, isLoading, isError, error, refetch, isFetching } = useAdminList<ValidationRow>(LIST_PATH)
  const rows = data?.data ?? []
  const [pending, setPending] = useState<PendingDecision>(null)

  const decide = useMutation({
    mutationFn: (p: { id: string; status: string }) =>
      apiFetch(`/admin/billing-profile/validations/${p.id}/status/${p.status}`, { method: "POST" }),
    onSuccess: (_d, p) => {
      toast.success(
        p.status === "APPROVED"
          ? "Validation approved — the billing profile has been activated"
          : "Validation rejected",
      )
      void qc.invalidateQueries({ queryKey: ["admin-list", LIST_PATH] })
    },
    // Surface the exact API error message (e.g. "Billing profile validation not found.").
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <>
      <PageHeader
        title="Validations"
        description="Pending billing-profile identity validations awaiting review."
        actions={
          <Button variant="outline" size="sm" onClick={() => void refetch()} disabled={isFetching}>
            <RefreshCw className={isFetching ? "size-4 animate-spin" : "size-4"} />
          </Button>
        }
      />

      {isLoading ? (
        <Skeleton className="h-64" />
      ) : isError ? (
        <div className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground">{(error as Error).message}</div>
      ) : !rows.length ? (
        <EmptyState
          icon={BadgeCheck}
          title="No pending validations"
          hint="Validation requests appear here when clients submit identity documents and the validation flow is enabled."
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Billing profile</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Submitted</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((v) => {
                const bp = (v.billingProfile ?? {}) as Record<string, any>
                const who = bp.id ? profileName(bp) : (v.billingProfileId as string) || (v.id as string)
                return (
                  <TableRow key={v.id}>
                    <TableCell>
                      {bp.id ? (
                        <Link className="font-medium hover:underline" to={`/clients/billing-profiles/${bp.id}`}>
                          {who}
                        </Link>
                      ) : (
                        <span className="font-medium">{who}</span>
                      )}
                      <p className="text-xs text-muted-foreground">{bp.email ?? "—"}</p>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={v.status} />
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">{fmtDateTime(v.createdAt)}</TableCell>
                    <TableCell className="text-right">
                      <div className="inline-flex gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={decide.isPending}
                          onClick={() => setPending({ id: v.id, status: "APPROVED", who })}
                        >
                          <Check className="size-4" /> Approve
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          className="text-destructive hover:text-destructive"
                          disabled={decide.isPending}
                          onClick={() => setPending({ id: v.id, status: "REJECTED", who })}
                        >
                          <X className="size-4" /> Reject
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </Card>
      )}

      <Dialog open={!!pending} onOpenChange={(o) => !o && setPending(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{pending?.status === "APPROVED" ? "Approve validation" : "Reject validation"}</DialogTitle>
            <DialogDescription>
              {pending?.status === "APPROVED"
                ? `Approve the validation for ${pending?.who}? This activates the billing profile and notifies the client.`
                : `Reject the validation for ${pending?.who}? The client will need to submit again.`}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPending(null)}>
              Cancel
            </Button>
            <Button
              variant={pending?.status === "REJECTED" ? "destructive" : "default"}
              onClick={() => {
                if (pending) decide.mutate({ id: pending.id, status: pending.status })
                setPending(null)
              }}
            >
              {pending?.status === "APPROVED" ? "Approve" : "Reject"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
