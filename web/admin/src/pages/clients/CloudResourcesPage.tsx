import { useMemo, useState } from "react"
import { Link } from "react-router-dom"
import { Cloud, RefreshCw } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useAdminList } from "@/lib/hooks"
import { timeAgo } from "@/lib/format"

// GET /admin/cloud-resource (handler.go cloudResourcesAll) — single(list), NO paging:
// [{id, data, createdAt, externalId, info, region, type, serviceId, project:{id,name}}].
// The live object sits under data.<typeKey> (data.server / data.network / …).
export type CloudResourceRow = Record<string, any>

// The display name lives on the type-keyed object inside `data` (data.server.name, …);
// buckets are flat (data.bucketName).
export function resourceName(cr: CloudResourceRow): string {
  const d = (cr.data ?? {}) as Record<string, any>
  if (typeof d.bucketName === "string" && d.bucketName) return d.bucketName
  for (const v of Object.values(d)) {
    if (v && typeof v === "object" && typeof (v as any).name === "string" && (v as any).name) {
      return (v as any).name as string
    }
  }
  return (cr.externalId as string) || (cr.id as string) || "—"
}

export function resourceStatus(cr: CloudResourceRow): string | undefined {
  const d = (cr.data ?? {}) as Record<string, any>
  for (const v of Object.values(d)) {
    if (v && typeof v === "object" && typeof (v as any).status === "string" && (v as any).status) {
      return (v as any).status as string
    }
  }
  return undefined
}

export function resourceCreatedAt(cr: CloudResourceRow): string | undefined {
  return (cr.info?.createdAt as string) ?? (cr.createdAt as string)
}

export default function CloudResourcesPage() {
  const { data, isLoading, isError, error, refetch, isFetching } =
    useAdminList<CloudResourceRow>("/admin/cloud-resource")
  const rows = data?.data ?? []
  const [type, setType] = useState("ALL")
  const [search, setSearch] = useState("")

  const types = useMemo(
    () => Array.from(new Set(rows.map((r) => r.type as string).filter(Boolean))).sort(),
    [rows],
  )

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    return rows.filter((r) => {
      if (type !== "ALL" && r.type !== type) return false
      if (!q) return true
      return [resourceName(r), r.externalId, r.type, r.region, r.project?.name, r.project?.id]
        .some((v) => typeof v === "string" && v.toLowerCase().includes(q))
    })
  }, [rows, type, search])

  return (
    <>
      <PageHeader
        title="Cloud resources"
        description="All cached cloud resources across every project."
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
        <EmptyState icon={Cloud} title="No cloud resources" hint="Resources appear here as projects provision infrastructure." />
      ) : (
        <>
          <div className="mb-4 flex flex-wrap items-center gap-2">
            <Input
              placeholder="Search by name, ID, project…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="h-9 w-64"
            />
            <Select value={type} onValueChange={setType}>
              <SelectTrigger className="h-9 w-48">
                <SelectValue placeholder="All types" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="ALL">All types</SelectItem>
                {types.map((t) => (
                  <SelectItem key={t} value={t}>
                    {t.toLowerCase().replace(/_/g, " ")}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <span className="ml-auto text-sm text-muted-foreground">
              {filtered.length} of {rows.length}
            </span>
          </div>

          {!filtered.length ? (
            <EmptyState icon={Cloud} title="No matches" hint="Try a different search or type filter." />
          ) : (
            <Card className="overflow-hidden py-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Type</TableHead>
                    <TableHead>Project</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Region</TableHead>
                    <TableHead>Created</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filtered.map((r) => (
                    <TableRow key={r.id}>
                      <TableCell>
                        <Link className="font-medium hover:underline" to={`/clients/cloud-resources/${r.id}`}>
                          {resourceName(r)}
                        </Link>
                        <p className="font-mono text-xs text-muted-foreground">{r.externalId ?? "—"}</p>
                      </TableCell>
                      <TableCell className="text-sm capitalize text-muted-foreground">
                        {(r.type as string | undefined)?.toLowerCase().replace(/_/g, " ") ?? "—"}
                      </TableCell>
                      <TableCell>
                        {r.project?.id ? (
                          <Link className="text-sm hover:underline" to={`/clients/projects/${r.project.id}`}>
                            {r.project?.name ?? r.project.id}
                          </Link>
                        ) : (
                          <span className="text-sm text-muted-foreground">—</span>
                        )}
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={resourceStatus(r)} />
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">{r.region ?? "—"}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">{timeAgo(resourceCreatedAt(r))}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Card>
          )}
        </>
      )}
    </>
  )
}
