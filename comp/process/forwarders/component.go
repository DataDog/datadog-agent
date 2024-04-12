// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package forwarders implements a component to provide forwarders used by the process agent.
package forwarders

import (
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
)

// team: processes

//nolint:revive // TODO(PROC) Fix revive linter
type Component interface {
	GetEventPlatformForwarder() eventplatform.Component
	GetEventForwarder() defaultforwarder.Component
	GetProcessForwarder() defaultforwarder.Component
	GetRTProcessForwarder() defaultforwarder.Component
	GetConnectionsForwarder() defaultforwarder.Component
}
