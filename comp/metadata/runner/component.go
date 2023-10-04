// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package runner implements a component to generate metadata payload at the right interval.
package runner

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: agent-shared-components

// Component is the component type.
type Component interface{}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newRunner),
)

// The runner component doesn't provides a mock since other component don't use it directly.
