// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/packagemanager"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var datadogAgentDDOTPackage = hooks{
	preInstall:  preInstallDatadogAgentDDOT,
	postInstall: postInstallDatadogAgentDdot,
	preRemove:   preRemoveDatadogAgentDdot,
}

const (
	agentDDOTPackage = "datadog-agent-ddot"
	datadogYamlPath  = "/etc/datadog-agent/datadog.yaml"
	datadogEnvPath   = "/etc/datadog-agent/environment"
)

var (
	// ddotConfigPermissions are the ownerships and modes that are enforced on the DDOT configuration files
	ddotConfigPermissions = file.Permissions{
		{Path: "otel-config.yaml.example", Owner: "dd-agent", Group: "dd-agent", Mode: 0644},
		{Path: "otel-config.yaml", Owner: "dd-agent", Group: "dd-agent", Mode: 0644},
	}

	// ddotPackagePermissions are the ownerships and modes that are enforced on the DDOT package files
	ddotPackagePermissions = file.Permissions{
		{Path: ".", Owner: "dd-agent", Group: "dd-agent", Recursive: true},
	}

	// ddotConfigUninstallPaths are the files that are deleted during an uninstall
	ddotConfigUninstallPaths = file.Paths{
		"otel-config.yaml.example",
		"otel-config.yaml",
	}

	// envFilePermissions are the ownerships and modes that are enforced on the environment file
	envFilePermissions = file.Permissions{
		{Path: "environment", Owner: "dd-agent", Group: "dd-agent", Mode: 0640},
	}

	// agentDDOTService are the services that are part of the DDOT package
	agentDDOTService = datadogAgentService{
		SystemdMainUnitStable: "datadog-agent-ddot.service",
		SystemdMainUnitExp:    "datadog-agent-ddot-exp.service",
		SystemdUnitsStable:    []string{"datadog-agent-ddot.service"},
		SystemdUnitsExp:       []string{"datadog-agent-ddot-exp.service"},

		UpstartMainService: "datadog-agent-ddot",
		UpstartServices:    []string{"datadog--ddot"},

		SysvinitMainService: "datadog-agent-ddot",
		SysvinitServices:    []string{"datadog-agent-ddot"},
	}
)

// preInstallDatadogAgentDDOT performs pre-installation steps for DDOT
func preInstallDatadogAgentDDOT(ctx HookContext) error {
	if err := agentDDOTService.StopStable(ctx); err != nil {
		log.Warnf("failed to stop stable unit: %s", err)
	}
	if err := agentDDOTService.DisableStable(ctx); err != nil {
		log.Warnf("failed to disable stable unit: %s", err)
	}
	if err := agentDDOTService.RemoveStable(ctx); err != nil {
		log.Warnf("failed to remove stable unit: %s", err)
	}
	return packagemanager.RemovePackage(ctx, agentDDOTPackage)
}

// postInstallDatadogAgentDdot performs post-installation steps for the DDOT package
func postInstallDatadogAgentDdot(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("setup_ddot_filesystem")
	defer func() {
		span.Finish(err)
	}()

	// Copy example config to default path
	if err = paths.CopyFile("/etc/datadog-agent/otel-config.yaml.example", "/etc/datadog-agent/otel-config.yaml"); err != nil {
		return fmt.Errorf("could not copy otel-config.yaml.example file: %s", err)
	}

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

	// Enable otelcollector in environment file
	if err = enableOtelCollectorConfig(); err != nil {
		return fmt.Errorf("failed to enable otelcollector in environment file: %v", err)
	}

	if err := agentDDOTService.WriteStable(ctx); err != nil {
		return fmt.Errorf("failed to write stable units: %s", err)
	}
	if err := agentDDOTService.EnableStable(ctx); err != nil {
		return fmt.Errorf("failed to install stable unit: %s", err)
	}
	if err := agentDDOTService.RestartStable(ctx); err != nil {
		return fmt.Errorf("failed to restart stable unit: %s", err)
	}

	return nil
}

// preRemoveDatadogAgentDdot performs pre-removal steps for the DDOT package
// All the steps are allowed to fail
func preRemoveDatadogAgentDdot(ctx HookContext) error {
	err := agentDDOTService.StopExperiment(ctx)
	if err != nil {
		log.Warnf("failed to stop experiment unit: %s", err)
	}
	err = agentDDOTService.RemoveExperiment(ctx)
	if err != nil {
		log.Warnf("failed to remove experiment unit: %s", err)
	}
	err = agentDDOTService.StopStable(ctx)
	if err != nil {
		log.Warnf("failed to stop stable unit: %s", err)
	}
	err = agentDDOTService.DisableStable(ctx)
	if err != nil {
		log.Warnf("failed to disable stable unit: %s", err)
	}
	err = agentDDOTService.RemoveStable(ctx)
	if err != nil {
		log.Warnf("failed to remove stable unit: %s", err)
	}

	if !ctx.Upgrade {
		// Only remove config files during actual uninstall, not during upgrades
		err := ddotConfigUninstallPaths.EnsureAbsent("/etc/datadog-agent")
		if err != nil {
			log.Warnf("failed to remove DDOT config files: %s", err)
		}

		// Disable otelcollector in environment file
		if err = disableOtelCollectorConfig(); err != nil {
			log.Warnf("failed to disable otelcollector in environment file: %s", err)
		}
	}

	return nil
}

// enableOtelCollectorConfig adds DD_OTELCOLLECTOR_ENABLED=true to the environment file
func enableOtelCollectorConfig() error {
	// Read existing environment file if it exists
	var envLines []string
	data, err := os.ReadFile(datadogEnvPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read environment file: %w", err)
	}

	if err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			trimmedLine := strings.TrimSpace(line)

			// Filter out existing OTEL collector settings
			if !strings.HasPrefix(trimmedLine, "DD_OTELCOLLECTOR_ENABLED=") {
				envLines = append(envLines, line)
			}
		}
	}

	envLines = append(envLines, "DD_OTELCOLLECTOR_ENABLED=true")

	// Update file
	envContent := strings.Join(envLines, "\n") + "\n"
	if err := os.WriteFile(datadogEnvPath, []byte(envContent), 0640); err != nil {
		return fmt.Errorf("failed to write environment file: %w", err)
	}

	if err := envFilePermissions.Ensure("/etc/datadog-agent"); err != nil {
		return fmt.Errorf("failed to set ownership on environment file: %w", err)
	}

	return nil
}

// disableOtelCollectorConfig removes DD_OTELCOLLECTOR_ENABLED from the environment file
func disableOtelCollectorConfig() error {
	data, err := os.ReadFile(datadogEnvPath)

	// Nothing to remove if the file doesn't exist
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read environment file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	var filteredLines []string
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Filter out existing OTEL collector settings
		if !strings.HasPrefix(trimmedLine, "DD_OTELCOLLECTOR_ENABLED=") {
			filteredLines = append(filteredLines, line)
		}
	}

	// Update file
	envContent := strings.Join(filteredLines, "\n")
	if len(filteredLines) > 0 {
		envContent += "\n"
	}

	if err := os.WriteFile(datadogEnvPath, []byte(envContent), 0640); err != nil {
		return fmt.Errorf("failed to write environment file: %w", err)
	}

	if err := envFilePermissions.Ensure("/etc/datadog-agent"); err != nil {
		return fmt.Errorf("failed to set ownership on environment file: %w", err)
	}

	return nil
}
