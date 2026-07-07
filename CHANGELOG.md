# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0]

Initial release of Stratos — a multi-tenant cloud billing and self-service
portal for OpenStack.

### Added

- **Customer console** (`web/client`) — self-service management of OpenStack
  resources (instances, volumes, networks, floating IPs, load balancers, object
  storage, and shares) plus billing, invoices, and account settings.
- **Operator console** (`web/admin`) — administration of pricing, regions,
  invoicing, promotions, and customer accounts.
- **Go API** serving three surfaces from one process:
  - `/api/v1` — the customer API,
  - `/admin-api/v1` — the SigV4-signed operator API,
  - `/mcp` — a Model Context Protocol server for AI agents.
- **Usage-based billing** — metered consumption rated against configurable price
  plans, resource types, currencies, and tax rules, with automated invoicing and
  overdue-account suspension.
- **Payments** — card and bank-transfer payments and refunds via Stripe, account
  credits, and sign-up bonuses.
- **Savings plans & promotions** — commitment discounts and promotional credits.
- **Multi-org / multi-project RBAC** — organizations, projects, per-project
  roles, and user invitations.
- **OpenStack integration** — resource sync, usage-metrics collection, and event
  notifications, with per-service enable/disable across one or more regions.
- **Identity** — sign-in via Keycloak or any OpenID Connect provider using the
  authorization-code + PKCE flow.
- **In-app documentation** — task and concept guides for tenants plus a
  platform-admin guide and Admin API reference, served at `/docs` in each console.
- **Deployment** — a `docker-compose.yml` for local development and a Helm chart
  (`deploy/chart`) that bundles PostgreSQL, RabbitMQ, and Keycloak (each
  toggle-able) for Kubernetes. Container images and the chart are published to
  `ghcr.io/menlocloud`.

[Unreleased]: https://github.com/menlocloud/stratos/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/menlocloud/stratos/releases/tag/v0.1.0
