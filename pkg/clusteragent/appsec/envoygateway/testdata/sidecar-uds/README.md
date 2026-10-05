# Envoy Gateway Datadog AppSec sidecar over UDS

Known-good local validation manifests for Envoy Gateway + Datadog AppSec `ext_proc` using a Unix Domain Socket backend.

Validated on the local Rancher Desktop Kubernetes context (`rancher-desktop`) with Envoy Gateway Helm chart `gateway-helm` `v1.7.1` / controller image `docker.io/envoyproxy/gateway:v1.7.1`.

## Prerequisites

Never apply this to a shared or remote cluster. Confirm the local context first:

```sh
kubectl config current-context
# expected: rancher-desktop
```

Install Envoy Gateway v1.7.1:

```sh
helm install eg oci://docker.io/envoyproxy/gateway-helm \
  --version v1.7.1 \
  -n envoy-gateway-system \
  --create-namespace
```

If Gateway API CRDs already exist and Helm reports server-side apply conflicts, this local run used:

```sh
helm install eg oci://docker.io/envoyproxy/gateway-helm \
  --version v1.7.1 \
  -n envoy-gateway-system \
  --create-namespace \
  --skip-crds

helm show crds oci://docker.io/envoyproxy/gateway-helm --version v1.7.1 \
  | kubectl apply --server-side --force-conflicts -f -

kubectl rollout restart deployment/envoy-gateway -n envoy-gateway-system
kubectl rollout status deployment/envoy-gateway -n envoy-gateway-system --timeout=180s
```

## Enable the Backend extension API

`Backend` must be enabled in Envoy Gateway before the UDS endpoint is consumed. This is a manual prerequisite; the future cluster-agent automation should detect and warn, not mutate this ConfigMap.

Command used in the local validation:

```sh
kubectl patch cm envoy-gateway-config -n envoy-gateway-system --type merge -p '{"data":{"envoy-gateway.yaml":"apiVersion: gateway.envoyproxy.io/v1alpha1\nkind: EnvoyGateway\nextensionApis:\n  enableBackend: true\ngateway:\n  controllerName: gateway.envoyproxy.io/gatewayclass-controller\nlogging:\n  level:\n    default: info\nprovider:\n  kubernetes:\n    rateLimitDeployment:\n      container:\n        image: docker.io/envoyproxy/ratelimit:c8765e89\n      patch:\n        type: StrategicMerge\n        value:\n          spec:\n            template:\n              spec:\n                containers:\n                - imagePullPolicy: IfNotPresent\n                  name: envoy-ratelimit\n    shutdownManager:\n      image: docker.io/envoyproxy/gateway:v1.7.1\n  type: Kubernetes\n"}}'
kubectl rollout restart deployment/envoy-gateway -n envoy-gateway-system
kubectl rollout status deployment/envoy-gateway -n envoy-gateway-system --timeout=180s
```

The installed CRDs were verified as the source of truth with:

```sh
kubectl explain envoyextensionpolicy.spec.extProc
kubectl explain envoyextensionpolicy.spec.extProc.backendRefs
kubectl explain envoyextensionpolicy.spec.extProc.processingMode
kubectl explain backend.spec.endpoints
kubectl explain backend.spec.endpoints.unix
kubectl explain envoyproxy.spec.provider.kubernetes.envoyDeployment.patch
```

## Cluster-agent RBAC (automated path)

The manifests above (`02-envoyproxy.yaml`, `04-backend-uds.yaml`, `05-envoyextensionpolicy.yaml`) reproduce the result **by hand**, which needs no cluster-agent permissions. When the cluster-agent does the work instead — the webhook injecting the `datadog-appsec` sidecar and the controller creating the `Backend` + `EnvoyExtensionPolicy` — its ServiceAccount needs the permissions in [`00-rbac.yaml`](./00-rbac.yaml).

The published Datadog Helm chart's `datadog.appsec.injector.enabled` RBAC block currently grants `envoyextensionpolicies` under `gateway.envoyproxy.io` but **not `backends`**, so UDS sidecar mode fails with `forbidden` on `backends.gateway.envoyproxy.io` until that rule is added. See the handover note `appsec-injector-eg-uds-backend-rbac-handover.md` in the `DataDog/helm-charts` repo. For a local run with a hand-built cluster-agent (or an older chart), apply the reference role and adjust the ServiceAccount subject to your release:

```sh
kubectl apply -f pkg/clusteragent/appsec/envoygateway/testdata/sidecar-uds/00-rbac.yaml
```

## Sidecar image

The real Datadog serviceextensions image was used, not a stand-in:

```text
ghcr.io/datadog/dd-trace-go/service-extensions-callout@sha256:d2a11c0346ee8a907749a7af5a7aba96546a0200cd9e4da34b1048e4c07c764f
```

The image is documented in `/Users/eliott.bouhana/go/src/github.com/DataDog/dd-trace-go/contrib/envoyproxy/go-control-plane/cmd/serviceextensions/README.md`. The README names the GitHub Container Registry package `DataDog/dd-trace-go/service-extensions-callout`; the Dockerfile is in the same directory.

The image was pulled locally with:

```sh
docker pull ghcr.io/datadog/dd-trace-go/service-extensions-callout:latest
```

Rancher Desktop could then use the image directly. If a cluster cannot pull GHCR, build and load it from the `dd-trace-go` checkout:

```sh
cd /Users/eliott.bouhana/go/src/github.com/DataDog/dd-trace-go
docker build -f contrib/envoyproxy/go-control-plane/cmd/serviceextensions/Dockerfile \
  -t datadog/serviceextensions:dev .
```

For Rancher Desktop using the Docker runtime, the image is already in the local Docker image store. For a containerd-backed local cluster, import/load according to the Rancher Desktop runtime configuration before applying `02-envoyproxy.yaml`.

## Apply order

From the repository root:

```sh
DIR=pkg/clusteragent/appsec/envoygateway/testdata/sidecar-uds
for file in \
  "$DIR/01-gatewayclass.yaml" \
  "$DIR/02-envoyproxy.yaml" \
  "$DIR/03-gateway.yaml" \
  "$DIR/04-backend-uds.yaml" \
  "$DIR/05-envoyextensionpolicy.yaml" \
  "$DIR/06-httproute.yaml" \
  "$DIR/07-sample-backend.yaml"; do
  kubectl apply -f "$file"
done
```

Wait for the sample app and Envoy data-plane pod:

```sh
kubectl wait --for=condition=ready pod -l app=sample-backend -n eg-appsec-demo --timeout=180s
kubectl wait --for=condition=ready pod \
  -l gateway.envoyproxy.io/owning-gateway-name=appsec-gateway \
  -n envoy-gateway-system \
  --timeout=240s
```

## Required target pod-state

`02-envoyproxy.yaml` applies a StrategicMerge patch to the Envoy Gateway data-plane Deployment template. The cluster-agent webhook implementation should reproduce the same target state:

- pod volume: `datadog-appsec-uds` as `emptyDir: {}`
- pod security context: `fsGroup: 65532`, `fsGroupChangePolicy: OnRootMismatch`
- existing `envoy` container volumeMount: `datadog-appsec-uds` mounted at `/var/run/datadog`
- sidecar container: `datadog-appsec`
- sidecar image: `ghcr.io/datadog/dd-trace-go/service-extensions-callout@sha256:d2a11c0346ee8a907749a7af5a7aba96546a0200cd9e4da34b1048e4c07c764f`
- sidecar env (injected by the webhook `BuildExtProcProcessorContainerUDS`):
  - `DD_SERVICE_EXTENSION_UDS_PATH=/var/run/datadog/extproc.sock`
  - `DD_SERVICE_EXTENSION_TLS=false`
  - `DD_SERVICE_EXTENSION_HEALTHCHECK_PORT=8081`
  - `DD_SERVICE_EXTENSION_OBSERVABILITY_MODE=false`
  - `DD_APM_TRACING_ENABLED=false`
  - `DD_APPSEC_BODY_PARSING_SIZE_LIMIT` (only when configured)
- sidecar env set ONLY by the manual `02-envoyproxy.yaml` patch (the webhook does NOT inject these):
  - `DD_APPSEC_WAF_TIMEOUT=10ms`
  - `DD_TRACE_AGENT_URL=http://127.0.0.1:8126` (optional local-test placeholder)
- sidecar port: named `health`, `containerPort: 8081`
- sidecar probes: HTTP GET `/` on `health`
- sidecar security context: `runAsUser: 65532`, `runAsGroup: 65532`, `allowPrivilegeEscalation: false`

The upstream Envoy Gateway pod also includes its own `shutdown-manager` container, so the observed pod had `envoy`, `datadog-appsec`, and `shutdown-manager`. The Datadog sidecar and the Envoy container shared the UDS volume.

## Traffic test

Port-forward the generated Envoy Gateway Service:

```sh
SVC=$(kubectl get svc -n envoy-gateway-system \
  -l gateway.envoyproxy.io/owning-gateway-name=appsec-gateway \
  -o jsonpath='{.items[0].metadata.name}')
kubectl port-forward svc/$SVC -n envoy-gateway-system 8888:80
```

In another shell:

```sh
curl -sv --max-time 10 http://127.0.0.1:8888/
curl -sv --max-time 10 -H 'User-Agent: dd-test-scanner-log-block' \
  'http://127.0.0.1:8888/?x=1'
```

Observed result from the validation run:

```text
benign request: HTTP/1.1 200 OK
NOW: 2026-06-16 15:37:36.846808188 +0000 UTC m=+145.790841820

attack request with User-Agent: dd-test-scanner-log-block: HTTP/1.1 403 Forbidden
{"errors":[{"title":"You've been blocked","detail":"Sorry, you cannot access this page. Please contact the customer service team. Security provided by Datadog."}],"security_response_id":"a4e08ef2-66e4-46a6-1bee-9ab7c838c0d2"}
```

Key sidecar log lines proving the Datadog ext_proc server listened on the UDS and processed traffic:

```text
INFO: 2026/06/16 15:35:17 service_extension: health check server started on 0.0.0.0:8081
INFO: 2026/06/16 15:35:17 service_extension: callout gRPC server started on unix:///var/run/datadog/extproc.sock
2026/06/16 15:37:36 Datadog Tracer v2.8.2 INFO: external_processing: first request received. Configuration: BlockingUnavailable=false, BodyParsingSizeLimit=0B, Framework=github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3
```

Socket evidence from the pod:

```text
$ kubectl exec -n envoy-gateway-system "$POD" -c datadog-appsec -- ls -l /var/run/datadog
srwxr-xr-x    1 65532    65532            0 Jun 16 15:35 extproc.sock
```

Because no Datadog Agent was running in the pod, the sidecar also logged expected local telemetry warnings like `dial tcp 127.0.0.1:8126: connect: connection refused`. These did not prevent local AppSec blocking.

## Reconcile sanity check

After the data-plane pod was ready, a no-op Gateway annotation update was applied:

```sh
POD_BEFORE=$(kubectl get pods -n envoy-gateway-system \
  -l gateway.envoyproxy.io/owning-gateway-name=appsec-gateway \
  -o jsonpath='{.items[0].metadata.name}')
kubectl annotate gateway appsec-gateway -n eg-appsec-demo \
  appsec.datadoghq.com/noop-reconcile="$(date +%s)" --overwrite
sleep 30
POD_AFTER=$(kubectl get pods -n envoy-gateway-system \
  -l gateway.envoyproxy.io/owning-gateway-name=appsec-gateway \
  -o jsonpath='{.items[0].metadata.name}')
```

Observed result:

```text
before=envoy-eg-appsec-demo-appsec-gateway-78fd575f-6cbc54df9b-5mjhp
after=envoy-eg-appsec-demo-appsec-gateway-78fd575f-6cbc54df9b-5mjhp
containers: envoy, datadog-appsec, shutdown-manager
volume: datadog-appsec-uds
envoy mount: /var/run/datadog
datadog-appsec mount: /var/run/datadog
```

Envoy Gateway did not strip the sidecar or re-roll the pod during this short no-op reconcile check.

## Cleanup

```sh
kubectl delete ns eg-appsec-demo --ignore-not-found=true
kubectl delete gatewayclass eg-appsec-demo --ignore-not-found=true
kubectl delete envoyproxy datadog-appsec-sidecar-uds -n envoy-gateway-system --ignore-not-found=true

# Optional: remove Envoy Gateway itself.
helm uninstall eg -n envoy-gateway-system
kubectl delete ns envoy-gateway-system --ignore-not-found=true
```
