# MicroK8s Quickstart

The quickest way to try Stratos is a single-node MicroK8s cluster on one VM. It gives you a fully working platform — API, portal, admin console, Keycloak, PostgreSQL — on a machine you can discard afterward.

## What you'll need

- An Ubuntu LTS VM with at least 4 vCPU / 8 GB RAM and disk headroom for the bundled databases.
- A public IP and a DNS record (e.g. `cloud.example.com`) pointing at it — Let's Encrypt has to reach the VM to issue certificates.

## 1. Install MicroK8s

```sh
sudo snap install microk8s --classic
microk8s status --wait-ready
```

Working as a non-root user? Add yourself to the group and re-login:

```sh
sudo usermod -a -G microk8s $USER
sudo chown -f -R $USER ~/.kube
su - $USER
```

## 2. Turn on the addons

```sh
microk8s enable dns
microk8s enable ingress
microk8s enable hostpath-storage
microk8s enable cert-manager
```

That gives the chart everything it expects: cluster DNS, an NGINX ingress controller, a default StorageClass on the local disk, and cert-manager for TLS.

Create a Let's Encrypt ClusterIssuer:

```sh
microk8s kubectl apply -f - <<'EOF'
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: you@example.com
    privateKeySecretRef:
      name: letsencrypt-account-key
    solvers:
      - http01:
          ingress:
            class: public
EOF
```

## 3. Install Stratos

Write a minimal `values.yaml` — note MicroK8s's ingress class is `public`:

```yaml
global:
  ingress:
    enabled: true
    hostname: "cloud.example.com"
    ingressClassName: "public"
    annotations:
      cert-manager.io/cluster-issuer: letsencrypt
    tls: true
```

Then install straight from the OCI registry:

```sh
microk8s helm upgrade --install stratos oci://ghcr.io/menlocloud/charts/stratos \
  --namespace stratos --create-namespace \
  -f values.yaml
```

First boot takes a few minutes: PostgreSQL, RabbitMQ and Keycloak all start, the Keycloak realms get provisioned, and the API waits — via its `wait-for-db` init container — until PostgreSQL answers.

```sh
microk8s kubectl -n stratos get pods
```

## 4. First login

- Customer portal: `https://cloud.example.com/`
- Admin console: `https://cloud.example.com/stratos_admin`
- Keycloak: `https://auth.cloud.example.com/`

Before anything else, save the encryption key offline:

```sh
microk8s kubectl -n stratos get secret stratos-api \
  -o jsonpath='{.data.encryption-key}' | base64 -d
```

From here, move on to [Installing on Kubernetes](/docs/self-hosting/install) for the production topics — external databases, scaling, [backups](/docs/self-hosting/backup). The values file and commands are the same; only the cluster differs.
