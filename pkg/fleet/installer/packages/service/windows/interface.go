// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package windows provides a set of functions to manage Windows services.
package windows

import (
	"context"
)

// ServiceManager interface abstracts all service management operations
//
// Could generalize for arbitrary services later, but we only need the Agent services for now.
type ServiceManager interface {
	StopAllAgentServices(ctx context.Context) error
	StartAgentServices(ctx context.Context) error
	RestartAgentServices(ctx context.Context) error
}
