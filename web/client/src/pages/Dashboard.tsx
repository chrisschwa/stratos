import { Link } from "react-router-dom"
import { HardDrive, Server, Wallet } from "lucide-react"
import { PageHeader } from "@/components/layout/PageHeader"
import { StatusBadge } from "@/components/status-badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useBillingSummary, useCostInfo, useProject, useProjectId } from "@/lib/hooks"
import { fmtMoney, timeAgo } from "@/lib/format"

function Stat({
  label,
  value,
  sub,
  icon: Icon,
}: {
  label: string
  value: string
  sub?: string
  icon: React.ComponentType<{ className?: string }>
}) {
  return (
    <Card>
      <CardContent className="flex items-start justify-between p-5">
        <div>
          <p className="text-sm text-muted-foreground">{label}</p>
          <p className="mt-1 font-display text-2xl font-semibold tabular-nums">{value}</p>
          {sub ? <p className="mt-1 text-xs text-muted-foreground">{sub}</p> : null}
        </div>
        <Icon className="size-5 text-muted-foreground/50" />
      </CardContent>
    </Card>
  )
}

export function DashboardPage() {
  const pid = useProjectId()
  const { project } = useProject(pid)
  const { data: cost, isLoading: costLoading } = useCostInfo(pid)
  const { data: summary } = useBillingSummary(pid)

  const currency = (summary?.currency as string) ?? "USD"
  const projectCost = cost?.projects?.[pid] ?? cost

  return (
    <>
      <PageHeader
        title={project?.name ?? "Dashboard"}
        description="Live usage, balance and this month's spend for this project."
      />

      {costLoading ? (
        <div className="grid gap-4 md:grid-cols-3">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-28" />
          ))}
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-3">
          <Stat
            label="Balance"
            value={fmtMoney(summary?.balance, currency)}
            sub={`Account credit ${fmtMoney(summary?.accountCredit, currency)} · Promo ${fmtMoney(summary?.promotionalCredit, currency)}`}
            icon={Wallet}
          />
          <Stat
            label="This month"
            value={fmtMoney(projectCost?.currentMonthCosts, currency)}
            sub={`Forecast ${fmtMoney(projectCost?.forecastedMonthEndCosts, currency)}`}
            icon={Server}
          />
          <Stat
            label="Due now"
            value={fmtMoney(projectCost?.dueAmount ?? 0, currency)}
            sub={summary?.status ? `Billing profile ${String(summary.status).toLowerCase()}` : undefined}
            icon={HardDrive}
          />
        </div>
      )}

      <div className="mt-6 grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Cost by service</CardTitle>
          </CardHeader>
          <CardContent>
            {projectCost?.currentMonthCostsByType && Object.keys(projectCost.currentMonthCostsByType).length > 0 ? (
              <div className="space-y-2">
                {Object.entries(projectCost.currentMonthCostsByType).map(([k, v]) => (
                  <div key={k} className="flex items-center justify-between border-b pb-2 text-sm last:border-0">
                    <span>{k}</span>
                    <span className="font-mono tabular-nums">{fmtMoney(v, currency)}</span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="py-6 text-center text-sm text-muted-foreground">No usage recorded yet this month.</p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Top cost generators</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {projectCost?.topResourcePrices?.length ? (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Resource</TableHead>
                    <TableHead>Type</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead className="text-right">Cost</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {projectCost.topResourcePrices.map((r, i) => (
                    <TableRow key={i}>
                      <TableCell className="font-medium">
                        {r.resource?.type === "SERVER" && r.resource?.id ? (
                          <Link className="hover:underline" to={`/p/${pid}/servers/${r.resource.id}`}>
                            {r.resource?.name ?? r.resource?.id}
                          </Link>
                        ) : (
                          (r.resource?.name ?? r.resource?.id ?? "—")
                        )}
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={r.resource?.type} />
                      </TableCell>
                      <TableCell className="text-muted-foreground">{timeAgo(r.resource?.createdAt)}</TableCell>
                      <TableCell className="text-right font-mono tabular-nums">{fmtMoney(r.currentCost ?? r.price, currency)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ) : (
              <p className="py-6 text-center text-sm text-muted-foreground">Nothing billed yet.</p>
            )}
          </CardContent>
        </Card>
      </div>
    </>
  )
}
