// Docs sidebar manifest. Each page maps to src/docs/content/<slug>.md.
// Slugs mirror the public URL: /docs/<slug>.
export type DocPage = { slug: string; title: string }
export type DocSection = { title: string; pages: DocPage[] }

export const docsTitle = "Stratos Docs"

export const sections: DocSection[] = [
  {
    title: "Getting Started",
    pages: [
      { slug: "getting-started/overview", title: "Meet the Stratos Portal" },
      { slug: "getting-started/account-setup", title: "Signing In & Activating Billing" },
      { slug: "getting-started/first-server", title: "Launch Your First Server" },
    ],
  },
  {
    title: "Guides",
    pages: [
      { slug: "guides/servers", title: "Working with Servers" },
      { slug: "guides/networks", title: "Networking" },
      { slug: "guides/volumes", title: "Volumes & Snapshots" },
      { slug: "guides/object-storage", title: "Object Storage Buckets" },
      { slug: "guides/team-members", title: "Teammates & Invitations" },
      { slug: "guides/savings-plans", title: "Savings Plans" },
      { slug: "guides/ai-agents", title: "AI Agent Access (MCP)" },
    ],
  },
  {
    title: "Concepts",
    pages: [
      { slug: "concepts/billing-and-metering", title: "How Metering & Billing Work" },
      { slug: "concepts/provisioning", title: "How Provisioning Works" },
      { slug: "concepts/identity", title: "How Identity Works" },
    ],
  },
  {
    title: "Self-Hosting",
    pages: [
      { slug: "self-hosting/overview", title: "Architecture & Deployment" },
      { slug: "self-hosting/install", title: "Installing on Kubernetes" },
      { slug: "self-hosting/quickstart", title: "MicroK8s Quickstart" },
      { slug: "self-hosting/sso", title: "Single Sign-On" },
      { slug: "self-hosting/custom-ca", title: "Trusting a Custom CA" },
      { slug: "self-hosting/backup", title: "Backup & Recovery" },
      { slug: "self-hosting/openstack-notifications", title: "OpenStack Notifications" },
    ],
  },
]

export const defaultSlug = sections[0].pages[0].slug
