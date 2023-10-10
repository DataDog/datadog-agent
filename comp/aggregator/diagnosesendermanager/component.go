// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package diagnosesendermanager defines the sender manager for the local diagnose check
package diagnosesendermanager

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Component is the component type.
type Component interface {
	sender.DiagnoseSenderManager
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newDiagnoseSenderManager),
)
