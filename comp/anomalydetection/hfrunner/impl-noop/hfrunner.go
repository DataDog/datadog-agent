// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package noopimpl provides a no-op hfrunner implementation.
// The full implementation (system/container check runners) ships in a later PR
// once pkg/collector/corechecks/system/* is migrated to Bazel.
package noopimpl

import (
	hfrunner "github.com/DataDog/datadog-agent/comp/anomalydetection/hfrunner/def"
	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// NewNoopComponent returns an hfrunner Component that never starts any runner.
func NewNoopComponent() hfrunner.Component {
	return &noopHFRunner{}
}

type noopHFRunner struct{}

func (n *noopHFRunner) StartSystem(_ observer.Handle) map[metrics.MetricSource]struct{} {
	return nil
}

func (n *noopHFRunner) StartContainer(_ observer.Handle) map[metrics.MetricSource]struct{} {
	return nil
}
