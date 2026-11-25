// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package inventory

import "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"

// CollectorsForTest returns every collector of the inventory, in every version.
func (ci *CollectorInventory) CollectorsForTest() []collectors.K8sCollector {
	var all []collectors.K8sCollector
	for _, cv := range ci.collectors {
		all = append(all, cv.Collectors...)
	}
	return all
}
