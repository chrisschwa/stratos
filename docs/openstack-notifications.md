# OpenStack Notifications

How to feed OpenStack resource-lifecycle events into Stratos for near-real-time
dashboard updates.

This document is for **operators** wiring a cloud into Stratos. For how the
cache is modelled and rated, see [cloud-integration.md](cloud-integration.md).

---

## What this is, and why it's optional

Stratos keeps a PostgreSQL cache of every customer's cloud resources and bills
against that cache (never straight off live OpenStack). Two paths keep the cache
honest:

- **Sync** — a periodic reconcile that lists live OpenStack and makes the cache
  match. Authoritative, but only as fresh as its interval.
- **Notifications** — this feature. A webhook fed by oslo.messaging events that
  applies single-resource changes within seconds of them happening.

Notifications are **optional**: with them off, the periodic sync still keeps the
cache correct — just not instantly. Turn them on when you want a customer's
dashboard to reflect a create/delete/resize almost immediately instead of
waiting for the next sync.

---

## Architecture

```
OpenStack services  ──emit──▶  RabbitMQ  ──consumed by──▶  notifier bridge  ──HTTP POST──▶  Stratos webhook
(nova, neutron, …)             (oslo topic)                 (you run this)                   /api/v1/notifications/{id}/{region}
```

**Stratos ships only the receiver** — the webhook endpoint. It does **not** dial
into your RabbitMQ. So yes: you add a **separate component** (a small "notifier"
sidecar/deployment) that connects to *your* RabbitMQ, consumes the OpenStack
notification stream, and POSTs each event to the Stratos webhook. That bridge is
the manifest you have to add.

Why a bridge instead of Stratos consuming RabbitMQ directly: the message broker
belongs to the cloud, not to Stratos. Keeping Stratos a pure HTTP receiver means
it needs no AMQP credentials, no network path into the cloud's control plane, and
one provider's broker outage can't stall another's ingestion.

---

## Step 1 — Make OpenStack emit notifications

Configure oslo.messaging notifications on the OpenStack services you care about
(nova, neutron, cinder, glance, designate, heat, magnum, manila). In each
service's config:

```ini
[oslo_messaging_notifications]
transport_url = rabbit://openstack:<password>@<rabbitmq-host>:5672//
driver = messagingv2
topics = notifications
```

**Kolla Ansible:** enabling ceilometer turns notifications on across the stack —
in `globals.yml`:

```yaml
enable_ceilometer: "yes"
```

After this, OpenStack publishes lifecycle events (e.g. `compute.instance.create.end`,
`volume.delete.end`) to the `notifications` topic on RabbitMQ.

---

## Step 2 — Get the webhook URL and set the secret

In the admin console: **Settings → Cloud providers → [provider]**, the
**OpenStack Notifier URI** section shows one URL per configured region:

```
https://cloud.<your-domain>/api/v1/notifications/<serviceId>/<region>
```

- `<serviceId>` is the cloud provider's external-service id (filled in for you).
- `<region>` is each configured region (e.g. `RegionOne`).

Set a **Notification secret** in the same section. The webhook is
**fail-closed**: until a secret is set it rejects every request, and once set it
accepts a request only if the caller sends that secret in the
`X-Stratos-Notification-Secret` header. The secret is stored encrypted and never
returned on reads.

> The endpoint is unauthenticated in the bearer-token sense (the notifier can't
> mint an OAuth token), so this shared secret is the **only** thing standing
> between the internet and forged cache mutations. Treat it like a password and
> send it over TLS only.

---

## Step 3 — Run the notifier bridge

The bridge must:

1. Connect to your RabbitMQ and subscribe to the OpenStack notification exchanges
   (`nova`, `neutron`, `cinder`, `glance`, `heat`, `magnum`, `manila`,
   `designate`) on the `notifications.*` topic, via its own durable queue.
2. For each message, HTTP `POST` the **raw oslo.messaging JSON body** to the
   region's Notifier URI with header
   `X-Stratos-Notification-Secret: <the secret from Step 2>`.
3. Ignore the response (Stratos always returns `200` — see *Delivery semantics*).

> **Note:** Stratos does not yet publish a prebuilt notifier image. The contract
> above is small (an AMQP consumer that reposts the body with one header); run
> any compatible RabbitMQ→HTTP forwarder configured to it, or ask us to ship a
> `stratos-notifier`.

Example Kubernetes Deployment (one bridge per cloud; fill in the placeholders):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: stratos-notifier-regionone
  namespace: menlo-cloud
spec:
  replicas: 1
  selector:
    matchLabels: { app: stratos-notifier-regionone }
  template:
    metadata:
      labels: { app: stratos-notifier-regionone }
    spec:
      containers:
        - name: notifier
          image: <your-notifier-image>
          env:
            # source: your OpenStack RabbitMQ
            - name: RABBITMQ_ADDRESSES
              value: "rabbitmq1:5672,rabbitmq2:5672,rabbitmq3:5672"
            - name: RABBITMQ_USERNAME
              value: "openstack"
            - name: RABBITMQ_PASSWORD
              valueFrom: { secretKeyRef: { name: notifier-rabbit, key: password } }
            - name: RABBITMQ_TOPIC
              value: "notifications.*"
            - name: RABBITMQ_QUEUE
              value: "stratos-notifier"
            - name: RABBITMQ_EXCHANGES
              value: "nova,neutron,cinder,glance,heat,magnum,manila,designate"
            # sink: the Stratos webhook (from Step 2) + its secret (from Step 2)
            - name: TARGET_URL
              value: "https://cloud.<your-domain>/api/v1/notifications/<serviceId>/RegionOne"
            - name: TARGET_SECRET
              valueFrom: { secretKeyRef: { name: notifier-rabbit, key: stratos-secret } }
```

Run **one bridge per (cloud, region)** — the Notifier URI is region-scoped.

---

## What Stratos does with each event

Stratos routes by the first dot-segment of `event_type`:

| event_type prefix | OpenStack service | cache resource |
|---|---|---|
| `compute` | nova | server (or bare-metal server) |
| `volume` | cinder | volume |
| `image` | glance | image |
| `network` / `subnet` / `port` / `router` / `floatingip` / `security_group` | neutron | the matching network resource |
| `dns` | designate | DNS zone |
| `orchestration` | heat | stack |
| `magnum` | magnum | cluster |
| `share` | manila | file share |

- A **delete** event (`*.delete.*`) removes the resource from the cache.
- Any other event re-fetches that one resource live from OpenStack and upserts it
  (so the cache reflects the post-change state, not just the event payload).
- An unmapped `event_type` is silently skipped.

Applied changes also push an SSE update to any open dashboard for that project, so
the UI refreshes without a reload.

---

## Delivery semantics & security

- **Always 200.** The webhook is fire-and-forget: even on a processing error it
  returns `200` and logs the failure, so a transient error can't make OpenStack
  (or the bridge) retry-storm. The periodic sync is the safety net that repairs
  anything a dropped event missed.
- **Fail-closed auth.** No secret configured, or a wrong/missing header →
  `401`, before any cache mutation. The comparison is constant-time.
- **Per-provider isolation.** Each provider has its own secret; a secret for one
  cloud can't post events against another.
- **Malformed body → 400.** The only case that isn't swallowed.

---

## Verify

1. Set the secret (Step 2), deploy the bridge (Step 3).
2. Create or delete a resource in OpenStack (e.g. boot an instance).
3. The customer's dashboard should reflect it within seconds — no manual refresh.
4. If nothing happens: check the bridge logs for AMQP connect + POST status,
   confirm the URL region matches, and confirm the `X-Stratos-Notification-Secret`
   header matches the saved secret (a mismatch is a silent `401` at Stratos).
```
