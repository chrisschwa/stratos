import { useMemo, useState } from "react"
import {
  Bar, BarChart, Cell, Pie, PieChart, ResponsiveContainer, Tooltip, XAxis, YAxis,
} from "recharts"
import { BarChart3 } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useBillingSummary, useOrgCostInfo, useProjectId, useProjects } from "@/lib/hooks"
import type { CostInfo } from "@/lib/types"
import { fmtMoney } from "@/lib/format"

// Distinct-enough palette that reads on light + dark; index-wrapped for any project/category count.
const PALETTE = ["#6366f1", "#14b8a6", "#f59e0b", "#ec4899", "#10b981", "#3b82f6", "#a855f7", "#ef4444"]

const n = (v: unknown) => Number(v ?? 0)

export default function OrgBillingPage() {
  const pid = useProjectId()
  const { data: summary } = useBillingSummary(pid)
  const bp = summary?.id
  const { data, isLoading } = useOrgCostInfo(bp)
  const { data: projects } = useProjects()

  const [month, setMonth] = useState<"current" | "last">("current")
  const [proj, setProj] = useState("__all__")

  const currency = data?.currency ?? summary?.currency ?? "USD"
  const projName = (id: string) => projects?.find((p) => p.id === id)?.name || id

  const costByProject = data?.projects ?? {}
  const projectIds = Object.keys(costByProject)

  const totalsKey = month === "current" ? "currentMonthCosts" : "lastMonthCosts"
  const byTypeKey = month === "current" ? "currentMonthCostsByType" : "lastMonthCostsByType"

  // Selected scope: whole org (billing profile) or one project.
  const scope: CostInfo | undefined = proj === "__all__" ? data?.billingProfileCostInfo : costByProject[proj]
  const scopeTotal = n(scope?.[totalsKey])
  const byType = (scope?.[byTypeKey] as Record<string, number> | undefined) ?? {}

  // Per-project rows sorted by the selected month's spend (drives the bar chart + table).
  const rows = useMemo(
    () =>
      projectIds
        .map((id) => ({ id, name: projName(id), current: n(costByProject[id]?.currentMonthCosts), last: n(costByProject[id]?.lastMonthCosts) }))
        .sort((a, b) => (month === "current" ? b.current - a.current : b.last - a.last)),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [data, projects, month],
  )
  const projBars = rows.map((r) => ({ name: r.name, value: month === "current" ? r.current : r.last })).filter((r) => r.value > 0)
  const typeBars = Object.entries(byType).map(([name, value]) => ({ name, value: n(value) })).filter((r) => r.value > 0)
  const topResources = (scope?.topResourcePrices ?? []).slice(0, 15)

  const orgThis = n(data?.billingProfileCostInfo?.currentMonthCosts)
  const orgLast = n(data?.billingProfileCostInfo?.lastMonthCosts)
  const delta = orgLast > 0 ? ((orgThis - orgLast) / orgLast) * 100 : null

  if (isLoading || !bp) {
    return (
      <>
        <PageHeader title="Organization billing" />
        <Skeleton className="h-72" />
      </>
    )
  }

  if (!projectIds.length) {
    return (
      <>
        <PageHeader title="Organization billing" description="Per-project and per-resource spend across the organization." />
        <EmptyState icon={BarChart3} title="No billed usage yet" hint="Once projects accrue cost this month, per-project and per-resource breakdowns show here." />
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="Organization billing"
        description="Per-project and per-resource spend across the organization."
        actions={
          <div className="flex items-center gap-2">
            <div className="inline-flex overflow-hidden rounded-md border">
              <Button variant={month === "current" ? "default" : "ghost"} size="sm" className="rounded-none" onClick={() => setMonth("current")}>
                This month
              </Button>
              <Button variant={month === "last" ? "default" : "ghost"} size="sm" className="rounded-none" onClick={() => setMonth("last")}>
                Last month
              </Button>
            </div>
            <Select value={proj} onValueChange={setProj}>
              <SelectTrigger className="w-56">
                <SelectValue placeholder="All projects" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all__">All projects</SelectItem>
                {rows.map((r) => (
                  <SelectItem key={r.id} value={r.id}>
                    {r.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        }
      />

      <div className="grid gap-4 md:grid-cols-3">
        <Stat label={proj === "__all__" ? "Selected scope" : projName(proj)} value={fmtMoney(scopeTotal, currency)} sub={month === "current" ? "This month" : "Last month"} />
        <Stat label="Org this month" value={fmtMoney(orgThis, currency)} sub={`${projectIds.length} project${projectIds.length === 1 ? "" : "s"} billed`} />
        <Stat
          label="Org last month"
          value={fmtMoney(orgLast, currency)}
          sub={delta === null ? "No prior month" : `${delta >= 0 ? "+" : ""}${delta.toFixed(0)}% vs this month`}
        />
      </div>

      <div className="mt-6 grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Spend by project</CardTitle>
          </CardHeader>
          <CardContent>
            {projBars.length ? (
              <ResponsiveContainer width="100%" height={280}>
                <BarChart data={projBars} layout="vertical" margin={{ left: 8, right: 16 }}>
                  <XAxis type="number" tickFormatter={(v) => fmtMoney(Number(v), currency)} tick={{ fontSize: 11 }} />
                  <YAxis type="category" dataKey="name" width={120} tick={{ fontSize: 11 }} />
                  <Tooltip formatter={(v) => fmtMoney(Number(v), currency)} cursor={{ fill: "rgba(127,127,127,0.1)" }} />
                  <Bar dataKey="value" radius={[0, 4, 4, 0]}>
                    {projBars.map((_, i) => (
                      <Cell key={i} fill={PALETTE[i % PALETTE.length]} />
                    ))}
                  </Bar>
                </BarChart>
              </ResponsiveContainer>
            ) : (
              <p className="py-16 text-center text-sm text-muted-foreground">No spend in the selected month.</p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Spend by resource type{proj === "__all__" ? "" : ` — ${projName(proj)}`}</CardTitle>
          </CardHeader>
          <CardContent>
            {typeBars.length ? (
              <div className="flex flex-col items-center gap-4 sm:flex-row">
                <ResponsiveContainer width="100%" height={220} className="max-w-[220px]">
                  <PieChart>
                    <Pie data={typeBars} dataKey="value" nameKey="name" innerRadius={50} outerRadius={90} paddingAngle={2}>
                      {typeBars.map((_, i) => (
                        <Cell key={i} fill={PALETTE[i % PALETTE.length]} />
                      ))}
                    </Pie>
                    <Tooltip formatter={(v) => fmtMoney(Number(v), currency)} />
                  </PieChart>
                </ResponsiveContainer>
                <div className="w-full space-y-2">
                  {typeBars.map((t, i) => (
                    <div key={t.name} className="flex items-center justify-between border-b pb-2 text-sm last:border-0">
                      <span className="flex items-center gap-2">
                        <span className="size-2.5 rounded-full" style={{ background: PALETTE[i % PALETTE.length] }} />
                        {t.name}
                      </span>
                      <span className="font-mono tabular-nums">{fmtMoney(t.value, currency)}</span>
                    </div>
                  ))}
                </div>
              </div>
            ) : (
              <p className="py-16 text-center text-sm text-muted-foreground">No spend in the selected month.</p>
            )}
          </CardContent>
        </Card>
      </div>

      <Card className="mt-6 overflow-hidden py-0">
        <div className="border-b px-5 py-3 text-sm font-medium text-muted-foreground">Cost by project</div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Project</TableHead>
              <TableHead className="text-right">This month</TableHead>
              <TableHead className="text-right">Last month</TableHead>
              <TableHead className="text-right">Share</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r) => {
              const val = month === "current" ? r.current : r.last
              const share = scopeAllTotal(rows, month) > 0 ? (val / scopeAllTotal(rows, month)) * 100 : 0
              return (
                <TableRow key={r.id} className="cursor-pointer" onClick={() => setProj(r.id)}>
                  <TableCell className="font-medium">{r.name}</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">{fmtMoney(r.current, currency)}</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">{fmtMoney(r.last, currency)}</TableCell>
                  <TableCell className="text-right text-sm text-muted-foreground tabular-nums">{share.toFixed(0)}%</TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </Card>

      <Card className="mt-6 overflow-hidden py-0">
        <div className="border-b px-5 py-3 text-sm font-medium text-muted-foreground">
          Top resources this month{proj === "__all__" ? "" : ` — ${projName(proj)}`}
        </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Resource</TableHead>
              <TableHead>Type</TableHead>
              <TableHead className="text-right">Cost</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {topResources.length ? (
              topResources.map((r, i) => (
                <TableRow key={i}>
                  <TableCell className="font-medium">{r.resource?.name ?? r.resource?.id ?? "—"}</TableCell>
                  <TableCell>
                    <StatusBadge status={r.resource?.type} />
                  </TableCell>
                  <TableCell className="text-right font-mono tabular-nums">
                    {fmtMoney(n((r as Record<string, unknown>).currentCost ?? r.price), currency)}
                  </TableCell>
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={3} className="py-6 text-center text-sm text-muted-foreground">
                  Nothing billed yet this month.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </Card>
    </>
  )
}

// Sum of the selected month across all projects — the denominator for each project's share.
function scopeAllTotal(rows: Array<{ current: number; last: number }>, month: "current" | "last"): number {
  return rows.reduce((s, r) => s + (month === "current" ? r.current : r.last), 0)
}

function Stat({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <Card>
      <CardContent className="p-5">
        <p className="text-sm text-muted-foreground">{label}</p>
        <p className="mt-1 font-display text-2xl font-semibold tabular-nums">{value}</p>
        {sub ? <p className="mt-1 text-xs text-muted-foreground">{sub}</p> : null}
      </CardContent>
    </Card>
  )
}
