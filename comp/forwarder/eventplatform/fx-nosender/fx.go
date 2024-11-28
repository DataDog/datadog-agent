// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fxnosender module for the eventplatform component
package fx

import (
	eventplatfornosender "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/impl-nosender"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the eventplatform component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			eventplatfornosender.NewComponent,
		),
	)
}
