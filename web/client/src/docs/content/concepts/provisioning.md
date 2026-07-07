# How Provisioning Works

Stratos is a self-service and billing layer that sits in front of OpenStack. When you create a server, volume, or network in the portal, Stratos provisions it on the real cloud underneath and then keeps its own view of that resource in step with reality. This page explains that machinery — useful when you're wondering why a status changed, or how the console stays current.

## Projects map onto the cloud

Each Stratos project is backed by a real tenant on an OpenStack region. Stratos manages that tenant using its own **service credentials** — you don't need OpenStack accounts of your own for the platform to work. When you act in the portal, Stratos translates the request into OpenStack API calls made with those credentials, so the resource is created in the tenant that belongs to your project.

(Human sign-in to native OpenStack tooling — Horizon, the CLI — is a separate, optional feature layered on with federated identity; see [Single Sign-On](/docs/self-hosting/sso). It isn't required for provisioning.)

## Keeping the console in sync

The state you see on a resource — a server going ACTIVE, a volume attaching — reflects the real cloud, kept current two ways that back each other up:

- **Periodic sync.** A background job regularly reconciles Stratos's cached view against OpenStack, catching anything that drifted and reconstructing state after any gap. This is the source of truth.
- **Live notifications.** Where the operator has wired it up, OpenStack emits lifecycle events that reach the Stratos API the moment something happens, so changes surface in near real time instead of waiting for the next sync pass. Setting this up is an operator task — see [OpenStack Event Notifications](/docs/self-hosting/openstack-notifications).

Notifications are an optimization, not the authority: if an event is missed or the listener is down, the next sync cycle reconciles the difference. You lose freshness in that window, never correctness. This is also why a change made directly on OpenStack — rebooting or deleting a server from the CLI — still shows up in the portal.

## Activation and lifecycle

Provisioning is gated on an active account: because every resource meters into a bill, the platform won't provision billable resources until billing is activated (see [How Metering and Billing Work](/docs/concepts/billing-and-metering)). On a user's first sign-in Stratos creates their platform record and runs any sign-up or activation logic; from there the create, sync, and delete lifecycle applies to every resource type the platform mirrors.
