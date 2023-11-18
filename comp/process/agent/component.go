// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent contains a process-agent component
package agent

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: processes

// Component represents the no-op Component interface.
type Component interface {
	Start(context.Context) error
	Stop(context.Context) error
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newProcessAgent),
)
