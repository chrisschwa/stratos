import { useState } from "react"
import { useParams } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Download, Wallet } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { fmtDate, fmtDateTime, fmtMoney } from "@/lib/format"
import { useBillingSummary, useProjectId, useProjects } from "@/lib/hooks"
import type { Bill, Transaction } from "@/lib/types"
import { downloadPdf } from "./HistoryPage"

export default function BillDetailPage() {
  const pid = useProjectId()
  const qc = useQueryClient()
  const { billId = "" } = useParams()
  const { data: summary } = useBillingSummary(pid)
  const bp = summary?.id
  const [payOpen, setPayOpen] = useState(false)
  // The bill is org (billing-profile) scoped — all projects' line items. Resolve + filter by project
  // so an org admin can read per-project spend on the same bill.
  const { data: projects } = useProjects()
  const [projFilter, setProjFilter] = useState("__all__")
  const projName = (id?: string) => projects?.find((p) => p.id === id)?.name || id || "—"

  const { data: bill, isLoading, error } = useQuery({
    queryKey: ["bill", bp, billId],
    queryFn: () => apiFetch<Bill>(`/bill/${bp}/${billId}`),
    enabled: !!bp && !!billId,
  })
  const { data: txns } = useQuery({
    queryKey: ["bill-transactions", bp, billId],
    queryFn: () => apiFetch<Transaction[]>(`/collect-transactions/${bp}/bill/${billId}`),
    enabled: !!bp && !!billId,
  })

  // Pay a SENT bill from the profile's credit balance (POST /payment/{bp}/bill/{billId}/pay,
  // no body; 400s: already-paid / open-bill / not-enough-credit).
  const payBill = useMutation({
    mutationFn: () => apiFetch(`/payment/${bp}/bill/${billId}/pay`, { method: "POST" }),
    onSuccess: () => {
      toast.success("Bill paid from balance")
      setPayOpen(false)
      void qc.invalidateQueries({ queryKey: ["bill", bp, billId] })
      void qc.invalidateQueries({ queryKey: ["bills", bp] })
      void qc.invalidateQueries({ queryKey: ["bill-transactions", bp, billId] })
      void qc.invalidateQueries({ queryKey: ["billing-summary", pid] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  if (isLoading || !bp) {
    return (
      <>
        <PageHeader title="Bill" />
        <Skeleton className="h-72" />
      </>
    )
  }
  if (error || !bill) {
    return (
      <>
        <PageHeader title="Bill" />
        <div className="rounded-md border bg-muted/40 p-4 text-sm text-muted-foreground">
          {(error as Error | null)?.message ?? "Bill not found."}
        </div>
      </>
    )
  }

  const ccy = bill.invoiceCurrency
  const payable = bill.status === "SENT" && Number(bill.unpaidGrossAmount ?? 0) > 0
  const adjustments = (bill.adjustments as Array<Record<string, any>> | undefined) ?? []
  const promoCredits = (bill.appliedPromotionalCredits as Array<Record<string, any>> | undefined) ?? []
  const accountCredits = (bill.appliedAccountCredits as Array<Record<string, any>> | undefined) ?? []
  const appliedCredits = [...promoCredits, ...accountCredits]

  const allItems = bill.items ?? []
  const projectIds = [...new Set(allItems.map((it) => it.projectId as string).filter(Boolean))]
  const items = projFilter === "__all__" ? allItems : allItems.filter((it) => (it.projectId as string) === projFilter)

  return (
    <>
      <PageHeader
        title={`Bill ${fmtDate(bill.createdAt)}`}
        description={`Created ${fmtDateTime(bill.createdAt)}`}
        actions={
          <>
            {payable ? (
              <Button size="sm" onClick={() => setPayOpen(true)}>
                <Wallet className="size-4" /> Pay with balance
              </Button>
            ) : null}
            <Button
              variant="outline"
              size="sm"
              onClick={() =>
                downloadPdf(`/bill/${bp}/download/${billId}/statement`, `bill-${billId}-statement.pdf`).catch(
                  (e: Error) => toast.error(e.message),
                )
              }
            >
              <Download className="size-4" /> Download statement
            </Button>
          </>
        }
      />

      <Dialog open={payOpen} onOpenChange={setPayOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Pay bill with balance</DialogTitle>
            <DialogDescription>
              Pay the unpaid {fmtMoney(bill.unpaidGrossAmount, ccy)} on this bill from your credit balance?
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPayOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => payBill.mutate()} disabled={payBill.isPending}>
              {payBill.isPending ? "Paying…" : "Pay bill"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <div className="mb-4 flex flex-wrap items-center gap-x-6 gap-y-2">
        <StatusBadge status={bill.status} />
        <Total label="Net" value={fmtMoney(bill.netAmount, ccy)} />
        <Total label="Gross" value={fmtMoney(bill.grossAmount, ccy)} />
        <Total label="Unpaid" value={fmtMoney(bill.unpaidGrossAmount, ccy)} />
      </div>

      <div className="mb-2 flex items-center justify-between gap-2">
        <h2 className="text-sm font-medium text-muted-foreground">Line items</h2>
        {projectIds.length > 1 ? (
          <Select value={projFilter} onValueChange={setProjFilter}>
            <SelectTrigger className="w-56">
              <SelectValue placeholder="All projects" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__all__">All projects</SelectItem>
              {projectIds.map((id) => (
                <SelectItem key={id} value={id}>
                  {projName(id)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : null}
      </div>

      <Card className="overflow-hidden py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Item</TableHead>
              <TableHead>Project</TableHead>
              <TableHead>Resource type</TableHead>
              <TableHead className="text-right">Net</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.length ? (
              items.map((it, i) => (
                <TableRow key={i}>
                  <TableCell className="font-medium">{(it.name as string) ?? (it.resourceId as string) ?? "—"}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{projName(it.projectId as string)}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{(it.resourceType as string) ?? "—"}</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">
                    {fmtMoney(it.netAmount as number, (it.currency as string) ?? ccy)}
                  </TableCell>
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={4} className="py-6 text-center text-sm text-muted-foreground">
                  No items on this bill.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </Card>

      {adjustments.length > 0 ? (
        <Card className="mt-4">
          <CardHeader>
            <CardTitle className="text-base">Adjustments</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {adjustments.map((a, i) => (
              <div key={i} className="flex items-center justify-between border-b pb-2 text-sm last:border-0">
                <span>{(a.description as string) ?? (a.type as string) ?? "Adjustment"}</span>
                <span className="font-mono tabular-nums">{fmtMoney(a.amount as number, ccy)}</span>
              </div>
            ))}
          </CardContent>
        </Card>
      ) : null}

      {appliedCredits.length > 0 ? (
        <Card className="mt-4">
          <CardHeader>
            <CardTitle className="text-base">Applied credits</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {appliedCredits.map((c, i) => (
              <div key={i} className="flex items-center justify-between border-b pb-2 text-sm last:border-0">
                <span>{(c.code as string) ?? (c.accountCreditId as string) ?? "Credit"}</span>
                <span className="font-mono tabular-nums">{fmtMoney(c.amount as number, ccy)}</span>
              </div>
            ))}
          </CardContent>
        </Card>
      ) : null}

      <Card className="mt-4 overflow-hidden py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Transaction</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Date</TableHead>
              <TableHead className="text-right">Amount</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {txns?.length ? (
              txns.map((t) => (
                <TableRow key={t.id}>
                  <TableCell className="font-mono text-xs">{(t.externalId as string) ?? t.id}</TableCell>
                  <TableCell>
                    <StatusBadge status={t.status} />
                  </TableCell>
                  <TableCell>{fmtDateTime(t.createdAt)}</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">
                    {fmtMoney(t.grossAmount, t.currency)}
                  </TableCell>
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={4} className="py-6 text-center text-sm text-muted-foreground">
                  No payments recorded for this bill.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </Card>
    </>
  )
}

function Total({ label, value }: { label: string; value: string }) {
  return (
    <span className="text-sm">
      <span className="text-muted-foreground">{label} </span>
      <span className="font-mono font-medium tabular-nums">{value}</span>
    </span>
  )
}
