import { useQuery } from "@tanstack/react-query"
import { apiFetch, apiFetchEnvelope, type Envelope } from "./api"

// Generic paged admin list — most /admin/* collections share the
// { data: [...], paging } envelope.
export function useAdminList<T = Record<string, any>>(path: string, enabled = true) {
  return useQuery({
    queryKey: ["admin-list", path],
    queryFn: () => apiFetchEnvelope<T[]>(path) as Promise<Envelope<T[]>>,
    enabled,
  })
}

export function useAdminGet<T = Record<string, any>>(path: string, enabled = true) {
  return useQuery({
    queryKey: ["admin-get", path],
    queryFn: () => apiFetch<T>(path),
    enabled,
  })
}

export type AdminStats = {
  users?: number
  projects?: number
  cloudResources?: number
  transactions?: number
  cloudProviderConfigured?: boolean
  billingConfigured?: boolean
  brandingConfigured?: boolean
  mailGatewayConfigured?: boolean
  pricePlanConfigured?: boolean
  insights?: {
    bills?: Array<{ year: number; month: number; total?: Record<string, number> }>
    payments?: Array<{ year: number; month: number; total?: Record<string, number> }>
    newUsers?: Array<{ year: number; month: number; day: number; count: number }>
    newBillingProfiles?: Array<{ year: number; month: number; day: number; count: number }>
  }
  [k: string]: unknown
}

export function useAdminStats() {
  return useQuery({
    queryKey: ["admin-stats"],
    queryFn: () => apiFetch<AdminStats>("/admin/stats"),
  })
}
