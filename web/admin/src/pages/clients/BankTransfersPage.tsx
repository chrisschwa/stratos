import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Check, Landmark, X } from "lucide-react"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { useAdminList } from "@/lib/hooks"
import { fmtDateTime, fmtMoney } from "@/lib/format"

// GET /admin/bank-transfer?integrationId= — the transfers of ONE BankTransfer gateway integration
// (the route always filters by paymentGatewayId==integrationId, so a gateway must be selected).
// Approve/Reject settle via POST /admin/bank-transfer/{id}/approve | /reject (processAddFunds:
// approve credits the account, reject marks the transaction FAILED).
type Row = Record<string, any>

// The list returns raw docs (no _id→id shaping) — normalize the key.
function rowId(r: Row): string {
  return String(r.id ?? r._id ?? "")
}

type PendingDecision = { id: string; action: "approve" | "reject" } | null

export default function BankTransfersPage() {
  const qc = useQueryClient()
  const integrations = useAdminList<Row>("/admin/integrations")
  const gateways = (integrations.data?.data ?? []).filter((i) => i.thirdParty === "BankTransfer")
  const [selected, setSelected] = useState("")
  const gw = selected || gateways[0]?.id || ""

  const listPath = `/admin/bank-transfer?integrationId=${gw}`
  const transfers = useAdminList<Row>(listPath, !!gw)
  const rows = transfers.data?.data ?? []
  const [pending, setPending] = useState<PendingDecision>(null)

  const decide = useMutation({
    mutationFn: ({ id, action }: { id: string; action: "approve" | "reject" }) =>
      apiFetch(`/admin/bank-transfer/${id}/${action}`, { method: "POST" }),
    onSuccess: (_d, { action }) => {
      toast.success(action === "approve" ? "Transfer approved — funds credited" : "Transfer rejected")
      void qc.invalidateQueries({ queryKey: ["admin-list", listPath] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <>
      <PageHeader
        title="Bank transfers"
        description="When a customer deposits via a Bank Transfer payment gateway, a pending transfer with a reference number appears here for an operator to approve (credits the account) or reject. Select a Bank Transfer gateway above; if none exists, install one under Integrations."
        actions={
          gateways.length > 1 ? (
            <Select value={gw} onValueChange={setSelected}>
              <SelectTrigger className="w-64">
                <SelectValue placeholder="Select a gateway" />
              </SelectTrigger>
              <SelectContent>
                {gateways.map((g) => (
                  <SelectItem key={g.id} value={g.id}>
                    {g.name ?? g.thirdParty ?? g.id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : undefined
        }
      />

      {integrations.isLoading || (!!gw && transfers.isLoading) ? (
        <Skeleton className="h-64" />
      ) : integrations.isError ? (
        <div className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground">
          {(integrations.error as Error).message}
        </div>
      ) : !gw ? (
        <EmptyState
          icon={Landmark}
          title="Select a bank-transfer gateway"
          hint="No Bank Transfer gateway is configured. Install one under Integrations to start receiving bank-transfer deposits here."
        />
      ) : transfers.isError ? (
        <div className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground">
          {(transfers.error as Error).message}
        </div>
      ) : !rows.length ? (
        <EmptyState
          icon={Landmark}
          title="No pending transfers for this gateway"
          hint="When a customer deposits via this Bank Transfer gateway, a pending transfer with its reference number appears here to approve or reject."
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Date</TableHead>
                <TableHead>Billing profile</TableHead>
                <TableHead className="text-right">Amount</TableHead>
                <TableHead>Reference</TableHead>
                <TableHead>Status</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((t) => {
                const id = rowId(t)
                return (
                  <TableRow key={id}>
                    <TableCell className="text-sm text-muted-foreground">{fmtDateTime(t.createdAt)}</TableCell>
                    <TableCell className="font-mono text-xs">{t.billingProfileId ?? "—"}</TableCell>
                    <TableCell className="text-right font-mono text-sm tabular-nums">
                      {fmtMoney(t.amount, t.currency)}
                    </TableCell>
                    <TableCell className="font-mono text-xs">{t.referenceNumber ?? "—"}</TableCell>
                    <TableCell>
                      <StatusBadge status={t.status} />
                    </TableCell>
                    <TableCell className="text-right">
                      {t.status === "PENDING" && (
                        <div className="flex justify-end gap-2">
                          <Button
                            size="sm"
                            disabled={decide.isPending}
                            onClick={() => setPending({ id, action: "approve" })}
                          >
                            <Check className="size-4" /> Approve
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={decide.isPending}
                            onClick={() => setPending({ id, action: "reject" })}
                          >
                            <X className="size-4" /> Reject
                          </Button>
                        </div>
                      )}
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
            <DialogTitle>{pending?.action === "approve" ? "Approve transfer" : "Reject transfer"}</DialogTitle>
            <DialogDescription>
              {pending?.action === "approve"
                ? "Approving settles the deposit — the client's account is credited. This cannot be undone."
                : "Rejecting marks the deposit transaction as failed. No credit is granted."}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPending(null)}>
              Cancel
            </Button>
            <Button
              variant={pending?.action === "reject" ? "destructive" : "default"}
              disabled={decide.isPending}
              onClick={() => {
                if (pending) decide.mutate(pending)
                setPending(null)
              }}
            >
              {pending?.action === "approve" ? "Approve" : "Reject"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
