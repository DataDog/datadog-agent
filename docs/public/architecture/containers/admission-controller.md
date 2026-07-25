# Admission controller

-----

The Datadog admission controller is a Kubernetes [dynamic admission webhook](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/) served by the [Cluster Agent](cluster-agent.md) (DCA). When enabled, the Kubernetes apiserver calls back into the DCA on every matching pod creation (and a few other operations), and the DCA mutates the object before it is persisted: injecting Datadog environment variables, APM tracer libraries, an agent sidecar on Fargate, autoscaling resource patches, or CWS instrumentation. It is the mechanism that lets Datadog configure application pods without users touching their manifests.

The code lives in [`pkg/clusteragent/admission`](<<<SRC>>>/pkg/clusteragent/admission). It has three moving parts: a **secret controller** that maintains the webhook's TLS certificate, a **webhook controller** that reconciles the `MutatingWebhookConfiguration`/`ValidatingWebhookConfiguration` objects in the cluster, and an **HTTPS server** that answers `AdmissionReview` requests. The first two are leader-only reconcilers; the server runs on every DCA replica.

## Key packages

| Path | Purpose |
|---|---|
| [`pkg/clusteragent/admission/start.go`](<<<SRC>>>/pkg/clusteragent/admission/start.go) | `StartControllers`: wires the secret and webhook controllers, API discovery gating, probe |
| [`pkg/clusteragent/admission/controllers/secret/controller.go`](<<<SRC>>>/pkg/clusteragent/admission/controllers/secret/controller.go) | Creates/rotates the self-signed webhook certificate in a Secret |
| [`pkg/clusteragent/admission/controllers/webhook/controller_base.go`](<<<SRC>>>/pkg/clusteragent/admission/controllers/webhook/controller_base.go) | Shared reconciler logic and `generateWebhooks` (the ordered catalog) |
| [`controller_v1.go`](<<<SRC>>>/pkg/clusteragent/admission/controllers/webhook/controller_v1.go), [`controller_v1beta1.go`](<<<SRC>>>/pkg/clusteragent/admission/controllers/webhook/controller_v1beta1.go) | API-version-specific reconciliation of the webhook configuration objects |
| [`pkg/clusteragent/admission/api_discovery.go`](<<<SRC>>>/pkg/clusteragent/admission/api_discovery.go) | Discovery-based gating: `UseAdmissionV1`, `supportsMatchConditions`, namespace-selector support |
| [`cmd/cluster-agent/admission/server.go`](<<<SRC>>>/cmd/cluster-agent/admission/server.go) | The HTTPS webhook server: decodes `AdmissionReview` v1/v1beta1, dispatches per endpoint |
| [`pkg/clusteragent/admission/mutate`](<<<SRC>>>/pkg/clusteragent/admission/mutate) | All mutating webhook implementations (one package each) |
| [`pkg/clusteragent/admission/validate`](<<<SRC>>>/pkg/clusteragent/admission/validate) | Validating webhooks: admission events, `DatadogInstrumentation` validation |
| [`pkg/clusteragent/admission/patch`](<<<SRC>>>/pkg/clusteragent/admission/patch) | Legacy remote-config patcher: labels Deployments for APM single-step instrumentation (SSI) from the `APM_TRACING` RC product |
| [`pkg/clusteragent/admission/probe/probe.go`](<<<SRC>>>/pkg/clusteragent/admission/probe/probe.go) | Self-test that verifies the webhook actually intercepts requests |
| [`pkg/clusteragent/admission/common`](<<<SRC>>>/pkg/clusteragent/admission/common) | Shared constants, label selectors, SSI lib-config types |

## Control plane: two reconcilers, one server

`StartControllers` in [`start.go`](<<<SRC>>>/pkg/clusteragent/admission/start.go) subscribes twice to the DCA's leadership notifications — once per controller — so each reacts independently to leadership changes. Both controllers run on every replica but no-op unless `isLeaderFunc()` returns true. The webhook HTTPS server, by contrast, runs unconditionally: mutation logic is stateless, the Kubernetes Service in front of the DCA load-balances admission requests across all replicas, and followers answer them just as well as the leader.

### Secret controller

The `secret.Controller` ([`controllers/secret`](<<<SRC>>>/pkg/clusteragent/admission/controllers/secret)) owns the webhook's TLS identity: a self-signed certificate whose DNS names derive from `admission_controller.service_name`, stored in the Secret named by `admission_controller.certificate.secret_name` (default `webhook-certificate`). Validity is `admission_controller.certificate.validity_bound` (one year), and the certificate is re-issued `expiration_threshold` (30 days) before expiry. The server hot-loads the certificate from a Secret informer on every TLS handshake, so rotation needs no restart.

### Webhook controller

The `webhook.Controller` ([`controller_base.go`](<<<SRC>>>/pkg/clusteragent/admission/controllers/webhook/controller_base.go)) reconciles exactly one `MutatingWebhookConfiguration` and one `ValidatingWebhookConfiguration`, both named `admission_controller.webhook_name` (default `datadog-webhook`), each embedding the CA bundle from the Secret and one webhook entry per enabled implementation. Reconciliation re-triggers on leadership change, Secret change, or any edit to the webhook objects themselves — manual tampering is reverted within seconds.

API discovery ([`api_discovery.go`](<<<SRC>>>/pkg/clusteragent/admission/api_discovery.go)) picks the code path: `admissionregistration.k8s.io/v1` versus `v1beta1` (`controller_v1.go` / `controller_v1beta1.go`), object selectors versus namespace selectors on older Kubernetes (`useNamespaceSelector`), and CEL `MatchConditions` where supported — the latter let webhooks skip entire request classes apiserver-side instead of paying the network round trip.

## The webhook catalog

`generateWebhooks` in [`controller_base.go`](<<<SRC>>>/pkg/clusteragent/admission/controllers/webhook/controller_base.go) returns the ordered list. Order is execution order, and it matters: the comment in the source pins `agent_sidecar` after `config` because the APM socket volume mount that `config` adds does not work on Fargate, and the sidecar webhook must strip it; `auto_instrumentation` likewise runs after `config`.

| Webhook | Kind | Package | What it does | Enabled by |
|---|---|---|---|---|
| `kubernetesadmissionevents` | validating | [`validate/kubernetesadmissionevents`](<<<SRC>>>/pkg/clusteragent/admission/validate/kubernetesadmissionevents) | Emits Datadog events for admission activity (deploy tracking) | `admission_controller.kubernetes_admission_events.enabled` |
| `datadoginstrumentation` | validating | [`validate/datadoginstrumentation`](<<<SRC>>>/pkg/clusteragent/admission/validate/datadoginstrumentation) | Validates `DatadogInstrumentation` CRs | `instrumentation_crd_controller.enabled` |
| `config` | mutating | [`mutate/config`](<<<SRC>>>/pkg/clusteragent/admission/mutate/config) | Injects `DD_AGENT_HOST`, `DD_TRACE_AGENT_URL`, `DD_ENTITY_ID`, external-data env; modes `hostip`, `service`, `socket`, `csi` via `admission_controller.inject_config.mode` | `admission_controller.inject_config.enabled` |
| `tagsfromlabels` | mutating | [`mutate/tagsfromlabels`](<<<SRC>>>/pkg/clusteragent/admission/mutate/tagsfromlabels) | Injects `DD_ENV`/`DD_SERVICE`/`DD_VERSION` from pod or owner labels (unified service tagging) | `admission_controller.inject_tags.enabled` |
| `agent_sidecar` | mutating | [`mutate/agent_sidecar`](<<<SRC>>>/pkg/clusteragent/admission/mutate/agent_sidecar) | Injects a full agent container into pods; provider profiles include Fargate | `admission_controller.agent_sidecar.enabled` |
| `autoscaling` | mutating | [`mutate/autoscaling`](<<<SRC>>>/pkg/clusteragent/admission/mutate/autoscaling) | Applies vertical-scaling resource patches from the [workload autoscaler's](autoscaling.md) `PodPatcher` | `autoscaling.workload.enabled` |
| `spot` | mutating | [`mutate/spot`](<<<SRC>>>/pkg/clusteragent/admission/mutate/spot) | Spot-node scheduling mutations for cluster autoscaling | `autoscaling.cluster.spot.enabled` |
| `appsec_proxies` | mutating | [`mutate/appsec`](<<<SRC>>>/pkg/clusteragent/admission/mutate/appsec) | AppSec proxy sidecar injection (Istio/Envoy Gateway) | AppSec sidecar injection patterns configured (`cluster_agent.appsec.*`) |
| `auto_instrumentation` | mutating | [`mutate/autoinstrumentation`](<<<SRC>>>/pkg/clusteragent/admission/mutate/autoinstrumentation) | APM single-step instrumentation: injects language tracer init containers or CSI volumes | `admission_controller.auto_instrumentation.enabled` |
| `cws_instrumentation` (pods + exec) | mutating | [`mutate/cwsinstrumentation`](<<<SRC>>>/pkg/clusteragent/admission/mutate/cwsinstrumentation) | Injects the `cws-instrumentation` binary for [CWS](../ebpf/cws.md) user-session tracking; also intercepts `pods/exec` | `admission_controller.cws_instrumentation.enabled` |
| `ncclprofiler` | mutating | [`mutate/ncclprofiler`](<<<SRC>>>/pkg/clusteragent/admission/mutate/ncclprofiler) | Injects NCCL profiler hooks into GPU jobs | `admission_controller.nccl_profiler.enabled` |

The `auto_instrumentation` webhook is the largest: it selects tracer libraries and versions ([`language_versions.go`](<<<SRC>>>/pkg/clusteragent/admission/mutate/autoinstrumentation/language_versions.go)), supports target-based configuration (`apm_config.instrumentation.targets`), consumes the language annotations written by the DCA's language-detection patcher (see [Cluster Agent](cluster-agent.md)), gates features on the Kubernetes server version, and can inject via CSI volumes instead of init containers (with optional driver detection, `apm_config.instrumentation.csi_driver_detection_enabled`; injection mechanics in [`libraryinjection`](<<<SRC>>>/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection)).

## The webhook server

[`cmd/cluster-agent/admission/server.go`](<<<SRC>>>/cmd/cluster-agent/admission/server.go) serves HTTPS on `admission_controller.port` (8000). Each webhook contributes an endpoint path; the server decodes the `AdmissionReview` (v1 or v1beta1, chosen by the request's GVK), calls the webhook's `WebhookFunc`, and returns a JSONPatch response. Mutation failures are converted to "allow without patch" responses wherever possible — the admission controller is designed to never block workloads because Datadog had a bug.

## The remote-config patcher

Separate from the webhooks themselves, [`patch/`](<<<SRC>>>/pkg/clusteragent/admission/patch) implements the legacy "auto-instrumentation patcher" (`admission_controller.auto_instrumentation.patcher.enabled`): it consumes the `APM_TRACING` [remote config](../configuration/remote-config.md) product and patches Deployments with SSI labels and annotations ([`rc_provider.go`](<<<SRC>>>/pkg/clusteragent/admission/patch/rc_provider.go), [`patcher.go`](<<<SRC>>>/pkg/clusteragent/admission/patch/patcher.go)). The patch changes the pod template, Kubernetes rolls the Deployment, and the `auto_instrumentation` webhook picks the new pods up — this is how enabling instrumentation from the Datadog UI reaches running workloads.

## Failure modes

- **Failure policy defaults to `Ignore`** (`admission_controller.failure_policy`): if the DCA is down or slow, the apiserver admits pods unmutated rather than blocking scheduling. Flipping it to `Fail` makes Datadog a hard dependency of pod creation — almost never what you want.
- **Unmutated pods are silent.** With `Ignore`, an outage window produces pods with no Datadog env vars or tracers; nothing retroactively fixes them until they are recreated.
- **Validating informer sync is tolerated.** On clusters whose RBAC predates the validating webhook, the `ValidatingWebhooksInformer` cannot sync; startup downgrades that specific `SyncInformersError` to a warning and closes the validating informer channel instead of failing the whole admission controller.
- **Certificate rotation is transparent** (hot-loaded per handshake), but deleting the Secret manually forces a re-issue and a CA-bundle re-reconcile; during the seconds in between, the apiserver may reject the webhook's TLS and — with `Ignore` — skip mutation.
- **The probe** ([`probe/`](<<<SRC>>>/pkg/clusteragent/admission/probe), `admission_controller.probe.enabled`) creates a canary admission request path and reports to the health platform whether the webhook is actually intercepting — the only positive signal that the whole chain (Service, cert, webhook object, server) works.
- **Ordering is a contract.** New webhooks that interact with volumes or env vars injected by `config` must be added after it in `generateWebhooks`; the function comment documents the Fargate socket-mount case.

## Configuration

| Key | Default | Meaning |
|---|---|---|
| `admission_controller.enabled` | false | Master switch |
| `admission_controller.port` | 8000 | Webhook server port |
| `admission_controller.webhook_name` | `datadog-webhook` | Name of both webhook configuration objects |
| `admission_controller.service_name` | `datadog-admission-controller` | Service the apiserver calls; also the cert's DNS name |
| `admission_controller.certificate.secret_name` | `webhook-certificate` | Where the TLS cert lives |
| `admission_controller.certificate.validity_bound` | 8760 h | Certificate validity |
| `admission_controller.certificate.expiration_threshold` | 720 h | Renewal lead time |
| `admission_controller.failure_policy` | Ignore | Apiserver behavior when the webhook is unreachable |
| `admission_controller.mutation.enabled` / `.validation.enabled` | true | Kill switches for each webhook class |
| `admission_controller.inject_config.mode` | hostip | How `config` points apps at the node agent: `hostip`, `service`, `socket`, `csi` |
| `admission_controller.auto_instrumentation.enabled` | true | SSI webhook (still requires the master switch) |
| `apm_config.instrumentation.*` | — | SSI targets, enabled namespaces, CSI driver detection |
| `admission_controller.probe.enabled` | false | Self-test probe |

## Deployment notes

On **Fargate** the admission controller is the delivery mechanism for the agent itself: the `agent_sidecar` webhook injects an agent container into every matching pod because no DaemonSet can exist, and its provider profile strips the socket mount added by `config`. On mixed clusters, namespace and object selectors (auto-chosen by API discovery) scope which pods each webhook touches; `MatchConditions` narrow it further on Kubernetes versions that support CEL. All reconciliation is leader-only, so a follower-only DCA outage does not affect webhook object state — only capacity to answer admission requests.
