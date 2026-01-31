// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package module provides common types for creating system-probe module components
package module

import (
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

type Component struct {
	Factory  *module.Factory
	CreateFn func() (types.SystemProbeModule, error)
}

func (m *Component) Name() sysconfigtypes.ModuleName {
	return m.Factory.Name
}

func (m *Component) ConfigNamespaces() []string {
	return m.Factory.ConfigNamespaces
}

func (m *Component) Create() (types.SystemProbeModule, error) {
	return m.CreateFn()
}

func (m *Component) NeedsEBPF() bool {
	return m.Factory.NeedsEBPF()
}

func (m *Component) OptionalEBPF() bool {
	return m.Factory.OptionalEBPF
}
