import { useParams } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import { PageHeader } from "@/components/layout/PageHeader"
import { StatusBadge } from "@/components/status-badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { apiFetch } from "@/lib/api"
import { useCloudResource, useCloudScope, useProjectId } from "@/lib/hooks"
import type { CloudResource } from "@/lib/types"
import { networkName, networkStatus } from "./NetworksPage"

export default function NetworkDetailPage() {
  const pid = useProjectId()
  const { resourceId = "" } = useParams()
  const { data: network, isLoading, error } = useCloudResource(pid, resourceId)

  if (isLoading || (!network && !error)) {
    return (
      <>
        <PageHeader title="Network" />
        <Skeleton className="h-72" />
      </>
    )
  }
  if (error || !network) {
    return (
      <>
        <PageHeader title="Network" />
        <div className="rounded-lg border bg-muted/40 p-6 text-sm text-muted-foreground">
          {(error as Error | null)?.message ?? "Network not found."}
        </div>
      </>
    )
  }

  const net = (network.data?.network ?? {}) as Record<string, unknown>
  const subnets = (net.subnets as string[] | undefined) ?? []

  return (
    <>
      <PageHeader title={networkName(network)} description="Network detail." />

      <div className="mb-4 flex items-center gap-3">
        <StatusBadge status={networkStatus(network)} />
        <span className="font-mono text-xs text-muted-foreground">{network.externalId}</span>
      </div>

      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="servers">Servers</TabsTrigger>
          <TabsTrigger value="subnets">Subnets</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Details</CardTitle>
            </CardHeader>
            <CardContent>
              <dl className="grid gap-x-8 gap-y-3 text-sm sm:grid-cols-2">
                <Row k="ID" v={<span className="font-mono text-xs">{(net.id as string) ?? network.externalId ?? "—"}</span>} />
                <Row k="Status" v={<StatusBadge status={networkStatus(network)} />} />
                <Row k="MTU" v={net.mtu != null ? String(net.mtu) : "—"} />
                <Row k="Shared" v={net.shared ? "Yes" : "No"} />
                <Row k="External" v={net["router:external"] ? "Yes" : "No"} />
                <Row k="Admin state up" v={net.admin_state_up === false ? "No" : "Yes"} />
                <Row k="Subnets" v={String(subnets.length)} />
              </dl>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="servers" className="mt-4">
          <NetworkServers pid={pid} resourceId={resourceId} />
        </TabsContent>

        <TabsContent value="subnets" className="mt-4">
          <Card>
            <CardContent className="pt-6">
              {subnets.length ? (
                <ul className="grid gap-2 text-sm">
                  {subnets.map((s) => (
                    <li key={s} className="border-b pb-2 font-mono text-xs last:border-0">
                      {s}
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="py-4 text-center text-sm text-muted-foreground">No subnets on this network.</p>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </>
  )
}

// Servers attached to this network — cloud action GET_SERVERS → data.result: CloudResource[].
function NetworkServers({ pid, resourceId }: { pid: string; resourceId: string }) {
  const scope = useCloudScope(pid)
  const { data, isLoading, error } = useQuery({
    queryKey: ["network-servers", pid, resourceId],
    queryFn: () =>
      apiFetch<{ result?: CloudResource[] }>(`/project/${pid}/cloud/${resourceId}/action`, {
        method: "POST",
        body: { action: "GET_SERVERS" },
        cloud: scope,
      }),
    enabled: !!pid && !!resourceId && !!scope,
  })

  if (isLoading) return <Skeleton className="h-40" />
  if (error) {
    return (
      <div className="rounded-lg border bg-muted/40 p-6 text-sm text-muted-foreground">{(error as Error).message}</div>
    )
  }
  const servers = data?.result ?? []
  return (
    <Card>
      <CardContent className="pt-6">
        {servers.length ? (
          <ul className="grid gap-2 text-sm">
            {servers.map((s) => (
              <li key={s.id} className="flex items-center justify-between border-b pb-2 last:border-0">
                <span className="font-medium">{(s.data?.server?.name as string) ?? s.id}</span>
                <StatusBadge status={(s.data?.server?.status as string) ?? s.status} />
              </li>
            ))}
          </ul>
        ) : (
          <p className="py-4 text-center text-sm text-muted-foreground">No servers attached to this network.</p>
        )}
      </CardContent>
    </Card>
  )
}

function Row({ k, v }: { k: string; v: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b pb-2 last:border-0 sm:border-0 sm:pb-0">
      <dt className="text-muted-foreground">{k}</dt>
      <dd className="text-right">{v}</dd>
    </div>
  )
}
