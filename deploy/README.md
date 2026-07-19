# Deploying MagiC on Kubernetes

Three supported install paths, in order of preference:

| Path | When to use |
|------|-------------|
| **Helm chart** (`deploy/helm/magic/`) | Production. Templated, supports PDB / HPA / ServiceMonitor / optional Postgres subchart. |
| **Plain manifests** (`deploy/k8s/`) | Air-gapped clusters, GitOps without Helm (ArgoCD kustomize), quick evaluation. |
| **Docker Compose** (repo root `docker-compose.yml`) | Single-host dev / demo. See `docs-site/guide/deployment.md`. |

---

## Option 1 — Helm (recommended)

### Prerequisites

- Kubernetes ≥ 1.24
- Helm ≥ 3.11
- (Optional) cert-manager + an ingress controller for TLS
- (Optional) Prometheus Operator if enabling `metrics.serviceMonitor`

### Install

```bash
# 1. Add the Bitnami repo for the Postgres dependency
helm dependency update deploy/helm/magic/

# 2. Generate an admin API key (32+ chars)
export MAGIC_API_KEY=$(openssl rand -hex 32)

# 3. Install with the bundled Postgres
helm install magic deploy/helm/magic/ \
  --namespace magic --create-namespace \
  --set secrets.apiKey="$MAGIC_API_KEY" \
  --set postgresql.auth.password="$(openssl rand -hex 16)"

# 4. Verify
kubectl -n magic rollout status deploy/magic
kubectl -n magic port-forward svc/magic 8080:80 &
curl -s http://localhost:8080/health
```

### Using an existing Postgres

```bash
helm install magic deploy/helm/magic/ \
  --namespace magic --create-namespace \
  --set postgresql.enabled=false \
  --set secrets.apiKey="$MAGIC_API_KEY" \
  --set secrets.postgresUrl="postgres://user:pass@db.example.com:5432/magic?sslmode=require"
```

### Using an externally-managed Secret (Sealed Secrets, External Secrets, Vault)

```bash
# Create secret out-of-band, then:
helm install magic deploy/helm/magic/ \
  --namespace magic \
  --set secrets.existingSecret=magic-prod-creds \
  ...
```

The referenced Secret MUST contain keys `MAGIC_API_KEY` and (optionally) `MAGIC_POSTGRES_URL`.

### Upgrade

```bash
helm upgrade magic deploy/helm/magic/ -n magic --reuse-values
```

### Rollback

```bash
helm history magic -n magic
helm rollback magic <REVISION> -n magic
```

### Uninstall

```bash
helm uninstall magic -n magic
# Postgres PVC is retained — delete explicitly if desired:
kubectl -n magic delete pvc -l app.kubernetes.io/instance=magic
```

### Common overrides

| Override | Default | Purpose |
|----------|---------|---------|
| `replicaCount` | `2` | Control-plane replicas (Postgres backend only) |
| `image.tag` | `""` (→ appVersion) | Pin a specific image version |
| `ingress.enabled` | `false` | Expose externally |
| `autoscaling.enabled` | `false` | HPA on CPU |
| `metrics.serviceMonitor.enabled` | `false` | Prometheus Operator scraping |
| `networkPolicy.enabled` | `false` | Lock down ingress/egress |
| `podDisruptionBudget.enabled` | `false` | Protect during node drain |

See `deploy/helm/magic/values.yaml` for the full list.

---

## Option 2 — Plain manifests

Good for ArgoCD / Flux without a Helm wrapper.

```bash
# 1. Create a real Secret (do NOT use secret.example.yaml as-is)
kubectl apply -f deploy/k8s/namespace.yaml

kubectl -n magic create secret generic magic \
  --from-literal=MAGIC_API_KEY="$(openssl rand -hex 32)" \
  --from-literal=MAGIC_POSTGRES_URL="postgres://..."

# 2. Apply the rest
kubectl apply -f deploy/k8s/configmap.yaml
kubectl apply -f deploy/k8s/deployment.yaml
kubectl apply -f deploy/k8s/service.yaml
# Edit the host first:
kubectl apply -f deploy/k8s/ingress.yaml

# 3. Verify
kubectl -n magic rollout status deploy/magic
kubectl -n magic port-forward svc/magic 8080:80 &
curl -s http://localhost:8080/health
```

**You must deploy PostgreSQL separately** (e.g. CloudNativePG, Zalando operator, RDS, Neon, Supabase) and reference it via `MAGIC_POSTGRES_URL`. The plain manifests intentionally don't bundle a database.

---

## Option 3 — Docker Compose

For single-host dev / small self-hosted. See [docs-site/guide/deployment.md](../docs-site/guide/deployment.md).

---

## Production checklist

- [ ] `MAGIC_API_KEY` generated fresh per environment, ≥ 32 chars
- [ ] PostgreSQL backend (never in-memory or SQLite at scale)
- [ ] `MAGIC_TRUSTED_PROXY=true` when behind ingress
- [ ] TLS terminated at ingress (cert-manager or equivalent)
- [ ] `ingress.nginx.kubernetes.io/proxy-buffering: "off"` for SSE endpoints
- [ ] ServiceMonitor or scrape annotations pointing at `/metrics`
- [ ] Alerts on `http_requests_total{code=~"5.."}` and `task_failed_total`
- [ ] PodDisruptionBudget + PodAntiAffinity (both set by default in chart)
- [ ] Resource requests/limits tuned against observed load
- [ ] NetworkPolicy restricting ingress to your ingress controller
- [ ] Backups for the Postgres volume

---

## Troubleshooting

```bash
# Pod status
kubectl -n magic describe pod -l app.kubernetes.io/name=magic

# Live logs
kubectl -n magic logs -l app.kubernetes.io/name=magic --tail=100 -f

# Verify env wiring
kubectl -n magic exec deploy/magic -- env | grep ^MAGIC_

# Hit the admin API through a port-forward
kubectl -n magic port-forward svc/magic 8080:80 &
curl -H "Authorization: Bearer $MAGIC_API_KEY" http://localhost:8080/api/v1/workers
```

If `/health` returns 503 and logs mention migrations, the Postgres DSN is likely wrong or the `vector` extension is missing. Use a `pgvector/pgvector`-compatible image.
