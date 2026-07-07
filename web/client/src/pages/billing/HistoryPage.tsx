import { useNavigate } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import { toast } from "sonner"
import { Download, FileText, Receipt } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { apiFetch } from "@/lib/api"
import { fmtDate, fmtDateTime, fmtMoney } from "@/lib/format"
import { useBillingSummary, useProjectId } from "@/lib/hooks"
import type { Bill, Transaction } from "@/lib/types"

// Fetch an authed endpoint as a blob and trigger a browser download.
export async function downloadPdf(path: string, filename: string) {
  const resp = await apiFetch<Response>(path, { raw: true })
  if (!resp.ok) throw new Error(`Download failed (${resp.status})`)
  const blob = await resp.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement("a")
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

export default function HistoryPage() {
  const pid = useProjectId()
  const { data: summary, isLoading } = useBillingSummary(pid)
  const bp = summary?.id

  return (
    <>
      <PageHeader title="Billing history" description="Bills and payment transactions on this billing profile." />
      {isLoading || !bp ? (
        <Skeleton className="h-64" />
      ) : (
        <Tabs defaultValue="bills">
          <TabsList>
            <TabsTrigger value="bills">Bills</TabsTrigger>
            <TabsTrigger value="transactions">Transactions</TabsTrigger>
            <TabsTrigger value="account-credits">Account credits</TabsTrigger>
          </TabsList>
          <TabsContent value="bills" className="mt-4">
            <BillsTab pid={pid} bp={bp} />
          </TabsContent>
          <TabsContent value="transactions" className="mt-4">
            <TransactionsTab bp={bp} />
          </TabsContent>
          <TabsContent value="account-credits" className="mt-4">
            <AccountCreditsTab bp={bp} />
          </TabsContent>
        </Tabs>
      )}
    </>
  )
}

function BillsTab({ pid, bp }: { pid: string; bp: string }) {
  const navigate = useNavigate()
  const { data: bills, isLoading, error } = useQuery({
    queryKey: ["bills", bp],
    queryFn: () => apiFetch<Bill[]>(`/bill/${bp}`),
  })

  if (isLoading) return <Skeleton className="h-64" />
  if (error) return <ErrorPanel message={(error as Error).message} />
  if (!bills?.length) {
    return <EmptyState icon={FileText} title="No bills yet" hint="Bills appear here once usage is charged." />
  }
  return (
    <Card className="overflow-hidden py-0">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Created</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="text-right">Net</TableHead>
            <TableHead className="text-right">Gross</TableHead>
            <TableHead className="text-right">Unpaid</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {bills.map((b) => (
            <TableRow
              key={b.id}
              className="cursor-pointer"
              onClick={() => navigate(`/p/${pid}/billing/history/bills/${b.id}`)}
            >
              <TableCell>{fmtDate(b.createdAt)}</TableCell>
              <TableCell>
                <StatusBadge status={b.status} />
              </TableCell>
              <TableCell className="text-right font-mono tabular-nums">
                {fmtMoney(b.netAmount, b.invoiceCurrency)}
              </TableCell>
              <TableCell className="text-right font-mono tabular-nums">
                {fmtMoney(b.grossAmount, b.invoiceCurrency)}
              </TableCell>
              <TableCell className="text-right font-mono tabular-nums">
                {fmtMoney(b.unpaidGrossAmount, b.invoiceCurrency)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}

function TransactionsTab({ bp }: { bp: string }) {
  // The list is query-param based: GET /collect-transactions?billingProfileId=
  // (the /collect-transactions/{id} path route is the single-by-id read).
  const { data: txns, isLoading, error } = useQuery({
    queryKey: ["collect-transactions", bp],
    queryFn: () => apiFetch<Transaction[]>(`/collect-transactions?billingProfileId=${bp}`),
  })

  if (isLoading) return <Skeleton className="h-64" />
  if (error) return <ErrorPanel message={(error as Error).message} />
  if (!txns?.length) {
    return <EmptyState icon={Receipt} title="No transactions yet" hint="Deposits and bill payments appear here." />
  }
  return (
    <Card className="overflow-hidden py-0">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Date</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="text-right">Amount</TableHead>
            <TableHead>External ID</TableHead>
            <TableHead className="text-right">Receipt</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {txns.map((t) => (
            <TableRow key={t.id}>
              <TableCell>{fmtDateTime(t.createdAt)}</TableCell>
              <TableCell>
                <StatusBadge status={t.status} />
              </TableCell>
              <TableCell className="text-right font-mono tabular-nums">
                {fmtMoney(t.grossAmount, t.currency)}
              </TableCell>
              <TableCell className="font-mono text-xs text-muted-foreground">
                {(t.externalId as string) ?? t.externalInvoiceId ?? "—"}
              </TableCell>
              <TableCell className="text-right">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() =>
                    downloadPdf(
                      `/collect-transactions/${bp}/download/${t.id}`,
                      `${t.externalInvoiceId ?? t.id}.pdf`,
                    ).catch((e: Error) => toast.error(e.message))
                  }
                >
                  <Download className="size-4" />
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}

// Account-credit transactions (deposits / refunds) — GET /account-credit-transactions?billingProfileId=.
// ponytail: no per-row receipt download — the Go download route is a deliberate 501 seam
// (external invoice provider not implemented); add the button when the seam goes live.
function AccountCreditsTab({ bp }: { bp: string }) {
  const { data: txns, isLoading, error } = useQuery({
    queryKey: ["account-credit-transactions", bp],
    queryFn: () => apiFetch<Transaction[]>(`/account-credit-transactions?billingProfileId=${bp}`),
  })

  if (isLoading) return <Skeleton className="h-64" />
  if (error) return <ErrorPanel message={(error as Error).message} />
  if (!txns?.length) {
    return <EmptyState icon={Receipt} title="No account-credit transactions" hint="Deposits and refunds appear here." />
  }
  return (
    <Card className="overflow-hidden py-0">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Date</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="text-right">Amount</TableHead>
            <TableHead className="text-right">Gross</TableHead>
            <TableHead>Currency</TableHead>
            <TableHead>External ID</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {txns.map((t) => (
            <TableRow key={t.id}>
              <TableCell>{fmtDateTime(t.createdAt)}</TableCell>
              <TableCell>
                <StatusBadge status={t.status} />
              </TableCell>
              <TableCell className="text-right font-mono tabular-nums">{fmtMoney(t.amount, t.currency)}</TableCell>
              <TableCell className="text-right font-mono tabular-nums">
                {fmtMoney(t.grossAmount, t.currency)}
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">{t.currency ?? "—"}</TableCell>
              <TableCell className="font-mono text-xs text-muted-foreground">
                {(t.externalId as string) ?? t.externalInvoiceId ?? "—"}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}

function ErrorPanel({ message }: { message: string }) {
  return <div className="rounded-md border bg-muted/40 p-4 text-sm text-muted-foreground">{message}</div>
}
