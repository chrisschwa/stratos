import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Cloud, Plus } from "lucide-react"
import { useNavigate } from "react-router-dom"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/PageHeader"
import { EmptyState } from "@/components/empty-state"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import { useAdminList } from "@/lib/hooks"

const LIST_PATH = "/admin/service"

export type CloudProvider = {
  id: string
  name?: string
  type?: string
  status?: string
  config?: {
    identityUrl?: string
    regions?: Record<string, unknown>
    services?: Record<string, Record<string, boolean>>
    auth?: Record<string, unknown>
  }
}

// ── create form ──────────────────────────────────────────────────────────────
type FormState = {
  name: string
  identityUrl: string
  adminUsername: string
  adminProjectName: string
  adminDomainName: string
  adminPassword: string
  region: string
  shared: boolean
}

const emptyForm: FormState = {
  name: "",
  identityUrl: "",
  adminUsername: "",
  adminProjectName: "",
  adminDomainName: "Default",
  adminPassword: "",
  region: "RegionOne",
  shared: false,
}

const formValid = (f: FormState) =>
  [f.name, f.identityUrl, f.adminUsername, f.adminProjectName, f.adminDomainName, f.adminPassword, f.region]
    .every((v) => v.trim() !== "")

// formToBody builds the ExternalService document the Go create handler (POST /admin/service) reads —
// the same shape as deploy/seed/external-service-dev.json. Services stay empty here; the operator
// enables per-region services + finishes Features/Quota on the detail page after "Test connection".
// A non-blank region displayName is required for the client create-form Location dropdown → use the
// region name. The secret carries the OpenStack admin password (stripped from every read response).
function formToBody(f: FormState) {
  const region = f.region.trim()
  return {
    name: f.name.trim(),
    type: "CLOUD",
    status: "PUBLIC",
    config: {
      identityUrl: f.identityUrl.trim(),
      provider: "openstack",
      shared: f.shared,
      auth: {
        adminAuthType: "password",
        adminUsername: f.adminUsername.trim(),
        adminProjectName: f.adminProjectName.trim(),
        adminDomainName: f.adminDomainName.trim(),
      },
      regions: { [region]: { name: region, country: "", displayName: region } },
      services: {},
      features: {},
      provisioning: { projectRoles: [] },
    },
    secret: { adminPassword: f.adminPassword },
  }
}

function ProviderForm({ form, setForm }: { form: FormState; setForm: (f: FormState) => void }) {
  return (
    <div className="grid gap-4">
      <div className="grid gap-2">
        <Label htmlFor="cp-name">Display name</Label>
        <Input id="cp-name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="dev region" />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="cp-identity">Identity URL (Keystone v3)</Label>
        <Input
          id="cp-identity"
          value={form.identityUrl}
          onChange={(e) => setForm({ ...form, identityUrl: e.target.value })}
          placeholder="https://keystone.example.com:5000/v3"
        />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="grid gap-2">
          <Label htmlFor="cp-user">Admin username</Label>
          <Input id="cp-user" value={form.adminUsername} onChange={(e) => setForm({ ...form, adminUsername: e.target.value })} autoComplete="off" />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="cp-project">Admin project</Label>
          <Input id="cp-project" value={form.adminProjectName} onChange={(e) => setForm({ ...form, adminProjectName: e.target.value })} placeholder="admin" />
        </div>
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="grid gap-2">
          <Label htmlFor="cp-domain">Admin domain</Label>
          <Input id="cp-domain" value={form.adminDomainName} onChange={(e) => setForm({ ...form, adminDomainName: e.target.value })} placeholder="Default" />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="cp-region">Region</Label>
          <Input id="cp-region" value={form.region} onChange={(e) => setForm({ ...form, region: e.target.value })} placeholder="RegionOne" />
        </div>
      </div>
      <div className="grid gap-2">
        <Label htmlFor="cp-pass">Admin password</Label>
        <Input
          id="cp-pass"
          type="password"
          value={form.adminPassword}
          onChange={(e) => setForm({ ...form, adminPassword: e.target.value })}
          autoComplete="new-password"
        />
      </div>
      <div className="flex items-center justify-between rounded-lg border p-3">
        <div>
          <div className="text-sm font-medium">Shared provider</div>
          <div className="text-xs text-muted-foreground">One OpenStack tenant shared by all projects (no per-project tenant).</div>
        </div>
        <Switch checked={form.shared} onCheckedChange={(on) => setForm({ ...form, shared: on })} />
      </div>
    </div>
  )
}

export default function CloudProvidersPage() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { data, isLoading, error } = useAdminList<CloudProvider>(LIST_PATH)
  const items = data?.data ?? []

  const [createOpen, setCreateOpen] = useState(false)
  const [form, setForm] = useState<FormState>(emptyForm)

  const create = useMutation({
    // POST /admin/service (externalServiceCreate). The operator finishes the Services/Features tabs on
    // the detail page (it has its own "Test connection") — so we go straight there on success.
    mutationFn: () => apiFetch<CloudProvider>(LIST_PATH, { method: "POST", body: formToBody(form) }),
    onSuccess: (created) => {
      toast.success("Cloud provider created")
      setCreateOpen(false)
      setForm(emptyForm)
      void qc.invalidateQueries({ queryKey: ["admin-list", LIST_PATH] })
      if (created?.id) navigate(`/system/cloud-providers/${created.id}`)
    },
    // Go's create runs a live Keystone auto-fill + provisioning; on this deployment it is a seam (501).
    // Surface the API message so the operator sees why it did not persist.
    onError: (e: Error) => toast.error(e.message),
  })

  const addBtn = (
    <Button size="sm" onClick={() => setCreateOpen(true)}>
      <Plus className="size-4" /> Add provider
    </Button>
  )

  return (
    <>
      <PageHeader
        title="Cloud providers"
        description="External services connected to the platform."
        actions={addBtn}
      />
      {isLoading ? (
        <Skeleton className="h-64" />
      ) : error ? (
        <div className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground">{(error as Error).message}</div>
      ) : items.length === 0 ? (
        <EmptyState
          icon={Cloud}
          title="No cloud providers"
          hint="Connect an OpenStack cloud so projects can provision resources."
          action={addBtn}
        />
      ) : (
        <Card className="overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>Name</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Identity URL</TableHead>
                <TableHead>Regions</TableHead>
                <TableHead>Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((p) => (
                <TableRow
                  key={p.id}
                  className="cursor-pointer"
                  onClick={() => navigate(`/system/cloud-providers/${p.id}`)}
                >
                  <TableCell className="font-mono text-xs">{p.id}</TableCell>
                  <TableCell className="font-medium">{p.name ?? "—"}</TableCell>
                  <TableCell>{p.type ?? "—"}</TableCell>
                  <TableCell className="font-mono text-xs">{p.config?.identityUrl ?? "—"}</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {Object.keys(p.config?.regions ?? {}).map((r) => (
                        <Badge key={r} variant="outline">{r}</Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={p.status} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add cloud provider</DialogTitle>
            <DialogDescription>
              Connect an OpenStack cloud with its Keystone admin credentials. You will enable per-region services and
              run "Test connection" on the provider page after it is created.
            </DialogDescription>
          </DialogHeader>
          <ProviderForm form={form} setForm={setForm} />
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => create.mutate()} disabled={!formValid(form) || create.isPending}>
              {create.isPending ? "Creating…" : "Create provider"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
