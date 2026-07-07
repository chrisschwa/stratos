# How Identity Works

Stratos keeps no passwords of its own. Every sign-in is delegated to an OpenID Connect provider — either the **Keycloak bundled with the Helm chart** (the default) or **any external OIDC provider** you already run. The Stratos API behaves purely as an OAuth2 resource server: it validates the JWTs the SPAs bring back and identifies the platform user by the token's **email** claim.

## Two audiences, two realms

Customer identity and operator identity are kept deliberately apart:

| Audience | Realm (bundled Keycloak) | SPA | Default client ID | Self-registration |
|----------|--------------------------|-----|-------------------|-------------------|
| Customers | `clients` | Customer portal (`stratos-web`) | `stratos-ui` | Enabled |
| Operators/admins | `master` | Admin console (`stratos-admin`, under `/stratos_admin`) | `stratos-admin`, plus `stratos-admin-api` for API access | Disabled — admins are provisioned by hand |

Both SPAs are public OIDC clients running the authorization-code flow with PKCE. Authenticating against the admin realm marks a user as an administrator; a customer holding a `clients`-realm token can never reach the admin console.

## The bundled Keycloak (default)

Set `keycloakx.enabled: true` and the chart runs Keycloak — the official image, via [codecentric/keycloakx](https://github.com/codecentric/helm-charts/tree/master/charts/keycloakx) — pointed at a PostgreSQL you provide (the bundled `postgresql`/`cnpg`, or a managed one; set `keycloakx.database.*`). Supply a realm export through `realmImport` to create both realms, the three clients and their redirect URIs; set the bootstrap admin in `keycloakx.extraEnv` and `smtp.*` for verification and password-reset mail. Keycloak is served at `auth.<your-hostname>` by default.

Because Keycloak fronts identity, its features come along at no extra cost: email verification, password recovery, TOTP two-factor auth, and identity brokering — social logins like Google or GitHub, or federation with LDAP, Active Directory, or SAML. You configure these per realm in the Keycloak admin console.

Registration itself is realm-side. The customer SPA simply redirects to the authorize endpoint, and Keycloak shows the "Register" link because the `clients` realm has user registration turned on, with **email as the username**. The first time a user signs in, Stratos creates the platform user from the token — get-or-create by email — and runs any sign-up bonus or activation logic.

## Bringing your own OIDC provider

Set `keycloakx.enabled: false` and aim Stratos at your own provider — an existing Keycloak, Okta, Auth0, or anything spec-compliant:

```yaml
keycloakx:
  enabled: false
externalOpenid:
  issuer:      "https://auth.example.com/realms/clients"       # end-user issuer
  adminIssuer: "https://admin-auth.example.com/realms/master"  # admin issuer
```

`issuer` and `adminIssuer` are independent: your customer auth domain and your admin auth domain can differ, and can even live on entirely different providers.

On an external IdP the chart provisions **nothing** — you set it up yourself, per realm/tenant:

- A **public** OIDC client (authorization code + PKCE) whose client ID matches the configured one, with `redirectUris` covering the app URL plus `/*` and `webOrigins` set to the app origin. The SPAs redirect back to `<app origin>/` exactly, so that URL must be permitted.
- Tokens that carry the **email** claim.
- On the end-user side only: self-registration enabled **with email as the username**. Stratos keys both users and billing profiles by email — username-based registration breaks user creation and billing.
- SMTP on the realm, so verification and reset mails actually send.

The client IDs stay configurable through `ui.oauth2.clientId`, `admin.oauth2.clientId`, and `adminApi.oauth2.clientId`.

## One identity across OpenStack too

The same Keycloak can authenticate users against OpenStack Keystone and Horizon as well, giving customers a single login across Stratos and native OpenStack tooling. That federation is an operator setup task — see [Single Sign-On](/docs/self-hosting/sso).
