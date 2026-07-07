import { CheckCircle2, CircleAlert, CreditCard, FolderKanban, Server, Users } from "lucide-react"
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts"
import { PageHeader } from "@/components/layout/PageHeader"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { useAdminStats } from "@/lib/hooks"

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
