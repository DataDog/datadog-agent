// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package forwardersimpl implements a component to provide forwarders used by the process agent.
package forwardersimpl

import (
	forwarders "github.com/DataDog/datadog-agent/comp/process/forwarders/def"
)

// NewMockForwarders creates a mock forwarders component for testing by using the real implementation.
func NewMockForwarders(deps dependencies) (forwarders.Component, error) {
	return NewComponent(deps)
}
