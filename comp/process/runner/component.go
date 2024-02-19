// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package runner implements a component to run data collection checks in the Process Agent.
package runner

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
)

// team: processes

// Component is the component type.
type Component interface {
	GetChecks() []checks.Check
	GetProvidedChecks() []types.CheckComponent
	Run(ctx context.Context) error
	Stop(ctx context.Context) error
}
