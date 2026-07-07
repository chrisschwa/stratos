// Runtime config for the admin console (window.__STRATOS__ from /config.js).
export type StratosConfig = {
  apiUrl: string
  authIssuer: string
  authClientId: string
  authScope: string
}

declare global {
  interface Window {
    __STRATOS__?: Partial<StratosConfig>
  }
}

const defaults: StratosConfig = {
  apiUrl: "https://stratos-cloud-api.menlo.ai/api/v1",
  authIssuer: "https://stratos-cloud-auth.menlo.ai/realms/master",
  authClientId: "stratos-admin",
  authScope: "openid profile email",
}

export const config: StratosConfig = { ...defaults, ...(window.__STRATOS__ ?? {}) }
