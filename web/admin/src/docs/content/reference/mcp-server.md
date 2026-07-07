# MCP Server

Stratos bundles a [Model Context Protocol](https://modelcontextprotocol.io) server, so AI agents — Claude Code, Claude Desktop, or any MCP-capable client — can drive the platform through curated tools rather than raw HTTP.

- **Endpoint:** `https://<api-host>/mcp` (Streamable HTTP, stateless — safe behind multiple API replicas)
- **Toolsets:** the tools you're offered depend on who you are. Admin principals get the admin toolset (users, organizations, projects, billing profiles, credits); end users signing in with a portal account get the client toolset (their own projects and cloud resources).

Every tool call runs against the very same REST endpoints documented in this reference — same permissions, same validation, same audit trail. MCP opens no privileged side door.

## Authentication

There are two ways in.

### 1. OAuth sign-in (interactive)

Add the server with no credentials and let your MCP client run the standard OAuth flow:

```bash
claude mcp add --transport http stratos https://<api-host>/mcp
```

On first use the server replies `401` with an RFC 9728 resource-metadata document pointing at the Stratos identity provider. Your MCP client discovers it, registers itself via dynamic client registration, opens a browser window, and redirects back to a local port with an authorization code (PKCE). No server-side setup required.

- Signing in with a **customer account** (clients realm) grants the **client toolset**.
- Signing in with an **admin account** (admin realm) grants the **admin toolset**.

If your MCP client can't do dynamic client registration, a pre-registered public client `stratos-mcp` exists in both realms (PKCE required).

### 2. API key (non-interactive)

Create an HMAC key pair under **System → HMAC Keys** in the admin console (the secret is shown once). Then point the MCP client at the server with a static bearer header that joins the pair with a dot:

```json
{
  "mcpServers": {
    "stratos-admin": {
      "type": "http",
      "url": "https://<api-host>/mcp",
      "headers": {
        "Authorization": "Bearer pk<32hex>.sk<40hex>"
      }
    }
  }
}
```

API-key principals always get the **admin toolset**. Treat the pair like any admin credential: it's checked with a constant-time compare, but it rides in the header — use HTTPS only, and rotate it via System → HMAC Keys.

## The admin toolset

Read tools: `list_users`, `get_user`, `list_organizations`, `get_organization`, `list_org_members`, `list_projects`, `get_project`, `list_billing_profiles`, `get_billing_profile`, `list_bills`, `get_bill`, `list_account_credits`, `get_account_credit`, `list_service_providers`, `get_service_provider`.

Write tools: `create_user`, `delete_user`, `create_organization`, `create_project`, `provision_project`, `activate_billing_profile`, `suspend_billing_profile`, `resume_billing_profile`, `create_account_credit`, `delete_account_credit`.

The semantics — status codes, pagination markers, error envelopes — line up exactly with the [Admin API reference](/docs/reference/overview); each tool is a thin wrapper over the matching endpoint.

## Good to know

- Responses are the raw JSON envelopes from the REST API; failed calls surface as tool errors carrying the HTTP status and body.
- Keyset pagination behaves the same way: pass the previous page's `next_marker` back as `marker`.
- The endpoint lives on the API host/ingress at path `/mcp` — there's no extra service or port to run.
