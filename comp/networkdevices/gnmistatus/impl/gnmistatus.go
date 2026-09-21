// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package gnmistatusimpl implements the gnmistatus component interface.
package gnmistatusimpl

import (
	corestatus "github.com/DataDog/datadog-agent/comp/core/status"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	gnmistatus "github.com/DataDog/datadog-agent/comp/networkdevices/gnmistatus/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/status"
)

// Requires defines the dependencies for the gnmistatus component.
type Requires struct {
	compdef.In
}

// Provides defines the output of the gnmistatus component.
type Provides struct {
	compdef.Out

	Comp           gnmistatus.Component
	StatusProvider corestatus.InformationProvider
}

// NewComponent creates the gNMI status registry component. The registry is
// shared by the Comp and StatusProvider outputs, so devices registered by the
// gNMI check appear in the agent status output.
func NewComponent(_ Requires) Provides {
	registry := status.NewRegistry()
	return Provides{
		Comp:           registry,
		StatusProvider: corestatus.NewInformationProvider(status.NewProvider(registry)),
	}
}
