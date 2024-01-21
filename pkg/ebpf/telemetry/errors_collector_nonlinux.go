// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf

package telemetry

import "github.com/prometheus/client_golang/prometheus"

// NewEbpfErrorsCollector initializes a new Collector object for ebpf helper and map operations errors.
// Not supported on Windows, thus returning noop collector instead.
func NewEbpfErrorsCollector() prometheus.Collector {
	return &NoopEbpfErrorsCollector{}
}
