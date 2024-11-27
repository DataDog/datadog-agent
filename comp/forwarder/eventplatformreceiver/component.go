// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package eventplatformreceiver implements the receiver for the event platform package
package eventplatformreceiver

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// team: agent-processing-and-routing

// Component is the component type.
type Component interface {
	SetEnabled(e bool) bool
	IsEnabled() bool
	HandleMessage(m *message.Message, rendered []byte, eventType string)
	Filter(filters *diagnostic.Filters, done <-chan struct{}) <-chan string
}

// NoneModule return a None optional type for authtoken.Component.
//
// This helper allows code that needs a disabled Optional type for authtoken to get it. The helper is split from
// the implementation to avoid linking with the dependencies from sysprobeconfig.
func NoneModule() fxutil.Module {
	return fxutil.Component(fx.Provide(func() optional.Option[Component] {
		return optional.NewNoneOption[Component]()
	}))
}
