// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
)

// getKubeletFilter returns a ContainerFilter for the kubelet check.
// Unlike docker/containerd/cri checks which filter by runtime, kubelet manages
// all container runtimes. Only the legacy container exclusion filter is applied.
func getKubeletFilter(filterStore workloadfilter.Component, store workloadmeta.Component) generic.ContainerFilter {
	return generic.LegacyContainerFilter{
		ContainerFilter: filterStore.GetContainerSharedMetricFilters(),
		Store:           store,
	}
}
