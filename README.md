# Authentication-Encryption-Proxy
A jwt based authentication and payload encryption standalone applications for API Gateway.
# The architecture Diagram :
╔═══════════════════════════════════════════════════════════════════════════════════════════╗
║         JWT AUTH · PAYLOAD ENCRYPTION · ISTIO · Jenkins CI/CD · Monitoring                ║
╚═══════════════════════════════════════════════════════════════════════════════════════════╝

 ┌─────────────────────────────────────────────────────────────────────────────────────────┐
 │  JENKINS PIPELINE  (RBAC gated · runs for Auth App & Redirect App independently)        │
 │                                                                                         │
 │  Git Push                                                                               │
 │     │                                                                                   │
 │     ▼                                                                                   │ 
 │  [Code Build]──►[Trivy Scan]──►[Docker Build]──►[Push Artifact Registry]──►[K8s Deploy] │
 │   go build       CVE check      multi-stage       versioned image tag        kubectl    │
 │   go test        CRIT=fail       scratch base      Artifact Registry         apply      │
 └─────────────────────────────────────────────┬───────────────────────────────────────────┘
                                               │ deploy(k8s cluster)
                                               ▼
 ┌─────────────────────────────────────────────────────────────────────────────────────────┐
 │                              REQUEST FLOW                                               │
 │             API Request--►                                                              │
 │  Browser ───────────────────────────────────────────────► Redirect App IP               │
 │                      (Istio AuthPolicy intercepts)                  │                   │
 │                                                                     └────► Auth App     │
 │                                                                            │  JWT valid │
 │                                                                            │  JTI check │
 │                                                                            │  ALLOW ↓   │
 │                                                                            ▼            │
 │                                                                       Redirect App      │
 │                                                                            │  AES Decr  │
 │                                                                   - - - - -▼- - - - -   │
 │                                                                       Istio VS          │
 │                                                                            │  VS match  │
 │                                                                   - - - - -▼- - - - -   │
 │                                                                        Backend          │
 │                                                                            │  logic     │
 │                              RESPONSE FLOW                                 │            │
 │                                                                            ▼            │
 │  Browser ◄──────────────── Redirect App (AES Encrypt) ◄─ Istio ◄─ - - - ---┘            │
 └────────────────────────────────────────────────────────────────────────────────────────-┘
                                               │
                                               ▼
 ┌─────────────────────────────────────────────────────────────────────────────────────────┐
 │  MONITORING                                                                             │
 │                                                                                         │
 │  Auth App /metrics ──┐                                                                  │
 │  Redirect App /metrics ─┤                                                               │
 │  Istio Gateway/metrics ─┴──► Prometheus ------──----► Grafana                           │
 │                                · memory · CPU · HPA    · dashboards · alerts            │
 │                                · timeseries data       · thresholds                     │
 └─────────────────────────────────────────────────────────────────────────────────────────┘

  HPA : Auth App · Redirect App · Istio Gateway    
  Security:  Istio AuthPolicy · JWT signature · JTI anti-replay · AES encrypt ·RBAC
