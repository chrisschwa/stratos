import { useState } from "react"
import { useInfiniteQuery } from "@tanstack/react-query"
import { ScrollText } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetchEnvelope } from "@/lib/api"
import { fmtDateTime } from "@/lib/format"

type PropertyChange = { field?: string; oldValue?: unknown; newValue?: unknown }

// GET /admin/audit rows are hydrated AuditEventDto { event, organization?, project?, user? }.
type AuditRow = {
  event: {
    id?: string
    timestamp?: string
    requestInterface?: string
    eventContext?: string
    action?: string
    resourceType?: string
    resourceId?: string
    resourceDisplayName?: string
    changes?: PropertyChange[]
    actor?: { type?: string; id?: string; displayName?: string }
    outcome?: string
  }
  organization?: { id?: string; name?: string }
  project?: { id?: string; name?: string }
  user?: { id?: string; sub?: string; email?: string; firstName?: string; lastName?: string }
}

type CursorPaging = { limit?: number; nextMarker?: string; prevMarker?: string }

const PAGE_SIZE = 50

function fmtValue(v: unknown): string {
  if (v === undefined || v === null || v === "") return "—"
  return typeof v === "string" ? v : JSON.stringify(v)
}

export default function AuditPage() {
  const q = useInfiniteQuery({
    queryKey: ["admin-audit"],
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      apiFetchEnvelope<AuditRow[]>(
        `/admin/audit?limit=${PAGE_SIZE}${pageParam ? `&after=${encodeURIComponent(pageParam)}` : ""}`,
      ),
    getNextPageParam: (last) => (last.paging as CursorPaging | undefined)?.nextMarker ?? undefined,
  })

  const rows = q.data?.pages.flatMap((p) => p.data ?? []) ?? []
  const [detail, setDetail] = useState<AuditRow | null>(null)

  const actorOf = (r: AuditRow) =>
    r.user?.email ?? r.event.actor?.displayName ?? r.event.actor?.id ?? "—"

  return (
    <>
      <PageHeader title="Audit log" description="Every admin, client and system mutation, newest first." />
      {q.isLoading ? (
        <Skeleton className="h-64" />
      ) : q.error ? (
        <div className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground">{(q.error as Error).message}</div>
      ) : rows.length === 0 ? (
        <EmptyState icon={ScrollText} title="No audit events" hint="Mutations will show up here as they happen." />
      ) : (
        <>
          <Card className="overflow-hidden py-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Time</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>Area</TableHead>
                  <TableHead>Action</TableHead>
                  <TableHead>Resource</TableHead>
                  <TableHead>Changes</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((r, i) => {
                  const e = r.event
                  const nChanges = e.changes?.length ?? 0
                  return (
                    <TableRow key={e.id ?? i}>
                      <TableCell className="whitespace-nowrap">{fmtDateTime(e.timestamp)}</TableCell>
                      <TableCell>{actorOf(r)}</TableCell>
                      <TableCell>
                        <Badge variant="outline">{e.requestInterface ?? "—"}</Badge>
                      </TableCell>
                      <TableCell className="font-medium">{e.action ?? "—"}</TableCell>
                      <TableCell>
                        <span>{e.resourceType ?? "—"}</span>
                        {e.resourceDisplayName || e.resourceId ? (
                          <span className="ml-2 font-mono text-xs text-muted-foreground">
                            {e.resourceDisplayName ?? e.resourceId}
                          </span>
                        ) : null}
                      </TableCell>
                      <TableCell>
                        {nChanges > 0 ? (
                          <Button variant="ghost" size="sm" onClick={() => setDetail(r)}>
                            {nChanges} {nChanges === 1 ? "change" : "changes"}
                          </Button>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </Card>
          {q.hasNextPage ? (
            <div className="mt-4 flex justify-center">
              <Button variant="outline" onClick={() => q.fetchNextPage()} disabled={q.isFetchingNextPage}>
                {q.isFetchingNextPage ? "Loading…" : "Load more"}
              </Button>
            </div>
          ) : null}
        </>
      )}

      <Dialog open={!!detail} onOpenChange={(o) => !o && setDetail(null)}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {detail?.event.action} — {detail?.event.resourceType}
            </DialogTitle>
            <DialogDescription>
              {fmtDateTime(detail?.event.timestamp)} by {detail ? actorOf(detail) : ""}
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-[60vh] overflow-y-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Field</TableHead>
                  <TableHead>Old value</TableHead>
                  <TableHead>New value</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(detail?.event.changes ?? []).map((c, i) => (
                  <TableRow key={i}>
                    <TableCell className="font-mono text-xs">{c.field ?? "—"}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{fmtValue(c.oldValue)}</TableCell>
                    <TableCell className="font-mono text-xs">{fmtValue(c.newValue)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
