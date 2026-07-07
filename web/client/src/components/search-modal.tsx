import { useEffect, useMemo, useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { useNavigate } from "react-router-dom"
import {
  Archive, Box, Boxes, Cable, Camera, FolderKanban, FolderTree, Globe, HardDrive,
  Key, Layers, Lock, Network, Receipt, Route, Scale, Search, Server, Shield,
  type LucideIcon,
} from "lucide-react"
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog"
import { cn } from "@/lib/utils"
import { apiFetch } from "@/lib/api"

// GET /search/{projectId} → {data:[{type, data:{name,…,id,region}}]} (Go clientcloud.go search).
// The set is prefilled — the FE filters client-side; there is no query param.
type SearchItem = {
  type: string
  data?: Record<string, any>
}

const TYPE_ICONS: Record<string, LucideIcon> = {
  SERVER: Server,
  NETWORK: Network,
  ROUTER: Route,
  SUBNET: Network,
  PORT: Cable,
  FLOATING_IP: Globe,
  SECURITY_GROUP: Shield,
  IMAGE: Camera,
  VOLUME: HardDrive,
  VOLUME_SNAPSHOT: Camera,
  KEYPAIR: Key,
  BUCKET: Archive,
  SHARE: FolderTree,
  LOAD_BALANCER: Scale,
  DNS_ZONE: Globe,
  STACK: Layers,
  BARBICAN_SECRET: Lock,
  SERVER_GROUP: Boxes,
  PROJECT: FolderKanban,
  BILL: Receipt,
}

// Everything without a dedicated detail route goes to its list page.
const LIST_PAGES: Record<string, string> = {
  VOLUME: "volumes",
  VOLUME_SNAPSHOT: "snapshots",
  BUCKET: "object-storage",
  SHARE: "shares",
  ROUTER: "routers",
  SUBNET: "networks",
  PORT: "ports",
  FLOATING_IP: "floating-ips",
  LOAD_BALANCER: "load-balancers",
  DNS_ZONE: "dns",
  STACK: "stacks",
  BARBICAN_SECRET: "secrets",
  KEYPAIR: "keypairs",
  SERVER_GROUP: "server-groups",
  IMAGE: "images",
}

function typeLabel(t: string): string {
  return t.replaceAll("_", " ").toLowerCase()
}

export default function SearchModal({
  pid, open, onOpenChange,
}: {
  pid: string
  open: boolean
  onOpenChange: (o: boolean) => void
}) {
  const nav = useNavigate()
  const inputRef = useRef<HTMLInputElement>(null)
  const [raw, setRaw] = useState("")
  const [query, setQuery] = useState("")
  const [active, setActive] = useState(0)

  // Fetch once per open (staleTime keeps re-opens cheap); filtering is client-side.
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["search", pid],
    queryFn: () => apiFetch<SearchItem[]>(`/search/${pid}`),
    enabled: open && !!pid,
    staleTime: 30_000,
  })

  // Debounce the input 200ms before filtering.
  useEffect(() => {
    const t = setTimeout(() => setQuery(raw), 200)
    return () => clearTimeout(t)
  }, [raw])

  // Reset on every open.
  useEffect(() => {
    if (open) {
      setRaw("")
      setQuery("")
      setActive(0)
    }
  }, [open])

  const filtered = useMemo(() => {
    const list = data ?? []
    const needle = query.trim().toLowerCase()
    if (!needle) return list
    const tokens = needle.split(/\s+/)
    return list.filter((it) => {
      const hay = [it.type, ...Object.values(it.data ?? {})]
        .filter((v) => typeof v === "string" || typeof v === "number")
        .join(" ")
        .toLowerCase()
      return tokens.every((t) => hay.includes(t))
    })
  }, [data, query])

  const groups = useMemo(() => {
    const m = new Map<string, SearchItem[]>()
    for (const it of filtered) {
      const k = it.type || "OTHER"
      const arr = m.get(k)
      if (arr) arr.push(it)
      else m.set(k, [it])
    }
    return [...m.entries()]
  }, [filtered])

  // Flat order (matches render order) for arrow-key navigation.
  const flat = useMemo(() => groups.flatMap(([, arr]) => arr), [groups])

  useEffect(() => {
    if (active >= flat.length) setActive(0)
  }, [flat.length, active])

  const go = (item: SearchItem) => {
    const id = (item.data?.id as string) ?? ""
    switch (item.type) {
      case "SERVER":
        nav(`/p/${pid}/servers/${id}`)
        break
      case "NETWORK":
        nav(`/p/${pid}/networks/${id}`)
        break
      case "SECURITY_GROUP":
        nav(`/p/${pid}/security-groups/${id}`)
        break
      case "BILL":
        nav(`/p/${pid}/billing/history/bills/${id}`)
        break
      case "PROJECT":
        nav(`/p/${id}/dashboard`)
        break
      default:
        nav(`/p/${pid}/${LIST_PAGES[item.type] ?? "dashboard"}`)
    }
    onOpenChange(false)
  }

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") {
      e.preventDefault()
      setActive((a) => Math.min(a + 1, Math.max(flat.length - 1, 0)))
    } else if (e.key === "ArrowUp") {
      e.preventDefault()
      setActive((a) => Math.max(a - 1, 0))
    } else if (e.key === "Enter") {
      e.preventDefault()
      const item = flat[active]
      if (item) go(item)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="gap-0 overflow-hidden p-0 sm:max-w-xl"
        showCloseButton={false}
        onOpenAutoFocus={(e) => {
          e.preventDefault()
          inputRef.current?.focus()
        }}
      >
        <DialogTitle className="sr-only">Search this project</DialogTitle>
        <div className="flex items-center gap-2 border-b px-4">
          <Search className="size-4 shrink-0 text-muted-foreground" />
          <input
            ref={inputRef}
            value={raw}
            onChange={(e) => setRaw(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Search servers, networks, volumes, bills…"
            className="h-12 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            aria-label="Search this project"
          />
        </div>

        <div className="max-h-[60vh] overflow-y-auto p-2">
          {isLoading ? (
            <p className="py-8 text-center text-sm text-muted-foreground">Loading…</p>
          ) : isError ? (
            <p className="py-8 text-center text-sm text-muted-foreground">{(error as Error).message}</p>
          ) : !flat.length ? (
            <p className="py-8 text-center text-sm text-muted-foreground">
              {query.trim() ? "No matches." : "Nothing to search yet."}
            </p>
          ) : (
            groups.map(([type, items]) => {
              const Icon = TYPE_ICONS[type] ?? Box
              return (
                <div key={type} className="mb-1">
                  <p className="px-2 py-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    {typeLabel(type)}
                  </p>
                  {items.map((item, i) => {
                    const idx = flat.indexOf(items[i])
                    const name = (item.data?.name as string) || (item.data?.id as string) || "—"
                    const sub = [item.data?.status, item.data?.ipv4, item.data?.flavor, item.data?.region]
                      .filter((v) => typeof v === "string" && v)
                      .join(" · ")
                    return (
                      <button
                        key={`${type}-${item.data?.id ?? i}`}
                        onClick={() => go(item)}
                        onMouseEnter={() => setActive(idx)}
                        className={cn(
                          "flex w-full items-center gap-3 rounded-md px-2 py-2 text-left text-sm",
                          idx === active ? "bg-accent text-accent-foreground" : "hover:bg-accent/50",
                        )}
                      >
                        <Icon className="size-4 shrink-0 text-muted-foreground" />
                        <span className="truncate font-medium">{name}</span>
                        {sub && <span className="ml-auto truncate text-xs text-muted-foreground">{sub}</span>}
                      </button>
                    )
                  })}
                </div>
              )
            })
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
