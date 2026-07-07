import { useState } from "react"
import { Link } from "react-router-dom"
import { Download, Receipt, RefreshCw } from "lucide-react"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { useAdminList } from "@/lib/hooks"
import { fmtMoney, timeAgo } from "@/lib/format"

// GET /admin/bill — every bill doc (shaped, money as numbers) + the joined `billingProfile`.
// The raw doc carries items[].netAmount (scale-16 running nets); the list net = their sum.
// (Gross/unpaid are only computed by the per-bill overview endpoints, not stored on the doc.)
type BillRow = Record<string, any>

function billNet(b: BillRow): number {
  const items = (b.items as Array<Record<string, any>>) ?? []
  return items.reduce((sum, it) => {
    const n = parseFloat(String(it?.netAmount ?? 0))
    return sum + (Number.isNaN(n) ? 0 : n)
  }, 0)
}

export default function BillsPage() {
  const { data, isLoading, isError, error, refetch, isFetching } = useAdminList<BillRow>("/admin/bill")
  const rows = data?.data ?? []
  const [downloading, setDownloading] = useState<string | null>(null)

  // GET /admin/bill/download/{billId} → statement PDF (streamed) → blob download.
  const download = async (billId: string) => {
    setDownloading(billId)
    try {
      const resp = await apiFetch<Response>(`/admin/bill/download/${billId}`, { raw: true })
      if (!resp.ok) throw new Error(`Download failed (${resp.status})`)
      const blob = await resp.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement("a")
      a.href = url
      a.download = `bill-${billId}.pdf`
      a.click()
      URL.revokeObjectURL(url)
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setDownloading(null)
    }
  }

  return (
    <>
      <PageHeader
        title="Bills"
        description="Every bill on the platform, newest first."
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
        <EmptyState icon={Receipt} title="No bills yet" hint="Bills appear once usage charging runs." />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Bill</TableHead>
                <TableHead>Client</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Net</TableHead>
                <TableHead>Created</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((b) => {
                const bp = (b.billingProfile as Record<string, any>) ?? {}
                const bpLabel =
                  bp.fullName ||
                  [bp.firstName, bp.lastName].filter(Boolean).join(" ") ||
                  bp.email ||
                  b.billingProfileId
                return (
                  <TableRow key={b.id}>
                    <TableCell className="font-mono text-xs">{b.id}</TableCell>
                    <TableCell>
                      {b.billingProfileId ? (
                        <Link className="text-sm hover:underline" to={`/clients/billing-profiles/${b.billingProfileId}`}>
                          {bpLabel}
                        </Link>
                      ) : (
                        <span className="text-sm text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={b.status} />
                    </TableCell>
                    <TableCell className="text-right font-mono text-sm tabular-nums">
                      {fmtMoney(billNet(b), b.invoiceCurrency ?? "USD")}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">{timeAgo(b.createdAt)}</TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={downloading === b.id}
                        onClick={() => void download(b.id)}
                      >
                        <Download className="size-4" /> PDF
                      </Button>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </Card>
      )}
    </>
  )
}
