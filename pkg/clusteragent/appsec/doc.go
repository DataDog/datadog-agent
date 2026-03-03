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

The controller watches proxy resources using Kubernetes informers and maintains a work queue for
processing add/modify/delete events with exponential backoff retry logic.

# Supported Proxies

Currently supported proxy types:

- Istio (ProxyTypeIstio): Configures EnvoyFilter resources for traffic routing
  - EXTERNAL mode: Routes to external processor service
  - SIDECAR mode: Routes to localhost sidecar, injects processor container into gateway pods

- Envoy Gateway (ProxyTypeEnvoyGateway): Configures EnvoyExtensionPolicy resources
  - EXTERNAL mode: Routes to external processor service
  - SIDECAR mode: Currently unsupported due to Kubernetes validation constraints on localhost references

Each proxy type implements the InjectionPattern interface, providing:
  - Resource detection (IsInjectionPossible)
  - Resource watching (Resource, Namespace)
  - Lifecycle management (Added, Modified, Deleted)
  - Mode detection (Mode)

In SIDECAR mode, proxy types additionally implement the SidecarInjectionPattern interface:
  - Pod selection (PodSelector)
  - Sidecar injection (InjectSidecar)
  - Cleanup handling (SidecarDeleted)

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
  - Pod DELETE operations: Trigger cleanup of proxy configuration resources

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
5. **Lazy Config Creation**: On first injection, create proxy configuration (EnvoyFilter/EnvoyExtensionPolicy)
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
	Pod created → InjectSidecar() → Create config (first time only)
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

When a gateway pod with an injected sidecar is deleted:

1. **Pod Deletion Event**: API server invokes webhook for DELETE operation
2. **Remaining Pod Count**: Check if other pods in namespace still have sidecars
3. **Last Pod Logic**: If this is the last pod with sidecar in namespace:
  - Delete proxy configuration resources (EnvoyFilter/EnvoyExtensionPolicy)
  - Remove namespace from ReferenceGrant (if applicable)

4. **Resource Cleanup**: Kubernetes garbage collection handles pod-owned resources

This ensures configuration is cleaned up when no longer needed while avoiding premature
deletion when multiple gateway pods exist.

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
  - Creates config during InjectSidecar() by calling embedded pattern
  - Handles cleanup via SidecarDeleted() when last pod is removed

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
 5. Cleans up EnvoyFilter when last pod in namespace is deleted

Pod selection criteria:
  - Pods must have label: gateway.networking.k8s.io/gateway-name (set by Istio gateway controller)
  - GatewayClass must have controller: istio.io/gateway-controller

## Envoy Gateway Implementation (EXTERNAL Mode Only)

For Envoy Gateway in EXTERNAL mode:
 1. Watches Gateway resources (gateway.networking.k8s.io/v1)
 2. Creates EnvoyExtensionPolicy resources that configure External Processing
 3. Manages ReferenceGrant resources for cross-namespace access
 4. Handles cleanup when gateways are deleted

The implementation ensures that:
  - Only one EnvoyExtensionPolicy is created per namespace
  - ReferenceGrants are properly managed for cross-namespace service references
  - Resources are labeled and annotated for tracking and management
  - Kubernetes events are emitted for operational visibility

SIDECAR mode for Envoy Gateway is currently blocked due to Kubernetes validation preventing
localhost references in ExtProc BackendRef configuration. See pkg/clusteragent/appsec/envoygateway/envoy_sidecar.go
for details.

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
  - pkg/clusteragent/appsec/envoygateway: Envoy Gateway implementation (EXTERNAL mode only)
  - pkg/clusteragent/appsec/sidecar: Sidecar container generation logic
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
