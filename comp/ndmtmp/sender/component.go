// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package sender exposes a Sender for netflow.
package sender

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: network-device-monitoring

// Component is the component type.
type Component interface {
	sender.Sender
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(getDefaultSender),
)
