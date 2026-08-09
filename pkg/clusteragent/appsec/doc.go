// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

/*
Package appsec implements automatic security processor injection for API and Application
Security (AppSec) monitoring in Kubernetes clusters.

# Overview

The appsec package provides infrastructure-level security monitoring for Kubernetes by automatically
configuring ingress proxies and gateways to route traffic through Datadog's Application Security
processor. This enables customers to gain API-wide security coverage without modifying individual
services or deploying tracers across their entire application fleet.

The package acts as a Kubernetes controller that watches for supported proxy resources (e.g., Envoy
Gateway instances) and automatically injects the necessary configuration to enable AppSec processing,
eliminating the need for manual proxy configuration and expertise in proxy-specific features.

# Architecture

The appsec injector follows a controller pattern with these key components:

  - securityInjector: The main controller that manages injection lifecycle
  - InjectionPattern: An interface for proxy-specific injection implementations
  - ProxyDetector: Auto-detection of supported proxies in the cluster
  - Configuration: Unified configuration for processor deployment and injection
  - ConfigMapReconciler: Watches original ConfigMaps for changes and re-syncs DD-owned copies (nginx only)

The controller watches proxy resources using Kubernetes informers and maintains a work queue for
processing add/modify/delete events with exponential backoff retry logic.

# Supported Proxies

Currently supported proxy types:

- Istio (ProxyTypeIstio): Configures EnvoyFilter resources for traffic routing
  - EXTERNAL mode: Routes to external processor service
  - SIDECAR mode: Routes to localhost sidecar, injects processor container into gateway pods

- Envoy Gateway (ProxyTypeEnvoyGateway): Configures EnvoyExtensionPolicy resources
  - EXTERNAL mode: Routes to external processor service
  - SIDECAR mode: Routes to localhost sidecar via Unix Domain Socket (UDS), injects processor container into gateway pods

- ingress-nginx (ProxyTypeIngressNginx): Injects the nginx-datadog WAF module (.so) into controller pods
  - Pod mutation mode only (uses init container, not a running sidecar): Adds init container + emptyDir volume, redirects --configmap to DD-owned ConfigMap
  - Auto-detects via IngressClass with spec.controller == "k8s.io/ingress-nginx"
  - Version detection from controller image tag for matching init container image

- GKE Gateway (ProxyTypeGKEGateway): Configures GCPTrafficExtension resources for external traffic routing
  - EXTERNAL mode only: Managed GKE has no in-cluster data plane; SIDECAR mode is not supported
  - Auto-detects via GatewayClass with external-managed controllers (gke-l7-global-external-managed, gke-l7-regional-external-managed); multi-cluster -mc variants are excluded by default (they require a ServiceImport backendRef; follow-up)
  - Creates one GCPTrafficExtension (networking.gke.io/v1) per Gateway in the Gateway's own namespace
  - Callout Deployment/Service/HealthCheckPolicy are user-deployed per public GKE docs

Each proxy type implements the InjectionPattern interface, providing:
  - Resource detection (IsInjectionPossible)
  - Resource watching (Resource, Namespace)
  - Lifecycle management (Added, Modified, Deleted)
  - Mode detection (Mode)

In SIDECAR mode, proxy types additionally implement the SidecarInjectionPattern interface:
  - Pod selection (PodSelector)
  - Sidecar injection (InjectSidecar)
  - Pod deletion handling (usually no-op; Gateway informer drives proxy-resource cleanup)

# Deployment Modes

The appsec package supports two deployment modes for the Application Security processor:

## EXTERNAL Mode

In EXTERNAL mode, users manually deploy the AppSec processor as a standalone Kubernetes service.
The injector configures proxies to route traffic to this external service via cross-namespace
service references.

Characteristics:
  - Processor deployed as separate Deployment/Service
  - Centralized processor instance(s) handle traffic from multiple gateways
  - Requires manual processor deployment and lifecycle management
  - Service discovery via Kubernetes service DNS
  - Suitable for centralized security processing

## SIDECAR Mode

In SIDECAR mode, the AppSec processor is automatically injected as a sidecar container into
gateway pods via a Kubernetes mutating admission webhook. The processor runs co-located with
each gateway instance, processing traffic via localhost.

Characteristics:
  - No manual processor deployment required
  - Processor lifecycle tied to gateway pod lifecycle
  - Traffic processed via localhost (reduced network overhead)
  - Automatic scaling with gateway pods
  - Suitable for distributed security processing

Key differences:

	| Aspect              | EXTERNAL                    | SIDECAR                          |
	|---------------------|----------------------------|----------------------------------|
	| Deployment          | Manual service deployment  | Automatic sidecar injection      |
	| Network             | Cross-namespace service    | Localhost communication          |
	| Scaling             | Independent                | Coupled with gateway pods        |
	| Resource overhead   | Centralized                | Per-gateway pod                  |
	| Lifecycle           | User-managed               | Automatic (webhook-managed)      |
	| Configuration       | Created at startup         | Lazy (on first pod injection)    |

# Configuration

The package is configured through the Datadog Cluster Agent configuration:

## EXTERNAL Mode Configuration

	appsec:
	  proxy:
	    enabled: true                           # Enable AppSec proxy integration
	    auto_detect: true                       # Auto-detect proxies in cluster
	    proxies: ["istio", "envoy-gateway"]     # Explicitly enabled proxies
	    processor:
	      address: "extproc-service.ns.svc"     # Processor service address (optional)
	      port: 443                             # Processor service port
	cluster_agent:
	  appsec:
	    injector:
	      enabled: true                         # Enable automatic injection
	      mode: "external"                      # Use EXTERNAL mode
	      labels: {}                            # Additional labels for created resources
	      annotations: {}                       # Additional annotations
	      processor:
	        service:
	          name: "extproc-service"           # Service name (required)
	          namespace: "datadog"              # Service namespace (optional, defaults to cluster-agent namespace)

## SIDECAR Mode Configuration

	appsec:
	  proxy:
	    enabled: true                           # Enable AppSec proxy integration
	    auto_detect: true                       # Auto-detect proxies in cluster
	    proxies: ["istio"]                      # Explicitly enabled proxies
	cluster_agent:
	  appsec:
	    injector:
	      enabled: true                         # Enable automatic injection
	      mode: "sidecar"                       # Use SIDECAR mode
	      labels: {}                            # Additional labels for created resources
	      annotations: {}                       # Additional annotations
	      sidecar:
	        image: "ghcr.io/datadog/dd-trace-go/service-extensions-callout"  # Sidecar image (required)
	        image_tag: "latest"                 # Image tag (optional)
	        port: 8080                          # Processor port (required)
	        health_port: 8081                   # Health check port (required, must differ from port)
	        uds_path: "/var/run/datadog/extproc.sock" # UDS socket path (optional, default: /var/run/datadog/extproc.sock)
	        run_as_user: 65532                  # User ID for sidecar container (optional, default: 65532)
	        body_parsing_size_limit: "10000000" # Body size limit in bytes (optional, default: 10MB)
	        resources:
	          requests:
	            cpu: "100m"                     # CPU request (optional)
	            memory: "128Mi"                 # Memory request (optional)
	          limits:
	            cpu: "200m"                     # CPU limit (optional)
	            memory: "256Mi"                 # Memory limit (optional)

# Usage

The package is automatically started by the Cluster Agent when both product and injection
features are enabled:

	if config.GetBool("appsec.proxy.enabled") && config.GetBool("cluster_agent.appsec.injector.enabled") {
	    if err := appsec.Start(ctx, logger, config); err != nil {
	        log.Errorf("Cannot start appsec injector: %v", err)
	    }
	}

The Start function initializes the injector, detects available proxies, and launches a goroutine
for each enabled proxy type to watch and manage resources.

# SIDECAR Mode Implementation

When SIDECAR mode is configured, the package uses a Kubernetes mutating admission webhook to
intercept pod creation/deletion events and inject the AppSec processor sidecar container into
gateway pods.

## Admission Webhook

The webhook is registered at the `/appsec-proxies` endpoint and handles:
  - Pod CREATE operations: Inject sidecar container if pod matches selection criteria
  - Pod DELETE operations: No-op for Istio and Envoy Gateway sidecar patterns

Key features:
  - CEL-based filtering: Label selectors are compiled into Common Expression Language (CEL)
    expressions and embedded in the MutatingWebhookConfiguration, enabling efficient
    server-side filtering before webhook invocation
  - Pattern-based routing: Each proxy pattern provides a PodSelector that determines
    which pods should receive sidecar injection
  - Lazy initialization: Proxy configuration resources (EnvoyFilter, EnvoyExtensionPolicy)
    are created on-demand during first sidecar injection, not at startup

## CEL Expression Generation

Label selectors from proxy patterns are converted to CEL expressions for webhook filtering:

	# Kubernetes label selector
	selector:
	  matchLabels:
	    gateway.networking.k8s.io/gateway-name: "my-gateway"
	  matchExpressions:
	    - {key: "app", operator: "In", values: ["gateway"]}

	# Generated CEL expression (embedded in webhook config)
	object.metadata.labels["gateway.networking.k8s.io/gateway-name"] == "my-gateway" &&
	object.metadata.labels["app"] in ["gateway"]

Supported operators: Exists, DoesNotExist, Equals (=), NotEquals (!=), In, NotIn, GreaterThan (>), LessThan (<)

Multiple patterns are combined with OR logic to create a unified webhook that handles all proxy types.

## Sidecar Injection Lifecycle

1. **Pod Creation Event**: API server invokes webhook for pods matching CEL expression
2. **Pattern Matching**: Webhook iterates through registered patterns to find matches
3. **Already Injected Check**: Skip if sidecar container already exists
4. **Container Injection**: Add AppSec processor container with configured resources
5. **Lazy Config Creation**: On first injection, create proxy configuration (EnvoyFilter, or Envoy Gateway Backend + EnvoyExtensionPolicy)
6. **Pod Admission**: Return modified pod spec to API server

Example injected sidecar container:

	containers:
	- name: datadog-appsec-processor
	  image: ghcr.io/datadog/dd-trace-go/service-extensions-callout:latest
	  ports:
	  - containerPort: 8080      # Processor port
	    protocol: TCP
	  - containerPort: 8081      # Health check port
	    protocol: TCP
	  livenessProbe:
	    httpGet:
	      path: /health
	      port: 8081
	  resources:
	    requests:
	      cpu: "100m"
	      memory: "128Mi"
	    limits:
	      cpu: "200m"
	      memory: "256Mi"
	  env:
	  - name: BODY_PARSING_SIZE_LIMIT
	    value: "10000000"

## Lazy Initialization Pattern

Unlike EXTERNAL mode where configuration is created at cluster-agent startup, SIDECAR mode
uses lazy initialization:

	# Traditional (EXTERNAL mode)
	Start() → Detect proxies → Watch resources → Added() → Create config

	# Lazy (SIDECAR mode)
	Start() → Detect proxies → Register webhook → [Wait for pod]
	                                             ↓
	Pod created → MutatePod() → Create config lazily (idempotent)
	                               ↓
	              Inject container → Return modified pod

Benefits:
  - No unused resources created in namespaces without gateway pods
  - Configuration created with actual pod context (namespace, labels)
  - Reduces startup time and resource overhead
  - Adapts to dynamic namespace creation

Tradeoff:
  - First pod injection is slower due to configuration creation
  - Requires careful handling of concurrent pod creations

## Cleanup Handling

For Istio and Envoy Gateway sidecar mode, pod deletion is a no-op. Cleanup remains driven by the
watched Gateway resources:

 1. **Pod Deletion Event**: API server may invoke the webhook for DELETE operation
 2. **PodDeleted No-op**: Sidecar patterns do not remove proxy configuration from pod deletion
 3. **Gateway Deletion**: The Gateway informer deletes proxy configuration resources when the
    last Gateway in the namespace is deleted

4. **Resource Cleanup**: Kubernetes garbage collection handles pod-owned resources

This avoids removing proxy configuration while replacement gateway pods are still starting.

## Pattern Wrapper Architecture

SIDECAR mode uses a pattern wrapper design where sidecar-capable patterns wrap their
EXTERNAL mode counterparts:

	istioInjectionPattern (EXTERNAL)
	        ↑ embedded
	        |
	istioGatewaySidecarPattern (SIDECAR)

The wrapper:
  - Embeds the EXTERNAL mode pattern for proxy configuration logic
  - Implements SidecarInjectionPattern interface for pod injection
  - Delegates Added() calls to no-op (lazy initialization)
  - Creates config during MutatePod() by calling the embedded pattern
  - Leaves PodDeleted() as a no-op; Gateway-informer Deleted() handles cleanup when the last Gateway in the namespace is removed

This design maximizes code reuse while maintaining clear separation between deployment modes.

# Proxy-Specific Implementations

## Istio Implementation

Istio support is implemented via the istio subpackage and works in both EXTERNAL and SIDECAR modes.

### EXTERNAL Mode (istioInjectionPattern)

For Istio in EXTERNAL mode:
 1. Watches GatewayClass resources (gateway.networking.k8s.io/v1)
 2. Creates EnvoyFilter resources that configure External Processing
 3. EnvoyFilter is created in the configured Istio namespace (cluster-wide scope)
 4. Routes traffic to external processor service via cross-namespace reference

### SIDECAR Mode (istioGatewaySidecarPattern)

For Istio in SIDECAR mode:
 1. Registers webhook with label selector for gateway pods
 2. Injects sidecar container into pods with matching labels
 3. Creates EnvoyFilter on first pod injection (lazy initialization)
 4. Routes traffic to localhost:8080 (sidecar processor)
 5. Cleans up EnvoyFilter when the last Gateway in the namespace is deleted

Pod selection criteria:
  - Pods must have label: gateway.networking.k8s.io/gateway-name (set by Istio gateway controller)
  - GatewayClass must have controller: istio.io/gateway-controller

## Envoy Gateway Implementation

Envoy Gateway support is implemented via the envoygateway subpackage and works in both EXTERNAL and SIDECAR modes.

### EXTERNAL Mode (envoyGatewayInjectionPattern)

For Envoy Gateway in EXTERNAL mode:
 1. Watches Gateway resources (gateway.networking.k8s.io/v1)
 2. Creates EnvoyExtensionPolicy resources that configure External Processing
 3. EnvoyExtensionPolicy backendRefs point at a Kubernetes Service
 4. Manages ReferenceGrant resources for cross-namespace access
 5. Handles cleanup when gateways are deleted

### SIDECAR Mode (envoyGatewaySidecarPattern)

For Envoy Gateway in SIDECAR mode:
 1. Registers webhook with label selector for gateway pods
 2. Injects sidecar container into pods with matching labels
 3. Creates a Backend (UDS) and an EnvoyExtensionPolicy on first pod injection (lazy initialization)
 4. EnvoyExtensionPolicy backendRefs point to the UDS Backend in the same namespace
 5. Routes traffic to the sidecar processor via Unix Domain Socket at /var/run/datadog/extproc.sock
 6. Cleans up resources when the last Gateway in the namespace is deleted; PodDeleted is a no-op

Sidecar Data Flow and Invariants:
  - The webhook injects the sidecar container and a shared emptyDir volume (datadog-appsec-uds) mounted into both the injected sidecar and the envoy container.
  - Pod fsGroup and sidecar runAsUser are set to 65532 to ensure shared-socket access.
  - The Gateway informer does not eagerly create sidecar-mode resources on Gateway add; MutatePod lazily creates a Backend (UDS) and an EnvoyExtensionPolicy in the Gateway's namespace on first matching pod injection.
  - Since the Backend is in the same namespace as the EnvoyExtensionPolicy, no ReferenceGrant is created (the no-ReferenceGrant invariant).
  - The extensionApis.enableBackend Envoy Gateway prerequisite is detected and warned about if disabled (the cluster-agent does not modify Envoy Gateway config).
  - The default injector mode is sidecar, so appsec-enabled Envoy Gateways default to sidecar injection.

Pod selection criteria:
  - Pods must have label: gateway.envoyproxy.io/owning-gateway-name
  - Pods must have label: gateway.envoyproxy.io/owning-gateway-namespace

## ingress-nginx Implementation (SIDECAR Mode Only)

ingress-nginx uses a native nginx module (.so) rather than Envoy's ext_proc protocol. The
implementation uses a ConfigMap redirect approach since load_module must be in the nginx main
context and can only be injected via ConfigMap snippets.

### Detection

Detects ingress-nginx by listing IngressClass resources and checking for
spec.controller == "k8s.io/ingress-nginx".

### Pod Mutation (nginxSidecarPattern)

 1. Parses the controller image tag to determine the nginx version (e.g., v1.15.1)
 2. Resolves the original ConfigMap name from the --configmap controller arg
 3. Creates a DD-owned ConfigMap mirroring the original with AppSec directives prepended
 4. Adds an init container (datadog/ingress-nginx-injection:<version>) that copies the .so module
 5. Adds an emptyDir volume shared between init and controller containers
 6. Redirects the --configmap arg to the DD-owned ConfigMap

### ConfigMap Management

The DD-owned ConfigMap:
  - Copies all keys from the original ConfigMap verbatim
  - Injects load_module + thread_pool + env DD_AGENT_HOST into main-snippet using idempotent comment markers
  - Injects datadog_appsec_enabled + datadog_waf_thread_pool_name into http-snippet using idempotent comment markers
  - Comment markers (# datadog-appsec-begin / # datadog-appsec-end) enable safe re-application and clean removal
  - Has an ownerReference to the original ConfigMap for garbage collection
  - Is labeled with appsec.datadoghq.com/proxy-type=ingress-nginx for cleanup

### Cleanup

On IngressClass deletion, checks if other ingress-nginx IngressClasses still exist before
deleting DD ConfigMaps (cross-controller coordination, same pattern as Istio).

### Key Differences from Istio/Envoy Gateway

  - No ext_proc: Uses native nginx module loaded via load_module directive
  - ConfigMap management: Creates/syncs a separate ConfigMap instead of CRD resources
  - Init container: Copies .so module, not a running sidecar process
  - Version coupling: Init container image must match nginx version
  - Mode() always returns SIDECAR regardless of global config (no external mode)

## GKE Gateway Implementation

GKE Gateway support is implemented via the gke subpackage and works in EXTERNAL mode ONLY.
Managed GKE has no in-cluster Envoy data plane, so SIDECAR mode is structurally impossible.

### Detection

Detects GKE-managed Gateways by inspecting the GatewayClass controllerName against an allowlist
of external-managed class names:

  - gke-l7-global-external-managed
  - gke-l7-regional-external-managed

Internal GKE GatewayClasses (e.g. gke-l7-rilb, gke-l7-regional-internal-managed) are intentionally
excluded because they do not support GCPTrafficExtension-based callouts. The multi-cluster variants
(gke-l7-global-external-managed-mc, gke-l7-regional-external-managed-mc) are also intentionally
excluded from the default allowlist: multi-cluster Gateways require the callout backendRef to be a
ServiceImport (group net.gke.io), whereas this reconciler only emits a core Service backendRef.
Multi-cluster (ServiceImport) support is a documented follow-up. The allowlist is overridable via
the appsec.proxy.gke.gateway_classes configuration key.

Gateway resources carrying the label appsec.datadoghq.com/enabled=false are skipped.

### Resource Lifecycle (CREATE-ONLY / Gateway-driven)

The injector watches Gateway resources (not GCPTrafficExtensions) and reacts to Add and Delete events:

 1. On Gateway Add: creates one GCPTrafficExtension (networking.gke.io/v1) in the Gateway's own namespace
 2. On Gateway Delete: deletes the corresponding GCPTrafficExtension

CRITICAL: The injector is CREATE-ONLY and has NO UpdateFunc, no GCPTrafficExtension watch, and no
periodic resync. Consequences:

  - A manually-edited or stale GCPTrafficExtension is NOT auto-reconciled.
  - A GatewayClass change on a live Gateway is NOT detected.
  - Deleting the GCPTrafficExtension alone does NOT recreate it — recreation requires a new Gateway
    Add event (delete+recreate the Gateway, or restart the cluster-agent / trigger leader re-election
    so the informer replays AddFunc for all existing Gateways). This create-only, no-resync behavior
    has been verified on a live GKE cluster: after deleting the CR while the Gateway still existed,
    the cluster-agent did not recreate it until the pod was restarted.
  - There is no drift reconciliation in v1.

### Ownership and Cleanup Model (NO ownerReferences)

The GCPTrafficExtension is created WITHOUT any metadata.ownerReferences. Cleanup is driven entirely
by the informer's DeleteFunc (Gateway delete) and by the disable-time Cleanup pass — NOT by
Kubernetes garbage collection. This is a deliberate design choice; the tradeoffs are:

  - If the cluster-agent is down (or not leader) at the moment a Gateway is deleted, the DeleteFunc
    event is missed and the GCPTrafficExtension is left orphaned until the agent restarts and its
    Cleanup/reconcile logic runs again (note: current v1 only deletes on live Delete events and on
    disable; it does not sweep orphans whose Gateway disappeared while the agent was down).
  - An ownerReference to the Gateway WOULD be namespace-valid (same namespace, see below) and would
    let Kubernetes GC remove the extension even while the agent is down; adopting it is a candidate
    hardening follow-up. It is intentionally omitted in v1 to keep the reconciler informer-driven.

### Same-Namespace Constraint (no cross-namespace, no ReferenceGrant)

GCPTrafficExtension targetRefs and backendRef do not carry a namespace field — this is enforced by
the CRD itself (verified empirically: applying either spec.targetRefs[].namespace or
spec.extensionChains[].extensions[].backendRef.namespace is rejected with a strict-decoding
"unknown field" error). Therefore:

  - The GCPTrafficExtension MUST reside in the Gateway's own namespace (targetRefs is a local ref).
  - The callout backend Service is always resolved in that same namespace (backendRef is local).
  - Cross-namespace backends are structurally impossible, so NO Gateway API ReferenceGrant is needed
    or even possible for the callout wiring.
  - Consequently, Processor.Namespace is NOT used for GKE Gateway injection; the callout Deployment,
    Service, and HealthCheckPolicy must be user-deployed in each Gateway's namespace per the public
    GKE service-extensions documentation.

### Teardown and Disabling (eventual GCP-resource garbage collection)

Two paths remove a managed GCPTrafficExtension:

 1. Gateway deletion -> the informer DeleteFunc calls Deleted(), which deletes the CR.
 2. Disabling injection (appsec.proxy.enabled=false or cluster_agent.appsec.injector.enabled=false)
    -> the start command runs appsec.Cleanup (leader-gated), which lists every Gateway and calls
    Deleted() for each, removing all cluster-agent-managed GCPTrafficExtensions.

CRITICAL teardown caveat (verified on a live GKE cluster): deleting the GCPTrafficExtension CR
removes the Kubernetes object immediately, but the GKE Gateway controller garbage-collects the
UNDERLYING GCP resource (a networkservices lbTrafficExtension) only EVENTUALLY. The GKE controller
does not place a finalizer on the CR, so there is no synchronous teardown hook. Measured lag on a
live cluster was ~5-7 minutes (observed ~401s) between CR deletion and the GCP lbTrafficExtension
disappearing / edge blocking stopping. During that window the callout remains programmed at the load
balancer and traffic continues to be inspected/blocked. Operators disabling AppSec should expect this
delay before blocking fully stops; it clears without manual intervention. The injector deliberately
does NOT call the GCP networkservices API to force-delete the resource (K8s-only reconciler); owning
that teardown via the GCP API is a possible follow-up if synchronous disable is ever required.

### GCPTrafficExtension Shape

One GCPTrafficExtension per Gateway is created with:

  - metadata.namespace: the Gateway's own namespace (same-namespace constraint)
  - metadata.labels: app.kubernetes.io/managed-by=datadog-cluster-agent
  - spec.targetRefs: one entry pointing at the Gateway (no namespace field; local ref)
  - spec.extensionChains: one chain with matchCondition.celExpressions: [{celMatcher: "1 == 1"}]
    and one extension entry with failOpen: true, supportedEvents: [RequestHeaders, ResponseHeaders],
    timeout: 1s, and a backendRef to the user-deployed callout Service (no namespace field)

Body inspection note: supportedEvents intentionally lists only the header events. The Datadog
callout still inspects request/response bodies when a rule needs them: the dd-trace-go ext_proc
callout dynamically requests the body via the ext_proc mode-override (ModeOverride
RequestBodyMode=STREAMED) after seeing the request headers, and GKE's managed data plane honors
allow_mode_override. This was verified on a live GKE cluster: with supportedEvents restricted to
header events, a WAF rule matching the parsed JSON request body still blocked the request (HTTP
403). Statically adding RequestBody/ResponseBody here is therefore unnecessary and would defeat the
callout's dynamic negotiation by forcing the data plane to stream every body unconditionally.

Load balancer programming lag is approximately 2 minutes after GCPTrafficExtension creation.

### Error Handling and RBAC Requirements

The cluster-agent requires the following RBAC (none of which ship in the default Datadog Helm
chart / Operator ClusterRole today — enabling the GKE Gateway injector requires adding them, which
is a packaging follow-up):

  - gateways.gateway.networking.k8s.io: get, list, watch (the informer source).
  - gcptrafficextensions.networking.gke.io: get, list, watch, create, delete (Get-before-Create
    ownership guard, create, and teardown).
  - customresourcedefinitions.apiextensions.k8s.io: get, list (Detect() checks for the
    gcptrafficextensions CRD).
  - events (core): create, patch in each Gateway's namespace (the injector records Gateway-scoped
    events; note these are namespaced Roles in the Gateway namespaces, not the cluster-agent's own).

Without the gcptrafficextensions permissions:

  - Any non-NotFound error on Get/Create/Delete (e.g. Forbidden) causes the reconcile to fail.
  - A WARN log is emitted and a Kubernetes event with reason GCPTrafficExtensionCreateFailed or
    GCPTrafficExtensionDeleteFailed is recorded on the Gateway object.
  - The reconcile is requeued once and then dropped. No GCPTrafficExtension is installed.
  - Customer traffic is unaffected (the missing extension is a silent pass-through at the LB layer).
  - This is NOT a silent no-op: failures are observable via events and cluster-agent logs.

# Event Recording

The package records Kubernetes events for important operations:

  - EnvoyExtensionPolicy creation/deletion
  - ReferenceGrant creation/update/deletion
  - Namespace additions/removals from grants
  - Failure conditions

Events are recorded on the relevant resources (Gateway, EnvoyExtensionPolicy) to provide
operational visibility and debugging information.

# Leader Election

When leader election is enabled in the Cluster Agent, only the leader instance processes
injection events. This prevents duplicate resource creation and ensures consistent state.

# Health Checks

Each proxy-specific controller registers a health check (e.g., "appsec-injector-envoy-gateway")
that is monitored by the Cluster Agent's health probe system.

# Telemetry

The package emits telemetry metrics:

  - appsec_injector.watched_changes: Counter tracking add/modify/delete operations
    Tags: proxy_type, operation, success

  - appsec_injector.sidecar_mutations: Counter tracking sidecar injection outcomes on the
    CREATE (MutatePod) admission path, emitted once per admission for a pod owned by a sidecar
    pattern. Not emitted on DELETE (pod deletion is a no-op).
    Tags: proxy_type, outcome, reason

# Configuration Validation

The package performs comprehensive validation of configuration at startup:

## SIDECAR Mode Validation

Required fields:
  - sidecar.image: Container image for the processor (must be non-empty)
  - sidecar.port: Processor port (must be 1-65535)
  - sidecar.health_port: Health check port (must be 1-65535, different from sidecar.port)

Validation rules:
  - Port values must be in valid range (1-65535)
  - Port and health_port cannot be identical
  - All errors are collected and reported together using errors.Join()

## EXTERNAL Mode Validation

Required fields:
  - processor.service.name: Kubernetes service name (required)
  - processor.service.namespace: Service namespace (optional, defaults to cluster-agent namespace)
  - processor.port: Service port (required, defaults from appsec.proxy.processor.port)

Invalid configurations are logged with ERROR level but do not prevent startup, allowing
administrators to fix configuration while the cluster-agent runs.

## ingress-nginx Configuration

	appsec:
	  proxy:
	    enabled: true                           # Enable AppSec proxy integration
	    auto_detect: true                       # Auto-detect proxies in cluster (detects IngressClass)
	    proxies: ["ingress-nginx"]              # Explicitly enabled proxies
	admission_controller:
	  appsec:
	    nginx:
	      init_image: "datadog/ingress-nginx-injection"  # Init container image (required)
	      module_mount_path: "/modules_mount"            # Module mount path (optional, default: /modules_mount)

ingress-nginx injection is fundamentally different from Istio/Envoy Gateway. Instead of
configuring an external processing (ext_proc) gRPC service, it loads a native nginx module
(ngx_http_datadog_module.so) via a ConfigMap redirect approach:

 1. Creates a DD-owned ConfigMap that mirrors the original ingress-nginx ConfigMap
 2. Prepends load_module and AppSec directives to main-snippet and http-snippet
 3. Mutates controller pods to add an init container that copies the .so module
 4. Redirects the controller's --configmap arg to the DD-owned ConfigMap
 5. Sets an ownerReference on the DD ConfigMap pointing to the original for garbage collection

The DD ConfigMap uses comment markers (# datadog-appsec-begin / # datadog-appsec-end) to
delimit injected directives, enabling idempotent updates and clean removal.

# Error Handling

The package implements robust error handling with:

  - Exponential backoff retry for transient failures
  - Maximum retry limits (default: 5 attempts)
  - Detailed error logging with context
  - Event recording for operational awareness
  - Configuration validation at startup with detailed error messages

Failed operations are requeued with exponential backoff, and after exceeding retry limits,
errors are logged but do not crash the controller.

In SIDECAR mode, webhook failures are handled gracefully:
  - Invalid configurations prevent webhook registration
  - Injection failures are logged and reported via admission response
  - Sidecar already exists: Skip injection (idempotent)
  - Pattern mismatch: Continue to next pattern

# Future Extensibility

The package is designed to support additional proxy types through the InjectionPattern interface.
New proxy implementations should:

 1. Implement the InjectionPattern interface in a dedicated subpackage
 2. Define a ProxyType constant in the config package
 3. Register ProxyConstructor and ProxyDetector in the proxy maps
 4. Add appropriate configuration options

# Related Packages

  - pkg/clusteragent/appsec/config: Configuration types, parsing, and validation
  - pkg/clusteragent/appsec/istio: Istio implementation (EXTERNAL and SIDECAR modes)
  - pkg/clusteragent/appsec/envoygateway: Envoy Gateway implementation (EXTERNAL and SIDECAR modes)
  - pkg/clusteragent/appsec/nginx: ingress-nginx implementation (SIDECAR mode only, native module injection)
  - pkg/clusteragent/appsec/sidecar: Sidecar container generation logic (ext_proc only, not used by nginx)
  - pkg/clusteragent/admission/mutate/appsec: SIDECAR mode admission webhook
  - pkg/clusteragent/admission: Base admission controller functionality

# References

For deployment examples and user documentation, see:
  - https://docs.datadoghq.com/security/application_security/
  - Gateway API specification: https://gateway-api.sigs.k8s.io/
  - Istio External Authorization: https://istio.io/latest/docs/tasks/security/authorization/authz-custom/
  - Envoy External Processing: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_proc_filter
*/
package appsec
