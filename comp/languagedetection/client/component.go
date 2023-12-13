// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// team: container-integrations

// Package client implements a component to send process metadata to the Cluster-Agent
package client

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: container-integrations

// Component is the component interface.
type Component interface{}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newClient),
)
