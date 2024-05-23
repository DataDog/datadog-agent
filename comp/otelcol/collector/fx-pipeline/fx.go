// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx creates the modules for fx
package fx

import (
	collectorimpl "github.com/DataDog/datadog-agent/comp/otelcol/collector/impl-pipeline"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: opentelemetry

// Module for OTel Agent
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(collectorimpl.NewComponent))
}
