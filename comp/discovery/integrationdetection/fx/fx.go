// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the integration detection component.
package fx

import (
	integrationdetectiondef "github.com/DataDog/datadog-agent/comp/discovery/integrationdetection/def"
	integrationdetectionimpl "github.com/DataDog/datadog-agent/comp/discovery/integrationdetection/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module returns the Fx options for the integration detection component.
// It provides an optional integrationdetectiondef.Component, present only when
// discovery.integration_detection.enabled is true in the agent configuration.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			integrationdetectionimpl.NewComponent,
		),
		fxutil.ProvideOptional[integrationdetectiondef.Component](),
	)
}
