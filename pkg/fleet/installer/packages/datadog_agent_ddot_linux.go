// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/packagemanager"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
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
	return writeOTelConfigCommon(datadogYamlPath, "/etc/datadog-agent/otel-config.yaml.example", "/etc/datadog-agent/otel-config.yaml", false, 0640)
}

// DDOT Extension methods

// preInstallDDOTExtension stops the existing DDOT service before extension installation
func preInstallDDOTExtension(ctx HookContext) error {
	span, ctx := ctx.StartSpan("pre_install_extension_ddot")
	defer span.Finish(nil)

	// Best effort - ignore errors
	_ = agentDDOTService.StopStable(ctx)

	return nil
}

// postInstallDDOTExtension sets up the DDOT extension after files are extracted
func postInstallDDOTExtension(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("post_install_extension_ddot")
	defer func() {
		span.Finish(err)
	}()

	extensionPath := filepath.Join(ctx.PackagePath, "ext", "ddot")

	// Write otel-config.yaml - best effort, ignore errors
	_ = writeDDOTExtensionOTelConfig(extensionPath)

	// Ensure the dd-agent user and group exist
	if err = user.EnsureAgentUserAndGroup(ctx, "/opt/datadog-agent"); err != nil {
		return fmt.Errorf("failed to create dd-agent user and group: %v", err)
	}

	// Ensure directories and files exist and have correct permissions
	if err = ddotDirectories.Ensure(ctx); err != nil {
		return fmt.Errorf("failed to create DDOT directories: %v", err)
	}
	if err = ddotPackagePermissions.Ensure(ctx, extensionPath); err != nil {
		return fmt.Errorf("failed to set DDOT extension ownerships: %v", err)
	}
	if err = ddotConfigPermissionsOCI.Ensure(ctx, "/etc/datadog-agent"); err != nil {
		return fmt.Errorf("failed to set DDOT config ownerships: %v", err)
	}

	if err := enableOtelCollectorConfig(ctx); err != nil {
		return fmt.Errorf("failed to enable otelcollector in datadog.yaml: %v", err)
	}

	if err := writeDDOTExtensionServiceUnits(ctx, extensionPath); err != nil {
		return fmt.Errorf("failed to write DDOT service units: %w", err)
	}

	if err := agentDDOTService.EnableStable(ctx); err != nil {
		return fmt.Errorf("failed to enable DDOT service: %s", err)
	}

	// Best effort service start - ignore errors
	_ = agentDDOTService.RestartStable(ctx)

	return nil
}

// preRemoveDDOTExtension stops and disables the DDOT service before extension removal
func preRemoveDDOTExtension(ctx HookContext) error {
	span, ctx := ctx.StartSpan("pre_remove_extension_ddot")
	defer span.Finish(nil)

	// Best effort - ignore errors
	_ = agentDDOTService.StopStable(ctx)
	_ = agentDDOTService.DisableStable(ctx)
	_ = agentDDOTService.RemoveStable(ctx)
	_ = disableOtelCollectorConfigCommon(datadogYamlPath)

	return nil
}

// copyFile copies a file from src to dst with the specified permissions
func copyFile(src, dst string, perm os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, perm)
}

// writeDDOTExtensionOTelConfig writes the DDOT otel-config.yaml with API key substitution
func writeDDOTExtensionOTelConfig(extensionPath string) error {
	templatePath := "/etc/datadog-agent/otel-config.yaml.example"

	extensionTemplatePath := filepath.Join(extensionPath, "etc", "datadog-agent", "otel-config.yaml.example")
	if _, err := os.Stat(extensionTemplatePath); err == nil {
		templatePath = extensionTemplatePath

		// Copy the .example file to /etc/datadog-agent/ for user reference
		exampleDestPath := "/etc/datadog-agent/otel-config.yaml.example"
		if err := copyFile(extensionTemplatePath, exampleDestPath, 0640); err != nil {
			log.Warnf("failed to copy otel-config.yaml.example to /etc/datadog-agent: %v", err)
		}
	}

	outPath := "/etc/datadog-agent/otel-config.yaml"
	return writeOTelConfigCommon(datadogYamlPath, templatePath, outPath, false, 0640)
}

// writeDDOTExtensionServiceUnits writes DDOT service units for the appropriate service manager
func writeDDOTExtensionServiceUnits(ctx HookContext, extensionPath string) error {
	switch service.GetServiceManagerType() {
	case service.SystemdType:
		return writeDDOTExtensionSystemdService(ctx, extensionPath)
	case service.UpstartType:
		return nil
	case service.SysvinitType:
		return nil
	default:
		return errors.New("unsupported service manager")
	}
}

// writeDDOTExtensionSystemdService writes the DDOT systemd unit with paths adjusted for the extension
func writeDDOTExtensionSystemdService(ctx HookContext, extensionPath string) error {
	ambiantCapabilitiesSupported, err := isAmbiantCapabilitiesSupported()
	if err != nil {
		log.Errorf("failed to check if ambiant capabilities are supported: %v", err)
		ambiantCapabilitiesSupported = true
	}

	unitName := agentDDOTService.SystemdMainUnitStable
	content, err := embedded.GetSystemdUnit(unitName, embedded.SystemdUnitTypeOCI, ambiantCapabilitiesSupported)
	if err != nil {
		return fmt.Errorf("failed to get systemd unit %s: %w", unitName, err)
	}

	contentStr := strings.ReplaceAll(string(content), "/opt/datadog-packages/datadog-agent-ddot/stable", extensionPath)

	unitPath := filepath.Join(ociUnitsPath, unitName)
	if err := os.MkdirAll(ociUnitsPath, 0755); err != nil {
		return fmt.Errorf("failed to create systemd directory: %v", err)
	}
	if err := os.WriteFile(unitPath, []byte(contentStr), 0644); err != nil {
		return fmt.Errorf("failed to write systemd unit file: %v", err)
	}

	return systemd.Reload(ctx)
}
