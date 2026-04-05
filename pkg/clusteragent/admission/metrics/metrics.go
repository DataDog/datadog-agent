// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package metrics defines the telemetry of the Admission Controller.
package metrics

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"

	"github.com/prometheus/client_golang/prometheus"
)

// Metric names
const (
	SecretControllerName   = "secrets"
	WebhooksControllerName = "webhooks"
)

// Mutation errors
const (
	InvalidInput         = "invalid_input"
	InternalError        = "internal_error"
	ConfigInjectionError = "config_injection_error"
)

// Status tags
const (
	StatusSuccess = "success"
	StatusError   = "error"
)

// Image resolution capability tags for operational context
const (
	DigestResolutionEnabled  = "enabled"  // Digest resolution available (rollout active)
	DigestResolutionDisabled = "disabled" // Digest resolution unavailable (fallback expected)
)

// Telemetry metrics
var (
	ReconcileSuccess = telemetryimpl.GetCompatComponent().NewGaugeWithOpts("admission_webhooks", "reconcile_success",
		[]string{"controller"}, "Number of reconcile success per controller.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	ReconcileErrors = telemetryimpl.GetCompatComponent().NewGaugeWithOpts("admission_webhooks", "reconcile_errors",
		[]string{"controller"}, "Number of reconcile errors per controller.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	CertificateDuration = telemetryimpl.GetCompatComponent().NewGaugeWithOpts("admission_webhooks", "certificate_expiry",
		[]string{}, "Time left before the certificate expires in hours.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	ValidationAttempts = telemetryimpl.GetCompatComponent().NewGaugeWithOpts("admission_webhooks", "validation_attempts",
		[]string{"webhook_name", "status", "validated", "error"}, "Number of pod validation attempts by validation type",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	MutationAttempts = telemetryimpl.GetCompatComponent().NewGaugeWithOpts("admission_webhooks", "mutation_attempts",
		[]string{"mutation_type", "status", "injected", "error"}, "Number of pod mutation attempts by mutation type",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	WebhooksReceived = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"admission_webhooks",
		"webhooks_received",
		[]string{"mutation_type", "webhook_name", "webhook_type"},
		"Number of webhook requests received.",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
	GetOwnerCacheHit = telemetryimpl.GetCompatComponent().NewGaugeWithOpts("admission_webhooks", "owner_cache_hit",
		[]string{"resource"}, "Number of cache hits while getting pod's owner object.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	GetOwnerCacheMiss = telemetryimpl.GetCompatComponent().NewGaugeWithOpts("admission_webhooks", "owner_cache_miss",
		[]string{"resource"}, "Number of cache misses while getting pod's owner object.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	WebhooksResponseDuration = telemetryimpl.GetCompatComponent().NewHistogramWithOpts(
		"admission_webhooks",
		"response_duration",
		[]string{"mutation_type", "webhook_name", "webhook_type"},
		"Webhook response duration distribution (in seconds).",
		prometheus.DefBuckets, // The default prometheus buckets are adapted to measure response time
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
	LibInjectionAttempts = telemetryimpl.GetCompatComponent().NewCounterWithOpts("admission_webhooks", "library_injection_attempts",
		[]string{"language", "injected", "auto_detected", "injection_type"}, "Number of pod library injection attempts by language and injection type",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	LibInjectionErrors = telemetryimpl.GetCompatComponent().NewCounterWithOpts("admission_webhooks", "library_injection_errors",
		[]string{"language", "auto_detected", "injection_type"}, "Number of library injection failures by language and injection type",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	CWSPodMutationAttempts = telemetryimpl.GetCompatComponent().NewCounterWithOpts("admission_webhooks", "cws_pod_mutation_attempts",
		[]string{"mode", "injected", "reason"}, "Count of pod mutation attempts per CWS instrumentation mode",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	CWSExecMutationAttempts = telemetryimpl.GetCompatComponent().NewCounterWithOpts("admission_webhooks", "cws_exec_mutation_attempts",
		[]string{"mode", "injected", "reason"}, "Count of exec mutation attempts per CWS instrumentation mode",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	CWSResponseDuration = telemetryimpl.GetCompatComponent().NewHistogramWithOpts(
		"admission_webhooks",
		"cws_response_duration",
		[]string{"mode", "webhook_name", "type", "success", "injected"},
		"CWS Webhook response duration distribution (in seconds).",
		prometheus.DefBuckets, // The default prometheus buckets are adapted to measure response time
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
	RemoteConfigs = telemetryimpl.GetCompatComponent().NewGaugeWithOpts("admission_webhooks", "rc_provider_configs",
		[]string{}, "Number of valid remote configurations.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	InvalidRemoteConfigs = telemetryimpl.GetCompatComponent().NewGaugeWithOpts("admission_webhooks", "rc_provider_configs_invalid",
		[]string{}, "Number of invalid remote configurations.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	PatchAttempts = telemetryimpl.GetCompatComponent().NewCounterWithOpts("admission_webhooks", "patcher_attempts",
		[]string{}, "Number of patch attempts.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	PatchCompleted = telemetryimpl.GetCompatComponent().NewCounterWithOpts("admission_webhooks", "patcher_completed",
		[]string{}, "Number of completed patch attempts.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	PatchErrors = telemetryimpl.GetCompatComponent().NewCounterWithOpts("admission_webhooks", "patcher_errors",
		[]string{}, "Number of patch errors.",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	// Image resolution tracking for gradual rollout monitoring
	ImageResolutionAttempts = telemetryimpl.GetCompatComponent().NewCounterWithOpts("admission_webhooks", "image_resolution_attempts",
		[]string{"repository", "tag", "bucket", "outcome"}, "Number of image resolution attempts by repository, tag, bucket, and resolution outcome",
		telemetry.Options{})
)
