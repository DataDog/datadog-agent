// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package appsec

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
)

var sidecarMutationsCounter = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
	"appsec_injector",
	"sidecar_mutations",
	[]string{"proxy_type", "outcome", "reason"},
	"Tracks appsec injector sidecar MutatePod admission outcomes",
	telemetry.DefaultOptions,
)

func outcomeString(o appsecconfig.MutationOutcome) string {
	switch o {
	case appsecconfig.MutationMutated:
		return "mutated"
	case appsecconfig.MutationSkipped:
		return "skipped"
	case appsecconfig.MutationError:
		return "error"
	default:
		return "error"
	}
}
