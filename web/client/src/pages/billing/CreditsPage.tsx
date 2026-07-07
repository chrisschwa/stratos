import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { BadgePercent, Gift } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { fmtDate, fmtMoney } from "@/lib/format"
import { useBillingSummary, useProjectId } from "@/lib/hooks"

type PromoCredit = {
  id: string
  code?: string
  initialAmount?: number | string
  remainingAmount?: number | string
  expirationDate?: string
  createdAt?: string
}

// The API uses a far-future sentinel (9999-01-01) for never-expiring credits.
function expiry(v?: string): string {
  if (!v) return "Never"
  const d = new Date(v)
  if (Number.isNaN(d.getTime()) || d.getFullYear() >= 9999) return "Never"
  return fmtDate(v)
}

export default function CreditsPage() {
  const pid = useProjectId()
  const { data: summary, isLoading } = useBillingSummary(pid)
  const bp = summary?.id
  const currency = summary?.currency ?? "USD"

  const { data: credits, isLoading: creditsLoading, error } = useQuery({
    queryKey: ["promotional-credits", bp],
    queryFn: () => apiFetch<PromoCredit[]>(`/promotional-credits/${bp}`),
    enabled: !!bp,
  })

  if (isLoading || !bp) {
    return (
      <>
        <PageHeader title="Promotional credits" />
        <Skeleton className="h-64" />
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="Promotional credits"
        description="Credits granted by promotions — they settle bills before account credit."
      />

      <div className="grid items-start gap-4 lg:grid-cols-3">
        <div className="lg:col-span-2">
          {creditsLoading ? (
            <Skeleton className="h-64" />
          ) : error ? (
            <div className="rounded-md border bg-muted/40 p-4 text-sm text-muted-foreground">
              {(error as Error).message}
            </div>
          ) : !credits?.length ? (
            <EmptyState
              icon={Gift}
              title="No promotional credits"
              hint="Redeem a promo code to receive credit."
            />
          ) : (
            <Card className="overflow-hidden py-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Code</TableHead>
                    <TableHead className="text-right">Amount</TableHead>
                    <TableHead className="text-right">Remaining</TableHead>
                    <TableHead>Expires</TableHead>
                    <TableHead>Granted</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {credits.map((c) => (
                    <TableRow key={c.id}>
                      <TableCell className="font-mono text-sm">{c.code ?? "—"}</TableCell>
                      <TableCell className="text-right font-mono tabular-nums">
                        {fmtMoney(c.initialAmount, currency)}
                      </TableCell>
                      <TableCell className="text-right font-mono tabular-nums">
                        {fmtMoney(c.remainingAmount, currency)}
                      </TableCell>
                      <TableCell>{expiry(c.expirationDate)}</TableCell>
                      <TableCell>{fmtDate(c.createdAt)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Card>
          )}
        </div>

        <RedeemCard pid={pid} bp={bp} />
      </div>
    </>
  )
}

// Same redeem endpoint as FundsPage — POST /promotion/{bp}/code?code= (both surfaces keep it).
function RedeemCard({ pid, bp }: { pid: string; bp: string }) {
  const qc = useQueryClient()
  const [code, setCode] = useState("")
  const redeem = useMutation({
    mutationFn: () =>
      apiFetch(`/promotion/${bp}/code?code=${encodeURIComponent(code.trim())}`, { method: "POST" }),
    onSuccess: () => {
      toast.success("Promo code redeemed")
      setCode("")
      void qc.invalidateQueries({ queryKey: ["promotional-credits", bp] })
      void qc.invalidateQueries({ queryKey: ["billing-summary", pid] })
    },
    onError: (e: Error) => toast.error(e.message),
  })
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <BadgePercent className="size-4" /> Redeem a promo code
        </CardTitle>
      </CardHeader>
      <CardContent className="flex gap-2">
        <Input placeholder="Promo code" value={code} onChange={(e) => setCode(e.target.value)} />
        <Button onClick={() => redeem.mutate()} disabled={!code.trim() || redeem.isPending}>
          {redeem.isPending ? "Redeeming…" : "Redeem"}
        </Button>
      </CardContent>
    </Card>
  )
}
