// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package eventmonitor ... /* TODO: detailed doc comment for the component */
package eventmonitor

import (
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers"
)

// team: agent-security

// Component is the component type.
type Component interface {
	types.SystemProbeModuleComponent
}

type ProcessEventConsumerComponent interface {
	ID() string
	ChanSize() int
	EventTypes() []consumers.ProcessConsumerEventTypes
	Set(*consumers.ProcessConsumer)
	Get() *consumers.ProcessConsumer
}
