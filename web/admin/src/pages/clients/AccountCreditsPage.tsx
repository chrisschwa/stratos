import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Wallet } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { Card } from "@/components/ui/card"
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { useAdminList } from "@/lib/hooks"
import { fmtMoney, timeAgo } from "@/lib/format"

// GET /admin/account-credit?billingProfileId= — the credits of ONE billing profile (the route
// always filters by billingProfileId; there is no global list), returned as a bare {data:[…]}
// envelope (no paging). So this page scopes by a billing-profile picker.
type Row = Record<string, any>

function profileLabel(p: Row): string {
  const full = [p.firstName, p.lastName].filter(Boolean).join(" ")
  return (p.fullName as string) || full || (p.companyName as string) || (p.email as string) || (p.id as string)
}

export default function AccountCreditsPage() {
  const profiles = useAdminList<Row>("/admin/billing-profile")
  const [selected, setSelected] = useState("")
  const bp = selected || profiles.data?.data?.[0]?.id || ""

  const credits = useQuery({
    queryKey: ["admin-account-credits", bp],
    queryFn: () => apiFetch<Row[]>(`/admin/account-credit?billingProfileId=${bp}`),
    enabled: !!bp,
  })

  const rows = credits.data ?? []

  return (
    <>
      <PageHeader
        title="Account credits"
        description="Spendable credits per billing profile."
        actions={
          <Select value={bp} onValueChange={setSelected}>
            <SelectTrigger className="w-64">
              <SelectValue placeholder="Select a billing profile" />
            </SelectTrigger>
            <SelectContent>
              {(profiles.data?.data ?? []).map((p) => (
                <SelectItem key={p.id} value={p.id}>
                  {profileLabel(p)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      />

      {profiles.isLoading || (!!bp && credits.isLoading) ? (
        <Skeleton className="h-64" />
      ) : profiles.isError ? (
        <div className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground">
          {(profiles.error as Error).message}
        </div>
      ) : credits.isError ? (
        <div className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground">
          {(credits.error as Error).message}
        </div>
      ) : !bp ? (
        <EmptyState icon={Wallet} title="No billing profiles" hint="Credits are scoped to a billing profile." />
      ) : !rows.length ? (
        <EmptyState
          icon={Wallet}
          title="No account credits"
          hint="Grant credits from the billing profile's Credits tab."
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Billing profile</TableHead>
                <TableHead>Credit</TableHead>
                <TableHead className="text-right">Amount</TableHead>
                <TableHead className="text-right">Initial</TableHead>
                <TableHead>Currency</TableHead>
                <TableHead>Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((c) => (
                <TableRow key={c.id}>
                  <TableCell className="font-mono text-xs">{c.billingProfileId ?? bp}</TableCell>
                  <TableCell className="font-mono text-xs">{c.id}</TableCell>
                  <TableCell className="text-right font-mono text-sm tabular-nums">
                    {fmtMoney(c.amount, c.currency)}
                  </TableCell>
                  <TableCell className="text-right font-mono text-sm tabular-nums">
                    {fmtMoney(c.initialAmount, c.currency)}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{c.currency ?? "—"}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{timeAgo(c.createdAt)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
    </>
  )
}
