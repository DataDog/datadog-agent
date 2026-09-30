// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !nodefilter

// Package nodefilter implements a workloadmeta Collector that watches pods
// scoped to the local node directly against the Kubernetes API server. This
// file is used when the nodefilter build tag is disabled.
package nodefilter

import "go.uber.org/fx"

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return nil
}
