// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/packagemanager"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var datadogAgentDDOTPackage = hooks{
	preInstall:  preInstallDatadogAgentDDOT,
	postInstall: postInstallDatadogAgentDDOT,
	preRemove:   preRemoveDatadogAgentDDOT,
}

const (
	agentDDOTPackage = "datadog-agent-ddot"
	datadogYamlPath  = "/etc/datadog-agent/datadog.yaml"
)

var (
	// ddotDirectories are the directories that DDOT needs to function
	ddotDirectories = file.Directories{
		{Path: "/etc/datadog-agent", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
	}

	// ddotConfigPermissionsDEBRPM are the ownerships and modes that are enforced on the DDOT configuration files for DEB/RPM packages
	ddotConfigPermissionsDEBRPM = file.Permissions{
		{Path: "otel-config.yaml.example", Owner: "dd-agent", Group: "dd-agent", Mode: 0640},
	}

	// ddotConfigPermissionsOCI are the ownerships and modes that are enforced on the DDOT configuration files for OCI packages
	ddotConfigPermissionsOCI = file.Permissions{
		{Path: "otel-config.yaml.example", Owner: "dd-agent", Group: "dd-agent", Mode: 0640},
		{Path: "otel-config.yaml", Owner: "dd-agent", Group: "dd-agent", Mode: 0640},
	}

	// ddotPackagePermissions are the ownerships and modes that are enforced on the DDOT package files
	ddotPackagePermissions = file.Permissions{
		{Path: ".", Owner: "dd-agent", Group: "dd-agent", Recursive: true},
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

// postInstallDatadogAgentDDOT performs post-installation steps for the DDOT packages
func postInstallDatadogAgentDDOT(ctx HookContext) (err error) {
	if ctx.PackageType == PackageTypeDEB || ctx.PackageType == PackageTypeRPM {
		return postInstallDatadogAgentDDOTDEBRPM(ctx)
	}
	if ctx.PackageType == PackageTypeOCI {
		return postInstallDatadogAgentDDOTOCI(ctx)
	}

	return fmt.Errorf("unsupported package type: %s", ctx.PackageType)
}

// postInstallDatadogAgentDDOTOCI performs post-installation steps for the DDOT OCI package
func postInstallDatadogAgentDDOTOCI(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("setup_ddot_filesystem")
	defer func() {
		span.Finish(err)
	}()

	// Write otel-config.yaml with API key substitution
	if err = writeOTelConfig(); err != nil {
		return fmt.Errorf("could not write otel-config.yaml file: %s", err)
	}

	// Ensure the dd-agent user and group exist
	if err = user.EnsureAgentUserAndGroup(ctx, "/opt/datadog-agent"); err != nil {
		return fmt.Errorf("failed to create dd-agent user and group: %v", err)
	}

	// Ensure directories and files exist and have correct permissions
	if err = ddotDirectories.Ensure(ctx); err != nil {
		return fmt.Errorf("failed to create DDOT directories: %v", err)
	}
	if err = ddotPackagePermissions.Ensure(ctx, ctx.PackagePath); err != nil {
		return fmt.Errorf("failed to set DDOT package ownerships: %v", err)
	}
	if err = ddotConfigPermissionsOCI.Ensure(ctx, "/etc/datadog-agent"); err != nil {
		return fmt.Errorf("failed to set DDOT config ownerships: %v", err)
	}

	// Enable otelcollector in datadog.yaml
	if err = enableOtelCollectorConfig(ctx); err != nil {
		return fmt.Errorf("failed to enable otelcollector in datadog.yaml: %v", err)
	}

	// Restart agent to pick up otelcollector config changes
	if err = agentService.RestartStable(ctx); err != nil {
		return fmt.Errorf("failed to restart agent after enabling otelcollector: %v", err)
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

// postInstallDatadogAgentDDOTDEBRPM performs post-installation steps for the DDOT DEB/RPM packages
func postInstallDatadogAgentDDOTDEBRPM(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("setup_ddot_filesystem")
	defer func() {
		span.Finish(err)
	}()

	// Ensure the dd-agent user and group exist
	if err = user.EnsureAgentUserAndGroup(ctx, "/opt/datadog-agent"); err != nil {
		return fmt.Errorf("failed to create dd-agent user and group: %v", err)
	}

	// Ensure directories and files exist and have correct permissions
	if err = ddotDirectories.Ensure(ctx); err != nil {
		return fmt.Errorf("failed to create DDOT directories: %v", err)
	}
	if err = ddotPackagePermissions.Ensure(ctx, ctx.PackagePath); err != nil {
		return fmt.Errorf("failed to set DDOT package ownerships: %v", err)
	}
	if err = ddotConfigPermissionsDEBRPM.Ensure(ctx, "/etc/datadog-agent"); err != nil {
		return fmt.Errorf("failed to set DDOT config ownerships: %v", err)
	}

	if err := agentDDOTService.WriteStable(ctx); err != nil {
		return fmt.Errorf("failed to write stable units: %s", err)
	}
	if err := agentDDOTService.EnableStable(ctx); err != nil {
		return fmt.Errorf("failed to install stable unit: %s", err)
	}

	return nil
}

// preRemoveDatadogAgentDDOT performs pre-removal steps for the DDOT package
// All the steps are allowed to fail
func preRemoveDatadogAgentDDOT(ctx HookContext) error {
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

	return nil
}

// enableOtelCollectorConfig adds otelcollector.enabled: true to datadog.yaml
func enableOtelCollectorConfig(ctx context.Context) error {
	if err := enableOtelCollectorConfigCommon(datadogYamlPath); err != nil {
		return err
	}

	datadogYamlPermissions := file.Permissions{
		{Path: "datadog.yaml", Owner: "dd-agent", Group: "dd-agent", Mode: 0640},
	}

	if err := datadogYamlPermissions.Ensure(ctx, "/etc/datadog-agent"); err != nil {
		return fmt.Errorf("failed to set ownership on datadog.yaml: %w", err)
	}

	return nil
}

// writeOTelConfig creates otel-config.yaml by substituting API key and site values from datadog.yaml
func writeOTelConfig() error {
	return writeOTelConfigCommon(datadogYamlPath, "/etc/datadog-agent/otel-config.yaml.example", "/etc/datadog-agent/otel-config.yaml", false, 0644)
}
