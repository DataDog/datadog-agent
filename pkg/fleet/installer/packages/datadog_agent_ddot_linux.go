// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var datadogAgentDdotPackage = hooks{
	postInstall: postInstallDatadogAgentDdot,
	preRemove:   preRemoveDatadogAgentDdot,
}

var (
	ddotConfigPermissions = file.Permissions{
		{Path: "otel-config.yaml", Owner: "dd-agent", Group: "dd-agent", Mode: 0644},
	}

	// ddotPackagePermissions are the ownerships and modes that are enforced on the DDOT package files
	ddotPackagePermissions = file.Permissions{
		{Path: ".", Owner: "dd-agent", Group: "dd-agent", Recursive: true},
	}

	ddotConfigUninstallPaths = file.Paths{
		"otel-config.yaml.example",
	}
)

// postInstallDatadogAgentDdot performs post-installation steps for the DDOT package
func postInstallDatadogAgentDdot(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("setup_ddot_filesystem")
	defer func() {
		span.Finish(err)
	}()

	// Ensure the dd-agent user and group exist
	if err = user.EnsureAgentUserAndGroup(ctx, "/opt/datadog-agent"); err != nil {
		return fmt.Errorf("failed to create dd-agent user and group: %v", err)
	}

	// Set DDOT package permissions
	if err = ddotPackagePermissions.Ensure(ctx.PackagePath); err != nil {
		return fmt.Errorf("failed to set DDOT package ownerships: %v", err)
	}

	// Set DDOT config permissions
	if err = ddotConfigPermissions.Ensure("/etc/datadog-agent"); err != nil {
		return fmt.Errorf("failed to set DDOT config ownerships: %v", err)
	}

	return nil
}

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
