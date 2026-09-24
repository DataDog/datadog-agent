// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package forwarderimpl implements the health platform forwarder component.
package forwarderimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/client"
	forwarderdef "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/def"
)

// Requires defines the dependencies for the forwarder.
type Requires struct {
	Config config.Component
}

// NewComponent creates a stateless sender with no lifecycle hooks.
func NewComponent(reqs Requires) forwarderdef.Component {
	return client.New(reqs.Config)
}
