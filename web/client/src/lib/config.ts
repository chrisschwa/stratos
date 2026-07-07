// Runtime config, injected by the nginx entrypoint as /config.js
// (window.__STRATOS__). Dev falls back to the live stratos-cloud endpoints so
// `npm run dev` works against the real test world.
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
  authIssuer: "https://stratos-cloud-auth.menlo.ai/realms/clients",
  authClientId: "stratos-ui",
  authScope: "openid profile email",
}

export const config: StratosConfig = { ...defaults, ...(window.__STRATOS__ ?? {}) }
