// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var datadogAgentDdotPackage = hooks{
	preRemove: preRemoveDatadogAgentDdot,
}

var (
	ddotConfigUninstallPaths = file.Paths{
		"otel-config.yaml.example",
	}
)

// preRemoveDatadogAgentDdot performs pre-removal steps for the DDOT package
// All the steps are allowed to fail
func preRemoveDatadogAgentDdot(ctx HookContext) error {
	if !ctx.Upgrade {
		// Only remove config files during actual uninstall, not during upgrades
		err := ddotConfigUninstallPaths.EnsureAbsent("/etc/datadog-agent")
		if err != nil {
			log.Warnf("failed to remove DDOT config files: %s", err)
		}
	}

	return nil
}
