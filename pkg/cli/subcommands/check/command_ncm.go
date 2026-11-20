// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build ncm

package check

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	networkconfigmanagementfx "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/fx"
	ncm "github.com/DataDog/datadog-agent/pkg/collector/corechecks/networkconfigmanagement"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const ncmRollbackEnabledKey = "network_devices.config_management.rollback.enabled"

func getNCMModule(args []string) fx.Option {
	if len(args) > 0 && args[0] == ncm.CheckName {
		// A one-off run never opens the rollback database: holding its lock while the Agent starts
		// would leave the Agent without rollback until it restarts. The override is scoped to this
		// module, so nothing else sees it.
		return fx.Module("ncm-check-cli",
			networkconfigmanagementfx.Module(),
			fx.Decorate(func(c config.Component) config.Component {
				return noRollbackConfig{c}
			}),
		)
	}
	// Other checks must not open or contend for the NCM rollback database.
	return fx.Supply(option.None[networkconfigmanagement.Component]())
}

// noRollbackConfig reports NCM rollback as disabled and leaves every other setting unchanged.
type noRollbackConfig struct {
	config.Component
}

func (c noRollbackConfig) GetBool(key string) bool {
	if key == ncmRollbackEnabledKey {
		return false
	}
	return c.Component.GetBool(key)
}
