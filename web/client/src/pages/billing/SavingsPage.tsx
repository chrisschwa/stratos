import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { PiggyBank } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { fmtDate, fmtMoney } from "@/lib/format"
import { useBillingSummary, useProjectId } from "@/lib/hooks"

type Tier = { startAmount?: number | string; discount?: number | string }
type Schedule = {
  durationMonths: number
  maxAmount?: number | string
  noUpfrontTiers?: Tier[]
  upfrontTiers?: Tier[]
}
type SavingsPlan = {
  id: string
  name?: string
  description?: string
  available?: boolean
  targets?: Array<{ resourceType?: string }>
  savingSchedule?: Schedule[]
}
type SavingsContract = {
  id: string
  savingsPlanId?: string
  savingsPlanName?: string
  status?: string
  durationMonths?: number
  monthlyCommittedAmount?: number | string
  discountRate?: number | string
  paidUpfront?: boolean
  startDate?: string
  endDate?: string
}

// Discount values are stored as percentages (e.g. 10 → 10%); shown raw.
function pct(v: number | string | undefined): string {
  if (v === undefined || v === null || v === "") return "—"
  return `${v}%`
}

export default function SavingsPage() {
  const pid = useProjectId()
  const { data: summary, isLoading } = useBillingSummary(pid)
  const bp = summary?.id
  const currency = summary?.currency ?? "USD"
  const [purchasing, setPurchasing] = useState<SavingsPlan | null>(null)

  const { data: plans, isLoading: plansLoading, error: plansError } = useQuery({
    queryKey: ["savings-plans", bp],
    queryFn: () => apiFetch<SavingsPlan[]>(`/savings-plans?billingProfileId=${bp}`),
    enabled: !!bp,
  })

  if (isLoading || !bp) {
    return (
      <>
        <PageHeader title="Savings plans" />
        <Skeleton className="h-64" />
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="Savings plans"
        description="Commit to monthly usage in exchange for a discount on matching resources."
      />

      {plansLoading ? (
        <Skeleton className="h-40" />
      ) : plansError ? (
        <ErrorPanel message={(plansError as Error).message} />
      ) : !plans?.length ? (
        <EmptyState icon={PiggyBank} title="No savings plans available" hint="Plans published by the operator appear here." />
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          {plans.map((p) => (
            <PlanCard key={p.id} plan={p} currency={currency} onPurchase={() => setPurchasing(p)} />
          ))}
        </div>
      )}

      <ContractsSection bp={bp} currency={currency} />

      {purchasing ? (
        <PurchaseDialog
          bp={bp}
          plan={purchasing}
          currency={currency}
          onOpenChange={(o) => !o && setPurchasing(null)}
        />
      ) : null}
    </>
  )
}

function PlanCard({ plan, currency, onPurchase }: { plan: SavingsPlan; currency: string; onPurchase: () => void }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{plan.name ?? plan.id}</CardTitle>
        {plan.description ? <p className="text-sm text-muted-foreground">{plan.description}</p> : null}
      </CardHeader>
      <CardContent className="space-y-3">
        {plan.targets?.length ? (
          <div className="flex flex-wrap gap-1.5">
            {plan.targets.map((t, i) => (
              <Badge key={i} variant="secondary">{t.resourceType ?? "any resource"}</Badge>
            ))}
          </div>
        ) : null}
        {plan.savingSchedule?.length ? (
          <div className="space-y-1 text-sm">
            {plan.savingSchedule.map((s, i) => (
              <div key={i} className="flex items-center justify-between border-b pb-1 last:border-0">
                <span className="text-muted-foreground">{s.durationMonths} months</span>
                <span>
                  {tierSummary(s.noUpfrontTiers, "no upfront")}
                  {s.noUpfrontTiers?.length && s.upfrontTiers?.length ? " · " : ""}
                  {tierSummary(s.upfrontTiers, "upfront")}
                  {s.maxAmount !== undefined ? ` · up to ${fmtMoney(s.maxAmount, currency)}` : ""}
                </span>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">No discount schedule published.</p>
        )}
        <Button size="sm" onClick={onPurchase}>Purchase plan</Button>
      </CardContent>
    </Card>
  )
}

function tierSummary(tiers: Tier[] | undefined, label: string): string {
  if (!tiers?.length) return ""
  const best = tiers[tiers.length - 1]
  return `up to ${pct(best.discount)} ${label}`
}

// Purchase: eligibility is checked first (GET .../{planId}/eligible → boolean), then
// POST /savings-contracts/{bp} {savingsPlanId, durationMonths, monthlyCommittedAmount, paidUpfront, startDate}.
function PurchaseDialog({
  bp, plan, currency, onOpenChange,
}: {
  bp: string
  plan: SavingsPlan
  currency: string
  onOpenChange: (o: boolean) => void
}) {
  const qc = useQueryClient()
  const schedules = plan.savingSchedule ?? []
  const [duration, setDuration] = useState(schedules[0] ? String(schedules[0].durationMonths) : "")
  const [amount, setAmount] = useState("")
  const [upfront, setUpfront] = useState(false)
  const [start, setStart] = useState("CURRENT_MONTH")

  const { data: eligible, isLoading: eligibleLoading } = useQuery({
    queryKey: ["savings-eligible", bp, plan.id],
    queryFn: () => apiFetch<boolean>(`/savings-contracts/${bp}/${plan.id}/eligible`),
  })

  const purchase = useMutation({
    mutationFn: () =>
      apiFetch(`/savings-contracts/${bp}`, {
        method: "POST",
        body: {
          savingsPlanId: plan.id,
          durationMonths: Number(duration),
          monthlyCommittedAmount: Number(amount),
          paidUpfront: upfront,
          startDate: start,
        },
      }),
    onSuccess: () => {
      toast.success(`Savings contract created for ${plan.name ?? "plan"}`)
      void qc.invalidateQueries({ queryKey: ["savings-contracts", bp] })
      onOpenChange(false)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const amountNum = Number(amount)
  const invalid = !duration || !Number.isFinite(amountNum) || amountNum <= 0

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Purchase {plan.name ?? "savings plan"}</DialogTitle>
          <DialogDescription>
            Commit to a monthly amount — matching usage gets the tier discount on each bill.
          </DialogDescription>
        </DialogHeader>

        {eligible === false ? (
          <p className="text-sm text-destructive">
            This billing profile already has an active contract for this plan.
          </p>
        ) : null}

        <div className="space-y-3">
          <div>
            <Label className="mb-1.5 block">Duration</Label>
            <Select value={duration} onValueChange={setDuration}>
              <SelectTrigger className="w-full">
                <SelectValue placeholder="Select duration" />
              </SelectTrigger>
              <SelectContent>
                {schedules.map((s) => (
                  <SelectItem key={s.durationMonths} value={String(s.durationMonths)}>
                    {s.durationMonths} months
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div>
            <Label className="mb-1.5 block">Monthly committed amount ({currency})</Label>
            <Input
              className="font-mono tabular-nums"
              type="number"
              min={0}
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
            />
          </div>
          <div>
            <Label className="mb-1.5 block">Starts</Label>
            <Select value={start} onValueChange={setStart}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="CURRENT_MONTH">This month</SelectItem>
                <SelectItem value="NEXT_MONTH">Next month</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center gap-2">
            <Checkbox id="upfront" checked={upfront} onCheckedChange={(v) => setUpfront(v === true)} />
            <Label htmlFor="upfront">Pay upfront (upfront tier discounts; cannot be cancelled)</Label>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button
            onClick={() => purchase.mutate()}
            disabled={invalid || purchase.isPending || eligibleLoading || eligible === false}
          >
            {purchase.isPending ? "Purchasing…" : "Purchase"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function ContractsSection({ bp, currency }: { bp: string; currency: string }) {
  const qc = useQueryClient()
  const [cancelling, setCancelling] = useState<SavingsContract | null>(null)
  const [extending, setExtending] = useState<SavingsContract | null>(null)

  const { data: contracts, isLoading, error } = useQuery({
    queryKey: ["savings-contracts", bp],
    queryFn: () => apiFetch<SavingsContract[]>(`/savings-contracts/${bp}`),
  })

  const invalidate = () => void qc.invalidateQueries({ queryKey: ["savings-contracts", bp] })

  const cancel = useMutation({
    // DELETE /savings-contracts/{bp}/{contractId} — non-upfront ACTIVE contracts only.
    mutationFn: (id: string) => apiFetch(`/savings-contracts/${bp}/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Contract cancelled")
      setCancelling(null)
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })
  const extend = useMutation({
    // POST .../extend — pushes endDate out by the contract's own duration.
    mutationFn: (id: string) => apiFetch(`/savings-contracts/${bp}/${id}/extend`, { method: "POST" }),
    onSuccess: () => {
      toast.success("Contract extended")
      setExtending(null)
      invalidate()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <div className="mt-8">
      <h2 className="mb-3 font-display text-lg font-semibold">Your contracts</h2>
      {isLoading ? (
        <Skeleton className="h-40" />
      ) : error ? (
        <ErrorPanel message={(error as Error).message} />
      ) : !contracts?.length ? (
        <EmptyState icon={PiggyBank} title="No savings contracts" hint="Purchase a plan above to start saving." />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Plan</TableHead>
                <TableHead className="text-right">Committed / month</TableHead>
                <TableHead className="text-right">Discount</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Start</TableHead>
                <TableHead>End</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {contracts.map((c) => (
                <TableRow key={c.id}>
                  <TableCell className="font-medium">
                    {c.savingsPlanName ?? c.savingsPlanId ?? "—"}
                    {c.paidUpfront ? <Badge variant="secondary" className="ml-2">Upfront</Badge> : null}
                  </TableCell>
                  <TableCell className="text-right font-mono tabular-nums">
                    {fmtMoney(c.monthlyCommittedAmount, currency)}
                  </TableCell>
                  <TableCell className="text-right font-mono tabular-nums">{pct(c.discountRate)}</TableCell>
                  <TableCell><StatusBadge status={c.status} /></TableCell>
                  <TableCell>{fmtDate(c.startDate)}</TableCell>
                  <TableCell>{fmtDate(c.endDate)}</TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setExtending(c)}
                      disabled={c.status !== "ACTIVE"}
                    >
                      Extend
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setCancelling(c)}
                      disabled={c.status !== "ACTIVE" || c.paidUpfront === true}
                    >
                      Cancel
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}

      <Dialog open={!!cancelling} onOpenChange={(o) => !o && setCancelling(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Cancel contract</DialogTitle>
            <DialogDescription>
              Cancel the {cancelling?.savingsPlanName ?? ""} contract? Its discount stops applying to future bills.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCancelling(null)}>Keep contract</Button>
            <Button
              variant="destructive"
              onClick={() => cancelling && cancel.mutate(cancelling.id)}
              disabled={cancel.isPending}
            >
              {cancel.isPending ? "Cancelling…" : "Cancel contract"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!extending} onOpenChange={(o) => !o && setExtending(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Extend contract</DialogTitle>
            <DialogDescription>
              Extend the {extending?.savingsPlanName ?? ""} contract by {extending?.durationMonths ?? "—"} months
              (new end date {extending?.endDate ? fmtDate(extending.endDate) : "—"} + {extending?.durationMonths ?? 0} months)?
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setExtending(null)}>Cancel</Button>
            <Button onClick={() => extending && extend.mutate(extending.id)} disabled={extend.isPending}>
              {extend.isPending ? "Extending…" : "Extend contract"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function ErrorPanel({ message }: { message: string }) {
  return <div className="rounded-md border bg-muted/40 p-4 text-sm text-muted-foreground">{message}</div>
}
