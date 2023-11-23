// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package forwarder implements the orchestrator forwarder component.
package forwarder

import "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"

// team: agent-shared-components

// Component is the component type.
type Component interface {
	Get() (defaultforwarder.Forwarder, bool)

	// TODO: (components): This function is used to know if Stop was already called.
	// Remove it when Stop is not part of this interface.
	Reset()
}
