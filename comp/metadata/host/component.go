// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package host implements a component to generate the 'host' metadata payload (also known as "v5").
package host

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: agent-shared-components

// Component is the component type.
type Component interface {
	GetPayloadAsJSON(ctx context.Context) ([]byte, error)
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newHostProvider),
)

// The runner component doesn't provides a mock since other component don't use it directly.
