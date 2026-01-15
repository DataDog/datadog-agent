// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package modules

import (
	privilegedlogsmodule "github.com/DataDog/datadog-agent/pkg/privileged-logs/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

func init() { registerModule(PrivilegedLogs) }

// PrivilegedLogs is a module that provides privileged logs access capabilities
var PrivilegedLogs = &module.Factory{
	Name:             config.PrivilegedLogsModule,
	ConfigNamespaces: []string{},
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		return privilegedlogsmodule.NewPrivilegedLogsModule(), nil
	},
	NeedsEBPF: func() bool {
		return false
	},
}
