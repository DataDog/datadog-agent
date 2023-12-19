// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package diagnosesendermanager defines the sender manager for the local diagnose check
package diagnosesendermanager

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

// team: agent-shared-components

// Component is the component type.
// This component must not be used with demultiplexer.Component
// See demultiplexer.provides for more information.
type Component interface {
	sender.DiagnoseSenderManager
}
