// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package metrics

import "github.com/DataDog/datadog-agent/pkg/telemetry"

const (
	SecretControllerName   = "secrets"
	WebhooksControllerName = "webhooks"
	TagsMutationType       = "standard_tags"
	ConfigMutationType     = "agent_config"
)

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
		[]string{"mutation_type", "injected"}, "Number of pod mutation attempts by mutation type (agent config, standard tags).",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	MutationErrors = telemetry.NewGaugeWithOpts("admission_webhooks", "mutation_errors",
		[]string{"mutation_type", "reason"}, "Number of mutation failures by mutation type (agent config, standard tags).",
		telemetry.Options{NoDoubleUnderscoreSep: true})
	WebhooksReceived = telemetry.NewGaugeWithOpts("admission_webhooks", "webhooks_received",
		[]string{}, "Number of mutation webhook requests received.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
)
