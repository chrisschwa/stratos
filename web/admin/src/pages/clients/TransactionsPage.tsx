import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { CreditCard, Download, RefreshCw } from "lucide-react"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { apiFetch } from "@/lib/api"
import { useAdminList } from "@/lib/hooks"
import { fmtDateTime, fmtMoney } from "@/lib/format"

// Platform-wide transaction lists (the old admin's global Financial → Transactions):
//   GET /admin/account-credit-transactions   (all deposits)
//   GET /admin/collect-transactions          (all collect charges)
// The billing-profile picker is an OPTIONAL filter — "All profiles" by default so a deposit is
// visible without hunting for the right profile. The gateway re-sync is
// GET /admin/account-credit-transactions/{id}/sync (account-credit deposits only).
type Row = Record<string, any>

function profileLabel(p: Row): string {
  const full = [p.firstName, p.lastName].filter(Boolean).join(" ")
  return (p.fullName as string) || full || (p.companyName as string) || (p.email as string) || (p.id as string)
}

// Stream a receipt PDF: read the blob, name it from Content-Disposition, click a temp <a download>.
async function downloadResponse(resp: Response, fallback: string) {
  const blob = await resp.blob()
  const cd = resp.headers.get("content-disposition")
  const m = cd && (/filename\*=(?:UTF-8'')?"?([^";]+)"?/i.exec(cd) || /filename="?([^";]+)"?/i.exec(cd))
  const filename = m ? decodeURIComponent(m[1]) : fallback
  const url = URL.createObjectURL(blob)
  const a = document.createElement("a")
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

export default function TransactionsPage() {
  const qc = useQueryClient()
  const profiles = useAdminList<Row>("/admin/billing-profile")
  const [selected, setSelected] = useState("") // "" = all profiles

  // Load platform-wide, then filter client-side by the optional picker.
  const credits = useAdminList<Row>("/admin/account-credit-transactions")
  const collects = useAdminList<Row>("/admin/collect-transactions")

  const filterByBp = (rows: Row[] | undefined) =>
    (rows ?? []).filter((t) => !selected || t.billingProfileId === selected)
  const creditRows = filterByBp(credits.data?.data)
  const collectRows = filterByBp(collects.data?.data)

  const sync = useMutation({
    mutationFn: (txnId: string) => apiFetch(`/admin/account-credit-transactions/${txnId}/sync`),
    onSuccess: () => {
      toast.success("Transaction re-synced with the gateway")
      void qc.invalidateQueries({ queryKey: ["admin-list", "/admin/account-credit-transactions"] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const download = useMutation({
    mutationFn: async (txnId: string) => {
      const resp = await apiFetch<Response>(`/admin/collect-transactions/download/${txnId}`, { raw: true })
      if (!resp.ok) throw new Error((await resp.text()) || `Download failed (${resp.status})`)
      await downloadResponse(resp, `${txnId}.pdf`)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <>
      <PageHeader
        title="Transactions"
        description="Every deposit and collect transaction across all billing profiles. Filter by profile with the picker."
        actions={
          <Select value={selected || "all"} onValueChange={(v) => setSelected(v === "all" ? "" : v)}>
            <SelectTrigger className="w-64">
              <SelectValue placeholder="All profiles" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All profiles</SelectItem>
              {(profiles.data?.data ?? []).map((p) => (
                <SelectItem key={p.id} value={p.id}>
                  {profileLabel(p)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      />

      {credits.isError ? (
        <div className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground">
          {(credits.error as Error).message}
        </div>
      ) : (
        <Tabs defaultValue="credits">
          <TabsList>
            <TabsTrigger value="credits">Account credit</TabsTrigger>
            <TabsTrigger value="collects">Collect</TabsTrigger>
          </TabsList>

          <TabsContent value="credits" className="mt-4">
            {credits.isLoading ? (
              <Skeleton className="h-48" />
            ) : !creditRows.length ? (
              <EmptyState icon={CreditCard} title="No deposit transactions" hint={selected ? "No deposits for this profile." : "Client deposits will show up here."} />
            ) : (
              <Card className="overflow-hidden py-0">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Date</TableHead>
                      <TableHead>Billing profile</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead className="text-right">Amount</TableHead>
                      <TableHead>External ID</TableHead>
                      <TableHead />
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {creditRows.map((t) => (
                      <TableRow key={t.id}>
                        <TableCell className="text-sm text-muted-foreground">{fmtDateTime(t.createdAt)}</TableCell>
                        <TableCell className="font-mono text-xs">{t.billingProfileId}</TableCell>
                        <TableCell>
                          <StatusBadge status={t.status} />
                        </TableCell>
                        <TableCell className="text-right font-mono text-sm tabular-nums">
                          {fmtMoney(t.amount, t.currency)}
                        </TableCell>
                        <TableCell className="font-mono text-xs">{t.externalId ?? "—"}</TableCell>
                        <TableCell className="text-right">
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={sync.isPending}
                            onClick={() => sync.mutate(t.id)}
                          >
                            <RefreshCw className={sync.isPending ? "size-4 animate-spin" : "size-4"} /> Sync
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Card>
            )}
          </TabsContent>

          <TabsContent value="collects" className="mt-4">
            {collects.isLoading ? (
              <Skeleton className="h-48" />
            ) : !collectRows.length ? (
              <EmptyState icon={CreditCard} title="No collect transactions" hint={selected ? "No collect charges for this profile." : "Card charges for bills land here."} />
            ) : (
              <Card className="overflow-hidden py-0">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Date</TableHead>
                      <TableHead>Billing profile</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead className="text-right">Amount</TableHead>
                      <TableHead>Gateway</TableHead>
                      <TableHead>External ID</TableHead>
                      <TableHead />
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {collectRows.map((t) => (
                      <TableRow key={t.id}>
                        <TableCell className="text-sm text-muted-foreground">{fmtDateTime(t.createdAt)}</TableCell>
                        <TableCell className="font-mono text-xs">{t.billingProfileId}</TableCell>
                        <TableCell>
                          <StatusBadge status={t.status} />
                        </TableCell>
                        <TableCell className="text-right font-mono text-sm tabular-nums">
                          {fmtMoney(t.amount, t.currency)}
                        </TableCell>
                        <TableCell className="font-mono text-xs">{t.paymentGatewayId ?? "—"}</TableCell>
                        <TableCell className="font-mono text-xs">{t.externalId ?? "—"}</TableCell>
                        <TableCell className="text-right">
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={download.isPending}
                            onClick={() => download.mutate(t.id)}
                          >
                            <Download className="size-4" /> Download
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Card>
            )}
          </TabsContent>
        </Tabs>
      )}
    </>
  )
}
