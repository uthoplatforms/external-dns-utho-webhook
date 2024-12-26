# ExternalDNS - Utho Webhook

## Installation Guide

### Step 1: Update Your Utho API Key

Edit the `secret.yaml` file to include your Utho API key:

```yaml
# secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: utho-credentials
  namespace: external-dns
type: Opaque
data:
  api-key: <UTHO_API_KEY>

```

### Step 2: Apply the Utho Secret

Run the following command to apply the secret:

```shell
kubectl apply -f secret.yaml
```

---

## Installing ExternalDNS with the Bitnami Chart

### Add the Bitnami Repository

Skip this step if you have already added the Bitnami repository:

```shell
helm repo add bitnami https://charts.bitnami.com/bitnami
```

### Install ExternalDNS with Utho Webhook

Use the example values file `external-dns-utho-values.yaml` to install ExternalDNS with Helm:

```shell
helm install external-dns-utho bitnami/external-dns -f external-dns-utho-values.yaml -n external-dns
```

### Verify Installation

Ensure that the deployment is successful by checking the Helm release and pod status:

```shell
helm list -n external-dns
```

```shell
kubectl get pods -n external-dns
```

---

## Environment Variables

### Mandatory Variables

| Variable      | Description            | Default   |
|---------------|------------------------|-----------|
| `UTHO_API_KEY`| Utho API token         | Mandatory |

### Optional Variables

| Variable          | Description                      | Default     |
|--------------------|----------------------------------|-------------|
| `DRY_RUN`         | Prevents changes from being applied | `false`     |
| `WEBHOOK_HOST`    | Webhook hostname or IP address   | `localhost` |
| `WEBHOOK_PORT`    | Webhook port                     | `8888`      |
| `HEALTH_HOST`     | Liveness and readiness hostname  | `0.0.0.0`   |
| `HEALTH_PORT`     | Liveness and readiness port      | `8080`      |
| `READ_TIMEOUT`    | Server read timeout in ms        | `60000`     |
| `WRITE_TIMEOUT`   | Server write timeout in ms       | `60000`     |

### Domain Filtering

| Environment Variable           | Description                 |
|--------------------------------|-----------------------------|
| `DOMAIN_FILTER`                | Domains to include          |
| `EXCLUDE_DOMAIN_FILTER`        | Domains to exclude          |
| `REGEXP_DOMAIN_FILTER`         | Regex for included domains  |
| `REGEXP_DOMAIN_FILTER_EXCLUSION` | Regex for excluded domains |

**Note:**  
- If `REGEXP_DOMAIN_FILTER` is set, it takes precedence.  
- Without it, `DOMAIN_FILTER` and `EXCLUDE_DOMAIN_FILTER` are used.

---

## Tweaking the Configuration

Here are a few tips for customization:

- **Port Conflicts:** Ensure `WEBHOOK_HOST` and `HEALTH_HOST` are not set to the same address unless different ports are used.
- **Record Deletion:** By default, ExternalDNS uses the `upsert-only` policy, which doesn't delete records. If you want deletions to occur, set the policy to `sync` by adding the following to `external-dns-utho-values.yaml`:

  ```yaml
  policy: sync
  ```
