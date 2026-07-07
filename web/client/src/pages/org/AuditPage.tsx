import { useState } from "react"
import { useInfiniteQuery } from "@tanstack/react-query"
import { toast } from "sonner"
import { Download, ScrollText } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch, apiFetchEnvelope } from "@/lib/api"
import { fmtDateTime } from "@/lib/format"
import { useProjectId } from "@/lib/hooks"
import { useOrg } from "./MembersPage"

type AuditEvent = {
  id?: string
  timestamp?: string
  action?: string
  resourceType?: string
  resourceId?: string
  resourceDisplayName?: string
  outcome?: string
  actor?: { id?: string; displayName?: string; type?: string }
}

const LIMIT = 50

export default function AuditPage() {
  const pid = useProjectId()
  const { org, isLoading: orgLoading } = useOrg(pid)

  // Cursor-paged: GET /organizations/{orgId}/audit?limit=&after= → paging.nextMarker.
  const { data, isLoading, error, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteQuery({
    queryKey: ["org-audit", org?.id],
    queryFn: ({ pageParam }) =>
      apiFetchEnvelope<AuditEvent[]>(
        `/organizations/${org?.id}/audit?limit=${LIMIT}${pageParam ? `&after=${encodeURIComponent(pageParam)}` : ""}`,
      ),
    initialPageParam: "",
    getNextPageParam: (last) => (last.paging as { nextMarker?: string } | undefined)?.nextMarker ?? undefined,
    enabled: !!org?.id,
  })

  const events = data?.pages.flatMap((p) => p.data ?? []) ?? []

  // GET /organizations/{id}/audit/export → text/csv attachment (UTF-8 BOM, capped 10000 events).
  const [exporting, setExporting] = useState(false)
  const exportCsv = async () => {
    if (!org?.id) return
    setExporting(true)
    try {
      const resp = await apiFetch<Response>(`/organizations/${org.id}/audit/export`, { raw: true })
      if (!resp.ok) throw new Error(`Export failed (${resp.status})`)
      const blob = await resp.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement("a")
      a.href = url
      a.download = "audit-events.csv"
      a.click()
      URL.revokeObjectURL(url)
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setExporting(false)
    }
  }

  return (
    <>
      <PageHeader
        title="Audit log"
        description={org?.name ? `Activity in the ${org.name} organization.` : "Organization activity."}
        actions={
          <Button size="sm" variant="outline" onClick={() => void exportCsv()} disabled={!org || exporting}>
            <Download className="size-4" /> {exporting ? "Exporting…" : "Export CSV"}
          </Button>
        }
      />

      {orgLoading || isLoading ? (
        <Skeleton className="h-64" />
      ) : error ? (
        <div className="rounded-md border bg-muted/40 p-4 text-sm text-muted-foreground">
          {(error as Error).message}
        </div>
      ) : !events.length ? (
        <EmptyState icon={ScrollText} title="No audit events" hint="Actions on this organization will show up here." />
      ) : (
        <>
          <Card className="overflow-hidden py-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Time</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>Action</TableHead>
                  <TableHead>Resource</TableHead>
                  <TableHead>Outcome</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {events.map((e, i) => (
                  <TableRow key={e.id ?? i}>
                    <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
                      {fmtDateTime(e.timestamp)}
                    </TableCell>
                    <TableCell className="text-sm">{e.actor?.displayName ?? e.actor?.id ?? "—"}</TableCell>
                    <TableCell>
                      <Badge variant="secondary">{e.action ?? "—"}</Badge>
                    </TableCell>
                    <TableCell className="text-sm">
                      {e.resourceType ?? "—"}
                      {e.resourceDisplayName || e.resourceId ? (
                        <span className="ml-2 font-mono text-xs text-muted-foreground">
                          {e.resourceDisplayName ?? e.resourceId}
                        </span>
                      ) : null}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">{e.outcome ?? "—"}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Card>
          {hasNextPage ? (
            <div className="mt-4 text-center">
              <Button variant="outline" onClick={() => void fetchNextPage()} disabled={isFetchingNextPage}>
                {isFetchingNextPage ? "Loading…" : "Load more"}
              </Button>
            </div>
          ) : null}
        </>
      )}
    </>
  )
}
