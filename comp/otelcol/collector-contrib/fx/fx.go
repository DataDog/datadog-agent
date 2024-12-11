// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collectorcontribfx provides fx access for the collectorcontrib component
package collectorcontribfx

import (
	collectorcontrib "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def"
	collectorcontribimpl "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module for collector contrib component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			collectorcontribimpl.NewComponent,
		),
		fxutil.ProvideOptional[collectorcontrib.Component](),
	)
}
