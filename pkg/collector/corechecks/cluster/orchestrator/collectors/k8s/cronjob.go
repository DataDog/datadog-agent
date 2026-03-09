// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
)

// NewCronJobCollectorVersions builds the group of collector versions for
func NewCronJobCollectorVersions() collectors.CollectorVersions {
	return collectors.NewCollectorVersions(
		NewCronJobV1Collector(),
		NewCronJobV1Beta1Collector(),
	)
}
