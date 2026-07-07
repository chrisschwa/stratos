# Trusting a Custom CA

When your OpenStack endpoints (Keystone, Nova, Neutron, …) or your identity provider present certificates signed by a private CA, the Stratos API has to be told to trust that CA. Otherwise every HTTPS call it makes — Keystone authentication, resource sync, fetching the JWKS from your OIDC issuer to validate tokens — fails TLS verification.

There are really two separate trust problems, each solved in its own place:

1. **Outbound** — `stratos-api` calling Keystone / OpenStack / IdP endpoints. Fixed by mounting the CA into the API pod.
2. **Inbound** — browsers reaching the Stratos ingress. Fixed with a TLS certificate on the ingress (from your private CA or a public one).

## Trusting a private CA in the API

Create a secret in the Stratos namespace holding the CA certificate (PEM format; concatenate multiple CAs into a single file if you have a chain):

```sh
kubectl -n stratos create secret generic ca-root-secret \
  --from-file=ca.crt=/path/to/ca.crt
```

If the certificate came from cert-manager inside the cluster, a suitable secret with `ca.crt` may already exist — reuse it.

Then mount it into the API container and point the `SSL_TRUST_CA_CERTS` environment variable at it, using the chart's extension hooks:

```yaml
api:
  extraEnvVars:
    - name: SSL_TRUST_CA_CERTS
      value: /opt/stratos/ca.crt
  extraVolumes:
    - name: ca-cert-volume
      secret:
        secretName: ca-root-secret
        items:
          - key: ca.crt
            path: ca.crt
  extraVolumeMounts:
    - name: ca-cert-volume
      mountPath: /opt/stratos/ca.crt
      subPath: ca.crt
      readOnly: true
```

Apply with `helm upgrade … -f values.yaml`. On startup the API adds the listed certificates to its trust store on top of the system defaults, so publicly-signed endpoints keep working right alongside your private ones.

To verify, check the API log after a restart — Keystone connectivity errors of the `x509: certificate signed by unknown authority` variety vanish once the CA is trusted. From inside the pod:

```sh
kubectl -n stratos exec deploy/stratos-api -- \
  sh -c 'ls -l $SSL_TRUST_CA_CERTS'
```

## TLS on the ingress with a private CA

If your users' browsers should also see certificates from the private CA — common in air-gapped or lab setups:

- With **cert-manager**, create a `ClusterIssuer` of type `ca` backed by your CA key pair, reference it in `global.ingress.annotations` (`cert-manager.io/cluster-issuer: <issuer>`), and keep `global.ingress.tls: true`.
- Without cert-manager, create the TLS secret yourself and reference it via `global.ingress.secrets`.

Remember that clients — browsers, but also any machine running the OpenStack CLI against a federated Keystone — need the CA in their own trust stores.

## Where a custom CA usually comes up

- Keystone / OpenStack API endpoints on a private network with internally-issued certs.
- An external Keycloak or OIDC provider on an internal domain — without trust, JWT validation fails because the JWKS endpoint can't be fetched.
- Internal SMTP or webhook targets reached over HTTPS.
