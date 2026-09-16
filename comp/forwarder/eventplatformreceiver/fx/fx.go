// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the eventplatformreceiver component.
package fx

import (
	eventplatformreceiverimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-log-pipelines

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(eventplatformreceiverimpl.NewComponent),
	)
}
