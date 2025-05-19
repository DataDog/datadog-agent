// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextensionfx provides fx access for the provider component
package ddprofilingextensionfx

import (
	ddprofilingextension "github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/def"
	ddprofilingextensionimpl "github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: opentelemetry-agent

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(ddprofilingextensionimpl.NewExtension),
		fxutil.ProvideOptional[ddprofilingextension.Component](),
	)
}
