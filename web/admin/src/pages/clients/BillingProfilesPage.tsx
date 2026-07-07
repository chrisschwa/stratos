import { Link } from "react-router-dom"
import { RefreshCw, Wallet } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useAdminList } from "@/lib/hooks"
import { fmtMoney, timeAgo } from "@/lib/format"

// GET /admin/billing-profile — shaped profile doc + computed financials
// (balance/accountCredit/promotionalCredit/currentMonth/lastMonth/forecastedMonthEnd as JSON numbers).
type BpRow = Record<string, any>

export function profileName(p: BpRow): string {
  const full = [p.firstName, p.lastName].filter(Boolean).join(" ")
  return (p.fullName as string) || full || (p.companyName as string) || (p.email as string) || (p.id as string)
}

export default function BillingProfilesPage() {
  const { data, isLoading, isError, error, refetch, isFetching } = useAdminList<BpRow>("/admin/billing-profile")
  const rows = data?.data ?? []

  return (
    <>
      <PageHeader
        title="Billing profiles"
        description="Customer billing profiles with live balances."
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
        <EmptyState icon={Wallet} title="No billing profiles" hint="Profiles appear here once clients sign up." />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Client</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Currency</TableHead>
                <TableHead className="text-right">Balance</TableHead>
                <TableHead className="text-right">This month</TableHead>
                <TableHead>Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((p) => (
                <TableRow key={p.id}>
                  <TableCell>
                    <Link className="font-medium hover:underline" to={`/clients/billing-profiles/${p.id}`}>
                      {profileName(p)}
                    </Link>
                    <p className="text-xs text-muted-foreground">{p.email ?? "—"}</p>
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={p.status} />
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{p.currency ?? "—"}</TableCell>
                  <TableCell className="text-right font-mono text-sm tabular-nums">
                    {fmtMoney(p.balance, p.currency)}
                  </TableCell>
                  <TableCell className="text-right font-mono text-sm tabular-nums">
                    {fmtMoney(p.currentMonth, p.currency)}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{timeAgo(p.createdAt)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
    </>
  )
}
