// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver || !kubelet

// Package kubemetadata implements the kube_metadata Workloadmeta collector.
package kubemetadata

import (
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/wmcatalog/def"
)

// NewCollector is a no-op constructor
func NewCollector() (wmcatalog.Collector, error) {
	return nil, nil
}
