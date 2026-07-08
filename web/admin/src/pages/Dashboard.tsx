import { CheckCircle2, CircleAlert, CreditCard, Cpu, FolderKanban, Server, Users } from "lucide-react"
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts"
import { PageHeader } from "@/components/layout/PageHeader"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { useAdminList, useAdminStats } from "@/lib/hooks"

function Stat({ label, value, icon: Icon }: { label: string; value: number | string | undefined; icon: React.ComponentType<{ className?: string }> }) {
  return (
    <Card>
      <CardContent className="flex items-start justify-between p-5">
        <div>
          <p className="text-sm text-muted-foreground">{label}</p>
          <p className="mt-1 font-display text-2xl font-semibold tabular-nums">{value ?? "—"}</p>
        </div>
        <Icon className="size-5 text-muted-foreground/50" />
      </CardContent>
    </Card>
  )
}

// GPU capacity (placement gpu-info) — one row block per cloud provider; hides itself
// when the provider reports no GPU resource providers.
type GpuRegionCapacity = { region: string; gpus: Array<{ name: string; total: number; inUse: number }> }

function ServiceGpuRows({ svc }: { svc: { id: string; name?: string } }) {
  const q = useAdminList<GpuRegionCapacity>(`/admin/service/${svc.id}/gpu-info`, !!svc.id)
  if (q.isLoading) return <Skeleton className="h-10" />
  const rows = (q.data?.data ?? []).flatMap((r) => r.gpus.map((g) => ({ ...g, region: r.region })))
  if (rows.length === 0) return null
  return (
    <div className="space-y-2">
      <p className="text-xs text-muted-foreground">{svc.name ?? svc.id}</p>
      {rows.map((g) => (
        <div key={`${g.region}-${g.name}`} className="flex items-center gap-3 text-sm">
          <span className="w-40 truncate font-mono text-xs">{g.name}</span>
          <div className="h-2 flex-1 rounded bg-muted">
            <div
              className="h-2 rounded bg-primary"
              style={{ width: `${g.total ? Math.round((g.inUse / g.total) * 100) : 0}%` }}
            />
          </div>
          <span className="w-20 text-right tabular-nums text-xs text-muted-foreground">
            {g.inUse}/{g.total} used
          </span>
        </div>
      ))}
    </div>
  )
}

function GpuCapacityCard() {
  const services = useAdminList<{ id: string; name?: string }>("/admin/service")
  const list = services.data?.data ?? []
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Cpu className="size-4 text-muted-foreground/70" /> GPU capacity
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {services.isLoading ? (
          <Skeleton className="h-16" />
        ) : list.length === 0 ? (
          <p className="text-sm text-muted-foreground">No cloud providers configured.</p>
        ) : (
          list.map((s) => <ServiceGpuRows key={s.id} svc={s} />)
        )}
      </CardContent>
    </Card>
  )
}

function SetupRow({ label, ok }: { label: string; ok?: boolean }) {
  return (
    <div className="flex items-center justify-between border-b py-2 text-sm last:border-0">
      <span>{label}</span>
      {ok ? (
        <span className="inline-flex items-center gap-1 text-ok">
          <CheckCircle2 className="size-4" /> Configured
        </span>
      ) : (
        <span className="inline-flex items-center gap-1 text-warn">
          <CircleAlert className="size-4" /> Not set up
        </span>
      )}
    </div>
  )
}

export function DashboardPage() {
  const { data, isLoading } = useAdminStats()

  const payments = (data?.insights?.payments ?? []).map((p) => ({
    label: `${p.year}-${String(p.month).padStart(2, "0")}`,
    total: Object.values(p.total ?? {}).reduce((a, b) => a + Number(b || 0), 0),
  }))
  const newUsers = (data?.insights?.newUsers ?? []).map((d) => ({
    label: `${String(d.month).padStart(2, "0")}-${String(d.day).padStart(2, "0")}`,
    count: d.count,
  }))

  return (
    <>
      <PageHeader title="Dashboard" description="Platform-wide activity at a glance." />
      {isLoading ? (
        <div className="grid gap-4 md:grid-cols-4">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-24" />
          ))}
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-4">
          <Stat label="Users" value={data?.users} icon={Users} />
          <Stat label="Projects" value={data?.projects} icon={FolderKanban} />
          <Stat label="Cloud resources" value={data?.cloudResources} icon={Server} />
          <Stat label="Transactions" value={data?.transactions} icon={CreditCard} />
        </div>
      )}

      <div className="mt-6 grid gap-4 lg:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Platform setup</CardTitle>
          </CardHeader>
          <CardContent>
            <SetupRow label="Cloud provider" ok={data?.cloudProviderConfigured} />
            <SetupRow label="Billing" ok={data?.billingConfigured} />
            <SetupRow label="Branding" ok={data?.brandingConfigured} />
            <SetupRow label="Mail gateway" ok={data?.mailGatewayConfigured} />
            <SetupRow label="Price plan" ok={data?.pricePlanConfigured} />
          </CardContent>
        </Card>
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle className="text-base">Inbound payments (12 months)</CardTitle>
          </CardHeader>
          <CardContent className="h-56">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={payments}>
                <defs>
                  <linearGradient id="pay" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="#4f46e5" stopOpacity={0.5} />
                    <stop offset="100%" stopColor="#4f46e5" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="label" tick={{ fontSize: 11 }} />
                <YAxis tick={{ fontSize: 11 }} width={40} />
                <Tooltip />
                <Area type="monotone" dataKey="total" stroke="#4f46e5" fill="url(#pay)" />
              </AreaChart>
            </ResponsiveContainer>
          </CardContent>
        </Card>
      </div>

      <div className="mt-4">
        <GpuCapacityCard />
      </div>

      <div className="mt-4">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">New users (30 days)</CardTitle>
          </CardHeader>
          <CardContent className="h-56">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={newUsers}>
                <defs>
                  <linearGradient id="nu" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="#f59e0b" stopOpacity={0.5} />
                    <stop offset="100%" stopColor="#f59e0b" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="label" tick={{ fontSize: 11 }} />
                <YAxis tick={{ fontSize: 11 }} width={40} allowDecimals={false} />
                <Tooltip />
                <Area type="monotone" dataKey="count" stroke="#f59e0b" fill="url(#nu)" />
              </AreaChart>
            </ResponsiveContainer>
          </CardContent>
        </Card>
      </div>
    </>
  )
}
