// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package metrics defines the telemetry of the Admission Controller.
package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"

	"github.com/prometheus/client_golang/prometheus"
)

// Metric names
const (
	SecretControllerName     = "secrets"
	WebhooksControllerName   = "webhooks"
	TagsMutationType         = "standard_tags"
	ConfigMutationType       = "agent_config"
	LibInjectionMutationType = "lib_injection"
	CWSPodInstrumentation    = "cws_pod_instrumentation"
	CWSExecInstrumentation   = "cws_exec_instrumentation"
)

// Telemetry metrics
var (
	ReconcileSuccess = telemetry.NewGaugeWithOpts("admission_webhooks", "reconcile_success",
		[]string{"controller"}, "Number of reconcile success per controller.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	ReconcileErrors = telemetry.NewGaugeWithOpts("admission_webhooks", "reconcile_errors",
		[]string{"controller"}, "Number of reconcile errors per controller.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	CertificateDuration = telemetry.NewGaugeWithOpts("admission_webhooks", "certificate_expiry",
		[]string{}, "Time left before the certificate expires in hours.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	MutationAttempts = telemetry.NewGaugeWithOpts("admission_webhooks", "mutation_attempts",
		[]string{"mutation_type", "injected", "language", "auto_detected"}, "Number of pod mutation attempts by mutation type (agent config, standard tags, lib injection).",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	MutationErrors = telemetry.NewGaugeWithOpts("admission_webhooks", "mutation_errors",
		[]string{"mutation_type", "reason", "language", "auto_detected"}, "Number of mutation failures by mutation type (agent config, standard tags, lib injection).",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	WebhooksReceived = telemetry.NewCounterWithOpts("admission_webhooks", "webhooks_received",
		[]string{}, "Number of mutation webhook requests received.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	GetOwnerCacheHit = telemetry.NewGaugeWithOpts("admission_webhooks", "owner_cache_hit",
		[]string{"resource"}, "Number of cache hits while getting pod's owner object.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	GetOwnerCacheMiss = telemetry.NewGaugeWithOpts("admission_webhooks", "owner_cache_miss",
		[]string{"resource"}, "Number of cache misses while getting pod's owner object.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	WebhooksResponseDuration = telemetry.NewHistogramWithOpts(
		"admission_webhooks",
		"response_duration",
		[]string{},
		"Webhook response duration distribution (in seconds).",
		prometheus.DefBuckets, // The default prometheus buckets are adapted to measure response time
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
	LibInjectionAttempts = telemetry.NewCounterWithOpts("admission_webhooks", "library_injection_attempts",
		[]string{"language", "injected", "auto_detected", "injection_type"}, "Number of pod library injection attempts by language and injection type",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	LibInjectionErrors = telemetry.NewCounterWithOpts("admission_webhooks", "library_injection_errors",
		[]string{"language", "auto_detected", "injection_type"}, "Number of library injection failures by language and injection type",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	RemoteConfigs = telemetry.NewGaugeWithOpts("admission_webhooks", "rc_provider_configs",
		[]string{}, "Number of valid remote configurations.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	InvalidRemoteConfigs = telemetry.NewGaugeWithOpts("admission_webhooks", "rc_provider_configs_invalid",
		[]string{}, "Number of invalid remote configurations.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	PatchAttempts = telemetry.NewCounterWithOpts("admission_webhooks", "patcher_attempts",
		[]string{}, "Number of patch attempts.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	PatchCompleted = telemetry.NewCounterWithOpts("admission_webhooks", "patcher_completed",
		[]string{}, "Number of completed patch attempts.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	PatchErrors = telemetry.NewCounterWithOpts("admission_webhooks", "patcher_errors",
		[]string{}, "Number of patch errors.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	TLSHandshakeErrors = telemetry.NewCounterWithOpts("admission_webhooks", "tls_handshake_errors",
		[]string{}, "Number of tls handshake errors.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
)
