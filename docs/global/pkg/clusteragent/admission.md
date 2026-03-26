> **TL;DR:** Implements the Kubernetes Admission Controller for the Datadog Cluster Agent, intercepting pod-creation requests to automatically inject Datadog configuration, APM libraries, sidecars, and security instrumentation.

# pkg/clusteragent/admission

## Purpose

Implements the Kubernetes **Admission Controller** managed by the Datadog Cluster Agent. It intercepts Kubernetes API server requests to validate or mutate objects (primarily Pods) before they are persisted. This enables automatic injection of Datadog configuration — environment variables, APM libraries, sidecars — without requiring users to instrument each workload manually.

The entire package tree is gated by the `kubeapiserver` build tag.

## Package Structure

```
admission/
├── start.go                  # Entry point: StartControllers
├── doc.go                    # Package-level documentation and webhook authoring guide
├── api_discovery.go          # Detects Kubernetes API capabilities (v1 vs v1beta1, MatchConditions, namespace selectors)
├── util.go                   # Shared HTTP handler utilities
├── status.go                 # Status reporting
├── common/                   # Shared constants, label selectors, library config helpers
├── controllers/
│   ├── secret/               # Manages the TLS Secret used by the webhook server
│   └── webhook/              # Reconciles MutatingWebhookConfiguration / ValidatingWebhookConfiguration objects
├── mutate/                   # Mutating webhook implementations
│   ├── config/               # Injects DD_* env vars (agent host, entity ID, …)
│   ├── autoinstrumentation/  # Injects APM language libraries (Java, Python, Node.js, …)
│   ├── tagsfromlabels/       # Propagates Kubernetes labels as Datadog tags
│   ├── agent_sidecar/        # Injects a Datadog Agent sidecar container
│   ├── appsec/               # AppSec instrumentation injection
│   ├── cwsinstrumentation/   # CWS (Cloud Workload Security) instrumentation
│   └── autoscaling/          # Patch pods for workload autoscaling recommendations
├── validate/                 # Validating webhook implementations
│   └── kubernetesadmissionevents/  # Emits Kubernetes audit events
└── patch/                    # Remote-Configuration-driven library patching (APM)
```

## Key Elements

### Key interfaces

### Startup

`StartControllers(ctx ControllerContext, wmeta, pa, datadogConfig)` is the single entry point. It:
1. Starts a **secret controller** (`controllers/secret`) that manages the self-signed TLS certificate stored in a Kubernetes Secret, rotating it before expiry.
2. Starts a **webhook controller** (`controllers/webhook`) that creates/reconciles the `MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration` objects in the cluster, using the certificate from step 1.
3. Detects cluster capabilities (namespace selector support, `MatchConditions`, v1 vs v1beta1 API) via `api_discovery.go`.
4. Optionally starts a readiness probe (`admission_controller.probe.enabled`).

Controlled via config key `admission_controller.enabled` (disabled → no-op).

### Webhook interface

Every webhook must implement the `Webhook` interface (defined in `controllers/webhook`):

| Method | Purpose |
|--------|---------|
| `Name() string` | Identifier used in telemetry tags. |
| `WebhookType()` | `"mutating"` or `"validating"`. |
| `IsEnabled() bool` | Whether the webhook is active (read from config). |
| `Endpoint() string` | HTTP path the Cluster Agent serves. |
| `Resources() []string` | Kubernetes resource types (e.g. `["pods"]`). |
| `Operations() []admiv1.OperationType` | e.g. `[CREATE, UPDATE]`. |
| `LabelSelectors(...)` | Label-based pre-filtering to reduce webhook traffic. |
| `MatchConditions(...)` | CEL expressions for fine-grained filtering (requires Kubernetes ≥ 1.28). |
| `WebhookFunc(...)` | Handler that returns the admission response / JSON patch. |

### Mutating webhooks (selected)

| Sub-package | Webhook | What it does |
|-------------|---------|--------------|
| `mutate/config` | `DatadogConfig` | Injects `DD_AGENT_HOST`, `DD_ENTITY_ID`, `DD_EXTERNAL_ENV` env vars into pods. |
| `mutate/autoinstrumentation` | `AutoInstrumentation` | Detects pod language (via annotations or admission labels), downloads the matching APM init-container image, and mounts the library. Supports Java, Python, Node.js, .NET, Ruby. |
| `mutate/tagsfromlabels` | `TagsFromLabels` | Copies Kubernetes labels (`app`, `version`, …) as `DD_ENV` / `DD_SERVICE` / `DD_VERSION` env vars. |
| `mutate/agent_sidecar` | `AgentSidecar` | Injects a full Datadog Agent container into the pod. |
| `mutate/autoscaling` | `AutoscalingWebhook` | Applies CPU/memory resource patches recommended by the workload autoscaler. |

### Common helpers (`common/`)

- `common/const.go` — well-known annotation/label keys (`admission.datadoghq.com/enabled`, etc.).
- `common/label_selectors.go` — default `NamespaceSelector` and `ObjectSelector` builders.
- `common/lib_config.go` — library injection config utilities shared across APM webhooks.
- `common/global.go` — cluster-wide settings (namespace, certificate secret name).

### Metrics (`metrics/`)

All webhooks share common telemetry counters defined here (`webhooks_received`, `webhooks_response_duration`, etc.) tagged by webhook name and response code.

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`pkg/clusteragent`](clusteragent.md) | The admission package is a sub-system of the Cluster Agent. `admission.StartControllers` is called from the DCA startup sequence alongside `clusterchecks.NewHandler` and `controllers.StartControllers`. |
| [`pkg/util/kubernetes/apiserver`](../util/kubernetes-apiserver.md) | Provides the `APIClient` used by `ControllerContext`: `CertificateSecretInformerFactory` and `WebhookConfigInformerFactory` are sourced directly from `APIClient` fields. The TLS secret controller and webhook configuration controller both rely on the k8s typed client (`APIClient.Cl`). |
| [`pkg/languagedetection`](../languagedetection.md) | The `mutate/autoinstrumentation` webhook reads `internal.dd.datadoghq.com/<container>.detected_langs` annotations (written by `pkg/clusteragent/languagedetection`) to decide which APM library to inject. `pkg/languagedetection/util.GetNamespacedBaseOwnerReference` is used to resolve the owning Deployment from a pod. |
| [`comp/core/workloadmeta`](../../comp/core/workloadmeta.md) | Passed to `StartControllers` as `wmeta workloadmeta.Component`. The `mutate/autoscaling` webhook and language-detection helpers query workloadmeta to resolve pod-owner relationships and language annotation state. |
| [`pkg/clusteragent/autoscaling`](autoscaling.md) | The `mutate/autoscaling` webhook applies CPU/memory recommendations produced by `autoscaling/workload.PodPatcher`. The two packages are decoupled via the `PodPatcher` interface so the autoscaling package can be tested independently. |

---

## Usage

The admission controller is started from `cmd/cluster-agent` via the component system. The call chain is:

```
cmd/cluster-agent → comp/core/... → StartControllers()
                                      ├── secret.NewController().Run()   // TLS cert lifecycle
                                      └── webhook.NewController().Run()  // reconciles MutatingWebhookConfiguration
                                            └── registers HTTP handlers for each Webhook
                                                  (served on the DCA HTTPS port, default :8443)
```

The Kubernetes API server is configured to call the DCA HTTPS endpoint (e.g. `https://<dca-service>:8443/datadog-webhook`) for every matching pod-creation request. The DCA must have a valid TLS certificate (managed by the secret controller) whose CA bundle is embedded in the `MutatingWebhookConfiguration`.

### Namespace and object filtering

Label selectors limit which pods trigger each webhook, reducing unnecessary traffic:

- The default `NamespaceSelector` requires `admission.datadoghq.com/enabled: "true"` on the namespace (or `"false"` to opt-out at namespace level).
- `ObjectSelector` on individual pods can override the namespace default via `admission.datadoghq.com/enabled` label on the pod itself.
- From Kubernetes 1.28+, `MatchConditions` (CEL expressions) provide further filtering without a round-trip to the DCA.

`api_discovery.go` probes the cluster at startup to detect whether `MatchConditions` and namespace selector are supported, and configures the webhooks accordingly.

### TLS certificate rotation

The `controllers/secret` controller creates and auto-renews the self-signed certificate stored in the `datadog-cluster-agent-admission-controller` Secret (default name). It re-issues the certificate when the remaining validity drops below the configured threshold (`admission_controller.certificate.expiry_threshold`). The new CA bundle is propagated to the `MutatingWebhookConfiguration` by the webhook controller.

### Adding a new webhook

1. Create a sub-package under `mutate/` or `validate/`.
2. Implement the `Webhook` interface.
3. Register the new webhook in `controllers/webhook/controller_base.go` inside `generateWebhooks()`.
4. Add config keys under `admission_controller.<webhook_name>`.
5. Add telemetry metrics if the webhook has unique counters.

Keep webhooks independent — cross-webhook dependencies should be avoided. If ordering matters, document it in the webhook's `doc.go`.
