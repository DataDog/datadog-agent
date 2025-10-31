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

  - Envoy Gateway (ProxyTypeEnvoyGateway): Automatically configures EnvoyExtensionPolicy resources
    to route traffic through the AppSec external processor

Each proxy type implements the InjectionPattern interface, providing:
  - Resource detection (IsInjectionPossible)
  - Resource watching (Resource, Namespace)
  - Lifecycle management (Added, Modified, Deleted)

# Configuration

The package is configured through the Datadog Cluster Agent configuration:

	appsec:
	  proxy:
	    enabled: true                           # Enable AppSec proxy integration
	    auto_detect: true                       # Auto-detect proxies in cluster
	    proxies: ["envoy-gateway"]              # Explicitly enabled proxies
	    processor:
	      address: "extproc-service.ns.svc"     # Processor service address
	      port: 443                             # Processor service port
	cluster_agent:
	  appsec:
	    injector:
	      enabled: true                         # Enable automatic injection
	      labels: {}                            # Additional labels for created resources
	      annotations: {}                       # Additional annotations
	      processor:
	        service:
	          name: "extproc-service"           # Service name
	          namespace: "datadog"              # Service namespace

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

# Envoy Gateway Implementation

For Envoy Gateway, the injector:

 1. Watches Gateway resources (gateway.networking.k8s.io/v1)
 2. Creates EnvoyExtensionPolicy resources that configure External Processing
 3. Manages ReferenceGrant resources for cross-namespace access
 4. Handles cleanup when gateways are deleted

The implementation ensures that:
  - Only one EnvoyExtensionPolicy is created per namespace
  - ReferenceGrants are properly managed for cross-namespace service references
  - Resources are labeled and annotated for tracking and management
  - Kubernetes events are emitted for operational visibility

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

# Error Handling

The package implements robust error handling with:

  - Exponential backoff retry for transient failures
  - Maximum retry limits (default: 5 attempts)
  - Detailed error logging with context
  - Event recording for operational awareness

Failed operations are requeued with exponential backoff, and after exceeding retry limits,
errors are logged but do not crash the controller.

# Future Extensibility

The package is designed to support additional proxy types through the InjectionPattern interface.
New proxy implementations should:

 1. Implement the InjectionPattern interface in a dedicated subpackage
 2. Define a ProxyType constant in the config package
 3. Register ProxyConstructor and ProxyDetector in the proxy maps
 4. Add appropriate configuration options

# Related Packages

  - pkg/clusteragent/appsec/config: Configuration types and parsing
  - pkg/clusteragent/appsec/envoygateway: Envoy Gateway implementation
  - pkg/clusteragent/admission: Related admission controller functionality

# References

For deployment examples and user documentation, see:
  - https://docs.datadoghq.com/security/application_security/
*/
package appsec
