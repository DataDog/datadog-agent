// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package forwarder implements a component to send payloads to the backend
package forwarder

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: Agent shared components

// Component is the component type.
type Component interface {
	// TODO: (components) When the code of the forwarder will be
	// in /comp/forwarder move the content of forwarder.Forwarder inside this interface.
	forwarder.Forwarder
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newForwarder),
)
