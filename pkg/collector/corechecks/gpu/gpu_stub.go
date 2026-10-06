// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux || !nvml

package gpu

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Readiness is unused when the GPU check is not supported.
type Readiness struct{}

// NewReadiness is a no-op when the GPU check is not supported.
func NewReadiness() *Readiness { return nil }

// Factory creates a new check factory
func Factory(_ tagger.Component, _ telemetry.Component, _ workloadmeta.Component, _ *Readiness) option.Option[func() check.Check] {
	return option.None[func() check.Check]()
}
