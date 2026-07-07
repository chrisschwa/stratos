# OpenStack Event Notifications

Stratos syncs resources from OpenStack on a schedule, but polling alone means the customer dashboard can trail reality by minutes. Turning on OpenStack notifications lets Stratos react the instant something happens — a server reaching ACTIVE, a volume attaching, a port changing — so customers see state changes almost immediately and usage tracking picks up lifecycle events as they occur. For the bigger picture of how this feeds the platform, see [How Provisioning Works](/docs/concepts/provisioning).

## How the pieces fit

```
OpenStack services ──(oslo.messaging notifications)──▶ OpenStack RabbitMQ
                                                            │
                                              notification listener / forwarder
                                                            │  HTTP webhook
                                                            ▼
        https://cloud.example.com/api/v1/notifications/{externalServiceId}/{region}
```

OpenStack services publish lifecycle events onto their control-plane RabbitMQ through oslo.messaging (`notifications.*` topics). A listener sitting next to that RabbitMQ consumes the relevant exchanges and forwards each event to the Stratos API as an HTTP POST.

## The webhook endpoint

```
POST /api/v1/notifications/{externalServiceId}/{region}
```

- `{externalServiceId}` — the ID of the OpenStack provider/service as registered in the Stratos admin console (shown on the provider's detail page).
- `{region}` — the OpenStack region name the events originate from, e.g. `RegionOne`.

Example: `https://cloud.example.com/api/v1/notifications/647724f1ec3c45627b0d3272/RegionOne`

There's one endpoint per (provider, region) pair — run multiple regions and each region's listener posts to its own URL.

You don't have to build the URL by hand: open the provider in the admin console and its **Connection** tab lists the ready-to-copy **Notifier URI** for every configured region (and is where you set the shared secret below).

## Securing the endpoint

The webhook can't use a bearer token — OpenStack can't present one — so it's protected by a **per-provider shared secret**. Set a secret on the cloud provider in the admin console (its **Connection** tab; stored encrypted as `secret.notificationSecret`): Stratos then rejects any request that doesn't carry that exact value in the `X-Stratos-Notification-Secret` header, so configure the OpenStack notifier (or the proxy in front of it) to send it.

The webhook **fails closed** — until a provider has a secret set, its notification endpoint rejects every request. So a provider you haven't configured a secret for simply won't ingest notifications (the periodic sync still keeps its cache correct); it can never be forged by an anonymous caller.

## Turning notifications on in OpenStack

Each service you care about needs oslo.messaging notifications enabled in its config file (`nova.conf`, `neutron.conf`, `cinder.conf`, …):

```ini
[oslo_messaging_notifications]
transport_url = rabbit://openstack:<password>@<rabbitmq-host>:5672/
driver = messagingv2
topics = notifications
```

**Kolla Ansible shortcut:** setting `enable_ceilometer: "yes"` in `globals.yml` flips notifications on across every service at once — it's Ceilometer's presence that makes Kolla template the `[oslo_messaging_notifications]` blocks. You don't have to use Ceilometer's own data for Stratos.

## Which events matter

The listener subscribes to the notification exchanges of the services whose state Stratos mirrors:

| Service | Exchange | Events of interest |
|---------|----------|--------------------|
| Nova | `nova` | Instance lifecycle: create/delete, power state, resize, reboot |
| Neutron | `neutron` | Networks, ports, floating IPs, routers created/updated/deleted |
| Cinder | `openstack` | Volume create/delete/attach/detach, snapshots |
| Heat | `heat` | Stack create/update/delete progress |

The topic pattern is `notifications.*` on each exchange. Events for projects Stratos doesn't manage are dropped on receipt, so over-subscribing does no harm.

## Deployment notes

- Run the forwarder close to the OpenStack RabbitMQ (same control-plane network); it only needs outbound HTTPS to the Stratos API.
- The Stratos API has to be reachable from the OpenStack control plane on the webhook URL. If OpenStack's egress goes through a proxy, or a private CA terminates TLS, see [Trusting a Custom CA](/docs/self-hosting/custom-ca) for the trust setup on the Stratos side and configure the forwarder's trust to match.
- Notifications are an optimization, not a source of truth: the periodic sync keeps running and reconciles anything a lost message missed. If the listener is down for a while, state converges again on the next sync cycle — you lose freshness, not correctness.

## Verifying it works

1. Create or reboot a server in OpenStack directly (CLI or Horizon) for a project Stratos manages.
2. Watch the customer portal — the server's status should update within seconds, without waiting for the next sync interval.
3. In the Stratos API logs, incoming `os-notification` requests confirm delivery end to end.
