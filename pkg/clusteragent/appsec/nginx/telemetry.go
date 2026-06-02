// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package nginx

import (
	"errors"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

const (
	mutationSkippedReasonCrossNamespaceConfigMap = "cross_namespace_configmap"
	mutationSkippedReasonEmptyConfigMapName      = "empty_configmap_name"
	mutationSkippedReasonOther                   = "other"
)

// mutationSkippedCounter counts admission webhook decisions to refuse nginx
// AppSec mutation because the pod's --configmap arg was malformed or pointed
// to a foreign namespace. This enables monitoring/alerting on suspicious
// activity at scale, since Kubernetes events are noisy and have short retention.
//
// Tags are bounded to a small enum (reason) to avoid cardinality blow-up.
// pod_namespace is deliberately not tagged: a malicious tenant controls it
// and could explode the metric series.
var mutationSkippedCounter = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
	"appsec_injector",
	"nginx_mutation_skipped",
	[]string{"reason"},
	"Number of times nginx AppSec pod mutation was skipped due to cross-namespace or malformed --configmap args",
	telemetry.DefaultOptions,
)

// recordMutationSkipped increments the mutation_skipped counter with a tag
// derived from the rejection reason. Unknown errors are bucketed under
// "other" to avoid silent drops.
func recordMutationSkipped(err error) {
	mutationSkippedCounter.Inc(mutationSkippedReason(err))
}

func mutationSkippedReason(err error) string {
	switch {
	case errors.Is(err, errCrossNamespaceConfigMap):
		return mutationSkippedReasonCrossNamespaceConfigMap
	case errors.Is(err, errEmptyConfigMapName):
		return mutationSkippedReasonEmptyConfigMapName
	default:
		return mutationSkippedReasonOther
	}
}
