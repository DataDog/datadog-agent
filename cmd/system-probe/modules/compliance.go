// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build linux

package modules

import (
	"sync/atomic"

	// import the full compliance code in the system-probe (including the rego evaluator)
	// this allows us to reserve the package size while we work on pluging things out
	_ "github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

func init() { registerModule(ComplianceModule) }

// ComplianceModule is a system-probe module that exposes an HTTP api to
// perform compliance checks that require more privileges than security-agent
// can offer.
//
// For instance, being able to run cross-container checks at runtime by directly
// accessing the /proc/<pid>/root mount point.
var ComplianceModule = &module.Factory{
	Name:             config.ComplianceModule,
	ConfigNamespaces: []string{},
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		return &complianceModule{}, nil
	},
	NeedsEBPF: func() bool {
		return false
	},
}

type complianceModule struct {
	performedChecks atomic.Uint64
}

// Close is a noop (implements module.Module)
func (*complianceModule) Close() {
}

// GetStats returns statistics related to the compliance module (implements module.Module)
func (m *complianceModule) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"performed_checks": m.performedChecks.Load(),
	}
}

// Register implements module.Module.
func (m *complianceModule) Register(_ *module.Router) error {
	return nil
}
