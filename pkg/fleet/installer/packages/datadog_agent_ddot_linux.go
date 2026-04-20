// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/packagemanager"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var datadogAgentDDOTPackage = hooks{
	preInstall:  preInstallDatadogAgentDDOT,
	postInstall: postInstallDatadogAgentDDOT,
	preRemove:   preRemoveDatadogAgentDDOT,
}

const (
	agentDDOTPackage      = "datadog-agent-ddot"
	datadogYamlPath       = "/etc/datadog-agent/datadog.yaml"
	otelConfigPath        = "/etc/datadog-agent/otel-config.yaml"
	otelConfigExamplePath = "/etc/datadog-agent/otel-config.yaml.example"

	ddotProcessConfigName = "datadog-agent-ddot.yaml"
	agentInstallDirDebRpm = "/opt/datadog-agent"
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

	// ddotConfigPermissions are the ownerships and modes that are enforced on the DDOT configuration files for OCI packages
	ddotConfigPermissions = file.Permissions{
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
	if err = writeOTelConfig(ctx); err != nil {
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
	if err = ddotConfigPermissions.Ensure(ctx, "/etc/datadog-agent"); err != nil {
		return fmt.Errorf("failed to set DDOT config ownerships: %v", err)
	}

	// Enable otelcollector in datadog.yaml
	if err = enableOTelCollectorConfigInDatadogYAML(ctx, datadogYamlPath); err != nil {
		return fmt.Errorf("failed to enable otelcollector in datadog.yaml: %v", err)
	}

	// Restart agent to pick up otelcollector config changes
	if err = agentService.RestartStable(ctx); err != nil {
		return fmt.Errorf("failed to restart agent after enabling otelcollector: %v", err)
	}

	// Write the procmgr YAML so dd-procmgrd manages DDOT instead of systemd.
	// This must happen before writing the systemd unit, because the unit's
	// ConditionPathExists=! will skip itself when the YAML is present.
	if err = writeDDOTProcessConfig(ctx); err != nil {
		return fmt.Errorf("failed to write DDOT process config: %v", err)
	}

	if err := agentDDOTService.WriteStable(ctx); err != nil {
		return fmt.Errorf("failed to write stable units: %s", err)
	}
	// For backwards compatibility, remove "/ext/ddot" from the unit path and replace "/opt/datadog-packages/datadog-agent" by "/opt/datadog-packages/datadog-agent-ddot"
	if err := modifyDDOTUnitFileForBackwardsCompatibility(ctx, agentDDOTService.SystemdMainUnitStable, true); err != nil {
		return fmt.Errorf("failed to modify unit file for backwards compatibility: %s", err)
	}
	if err := systemd.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd: %s", err)
	}
	if err := agentDDOTService.EnableStable(ctx); err != nil {
		return fmt.Errorf("failed to install stable unit: %s", err)
	}
	if err := agentDDOTService.RestartStable(ctx); err != nil {
		return fmt.Errorf("failed to restart stable unit: %s", err)
	}

	// Restart procmgrd so it picks up the new DDOT config file.
	// dd-procmgrd does not watch the config directory; it requires an
	// explicit reload or restart to detect new process definitions.
	if err := restartProcmgrd(ctx); err != nil {
		log.Warnf("failed to restart procmgrd: %s", err)
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

	// Write the procmgr YAML so dd-procmgrd manages DDOT instead of systemd.
	if err = writeDDOTProcessConfig(ctx); err != nil {
		return fmt.Errorf("failed to write DDOT process config: %v", err)
	}

	if err := agentDDOTService.WriteStable(ctx); err != nil {
		return fmt.Errorf("failed to write stable units: %s", err)
	}
	// For backwards compatibility, remove "/ext/ddot" from the unit path
	if err := modifyDDOTUnitFileForBackwardsCompatibility(ctx, agentDDOTService.SystemdMainUnitStable, false); err != nil {
		return fmt.Errorf("failed to modify unit file for backwards compatibility: %s", err)
	}
	if err := systemd.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd: %s", err)
	}
	if err := agentDDOTService.EnableStable(ctx); err != nil {
		return fmt.Errorf("failed to install stable unit: %s", err)
	}

	// Restart procmgrd so it picks up the new DDOT config file.
	if err := restartProcmgrd(ctx); err != nil {
		log.Warnf("failed to restart procmgrd: %s", err)
	}

	return nil
}

// preRemoveDatadogAgentDDOT performs pre-removal steps for the DDOT package
// All the steps are allowed to fail
func preRemoveDatadogAgentDDOT(ctx HookContext) error {
	removeDDOTProcessConfig(ctx.PackageType)
	// Restart procmgrd so it detects the removed config and stops the DDOT
	// process. Without this, procmgrd would keep supervising otel-agent
	// from its in-memory state until the agent service is restarted.
	if err := restartProcmgrd(ctx); err != nil {
		log.Warnf("failed to restart procmgrd after removing DDOT config: %s", err)
	}

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

// writeOTelConfig creates otel-config.yaml by substituting API key and site values from datadog.yaml, fallback with env variables.
func writeOTelConfig(ctx HookContext) error {
	return writeOTelConfigCommon(ctx, datadogYamlPath, otelConfigExamplePath, otelConfigPath, false, 0640)
}

//////////////////////////////
/// DDOT EXTENSION METHODS ///
//////////////////////////////

// preInstallDDOTExtension stops and removes the existing DDOT service and package before extension installation
func preInstallDDOTExtension(ctx HookContext) error {
	span, ctx := ctx.StartSpan("pre_install_extension_ddot")
	defer span.Finish(nil)

	if err := packagemanager.RemovePackage(ctx, agentDDOTPackage); err != nil {
		log.Warnf("failed to remove deb/rpm package: %s", err)
	}
	return nil
}

// postInstallDDOTExtension is the post-install hook for the DDOT extension
func postInstallDDOTExtension(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("post_install_extension_ddot")
	defer func() {
		span.Finish(err)
	}()

	// extensionPath is the path to the DDOT extension. It is already scoped to stable / experiment per the Agent package.
	extensionPath := filepath.Join(ctx.PackagePath, "ext", "ddot")

	// Copy the example file to the configuration directory
	// XXX: Maybe we should always embed the example file in the Agent package?
	if err := copyFile(filepath.Join(extensionPath, otelConfigExamplePath), otelConfigExamplePath, 0640); err != nil {
		return fmt.Errorf("failed to copy otel-config.yaml.example to /etc/datadog-agent: %v", err)
	}

	// Write otel-config.yaml. Doesn't update the file if it already exists.
	if err := writeOTelConfigCommon(ctx, datadogYamlPath, otelConfigExamplePath, otelConfigPath, true, 0640); err != nil {
		return fmt.Errorf("failed to write otel-config.yaml: %w", err)
	}

	// Enable the DDOT IPC server in datadog.yaml
	if err := enableOTelCollectorConfigInDatadogYAML(ctx, datadogYamlPath); err != nil {
		return fmt.Errorf("failed to enable otelcollector in datadog.yaml: %v", err)
	}

	// Ensure the DDOT configuration files have the correct permissions
	if err = ddotConfigPermissions.Ensure(ctx, "/etc/datadog-agent"); err != nil {
		return fmt.Errorf("failed to set DDOT config ownerships: %v", err)
	}

	return nil
}

// preRemoveDDOTExtension stops and disables the DDOT service before extension removal
func preRemoveDDOTExtension(ctx HookContext) error {
	span, _ := ctx.StartSpan("pre_remove_extension_ddot")
	defer span.Finish(nil)

	// Disable the DDOT IPC server in datadog.yaml.
	// During an upgrade, this will be re-enabled by the post-install hook. This gives us flexibility to change the config during upgrade.
	if err := disableOtelCollectorConfigCommon(datadogYamlPath); err != nil {
		log.Warnf("failed to disable otelcollector config: %s", err)
	}

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

// agentProcessesDir returns the agent's processes.d directory based on the package type.
func agentProcessesDir(packageType PackageType) string {
	if packageType == PackageTypeOCI {
		return filepath.Join(paths.PackagesPath, "datadog-agent", "stable", "processes.d")
	}
	return filepath.Join(agentInstallDirDebRpm, "processes.d")
}

// procmgrdBinaryPath returns the expected path to the dd-procmgrd binary.
func procmgrdBinaryPath(packageType PackageType) string {
	if packageType == PackageTypeOCI {
		return filepath.Join(paths.PackagesPath, "datadog-agent", "stable", "embedded", "bin", "dd-procmgrd")
	}
	return filepath.Join(agentInstallDirDebRpm, "embedded", "bin", "dd-procmgrd")
}

// writeDDOTProcessConfig writes the process manager YAML config for DDOT to
// the agent's processes.d directory. When this file is present, systemd's
// ConditionPathExists=! causes it to skip the DDOT unit, and dd-procmgrd
// picks up management instead.
//
// If dd-procmgrd is not installed (binary not found), the write is skipped
// and DDOT falls back to systemd management.
func writeDDOTProcessConfig(ctx HookContext) error {
	if _, err := os.Stat(procmgrdBinaryPath(ctx.PackageType)); os.IsNotExist(err) {
		log.Infof("dd-procmgrd not found, skipping process config write; DDOT will be managed by systemd")
		return nil
	}

	var unitType embedded.SystemdUnitType
	if ctx.PackageType == PackageTypeOCI {
		unitType = embedded.SystemdUnitTypeOCI
	} else {
		unitType = embedded.SystemdUnitTypeDebRpm
	}

	content, err := embedded.GetDDOTProcessConfig(unitType, true)
	if err != nil {
		return fmt.Errorf("failed to get DDOT process config: %w", err)
	}

	// The embedded YAML uses extension-layout paths (e.g. .../ext/ddot/...).
	// The standalone DDOT package installs the binary at a different path,
	// so apply the same rewrites as modifyDDOTUnitFileForBackwardsCompatibility.
	content = strings.ReplaceAll(content, "/ext/ddot", "")
	if ctx.PackageType == PackageTypeOCI {
		content = strings.ReplaceAll(content, "/opt/datadog-packages/datadog-agent/stable", "/opt/datadog-packages/datadog-agent-ddot/stable")
		content = strings.ReplaceAll(content, "/opt/datadog-packages/datadog-agent/experiment", "/opt/datadog-packages/datadog-agent-ddot/experiment")
	}

	dest := filepath.Join(agentProcessesDir(ctx.PackageType), ddotProcessConfigName)
	if err := os.WriteFile(dest, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write DDOT process config to %s: %w", dest, err)
	}
	return nil
}

// removeDDOTProcessConfig removes the process manager YAML config for DDOT
// from the agent's processes.d directory. After removal, systemd's
// ConditionPathExists=! is satisfied and the DDOT unit can start again.
func removeDDOTProcessConfig(packageType PackageType) {
	dest := filepath.Join(agentProcessesDir(packageType), ddotProcessConfigName)
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		log.Warnf("failed to remove DDOT process config %s: %s", dest, err)
	}
}

// restoreDDOTProcessConfig re-creates the DDOT procmgr YAML if the standalone
// DDOT OCI package is still installed. This is needed for OCI agent upgrades
// where the versioned directory (and its processes.d/) is recreated from scratch.
//
// This intentionally skips DEB/RPM: the install path is stable across upgrades
// and extension-based DDOT installs use different paths that would be broken
// by the standalone path rewriting in writeDDOTProcessConfig.
func restoreDDOTProcessConfig(ctx HookContext) error {
	if ctx.PackageType != PackageTypeOCI {
		return nil
	}
	ddotPkgPath := filepath.Join(paths.PackagesPath, "datadog-agent-ddot", "stable")
	if _, err := os.Stat(ddotPkgPath); os.IsNotExist(err) {
		return nil
	}
	return writeDDOTProcessConfig(ctx)
}

const procmgrdUnit = "datadog-agent-procmgrd.service"

// restartProcmgrd restarts the dd-procmgrd systemd unit so it reloads its
// config directory and picks up newly added or removed process definitions.
func restartProcmgrd(ctx HookContext) error {
	return systemd.RestartUnit(ctx, procmgrdUnit)
}

// modifyDDOTUnitFileForBackwardsCompatibility modifies the systemd unit file to remove "/ext/ddot" from paths
// for backwards compatibility. For OCI packages, it also replaces "/opt/datadog-packages/datadog-agent" with
// "/opt/datadog-packages/datadog-agent-ddot".
// This is likely temporary, it'll be removed when we remove the standalone DDOT package.
func modifyDDOTUnitFileForBackwardsCompatibility(ctx HookContext, unitName string, isOCI bool) error {
	// Determine the unit file path based on package type
	var unitPath string
	switch ctx.PackageType {
	case PackageTypeDEB:
		unitPath = filepath.Join("/lib/systemd/system", unitName)
	case PackageTypeRPM:
		unitPath = filepath.Join("/usr/lib/systemd/system", unitName)
	case PackageTypeOCI:
		unitPath = filepath.Join("/etc/systemd/system", unitName)
	default:
		return fmt.Errorf("unsupported package type: %s", ctx.PackageType)
	}

	// Read the unit file
	content, err := os.ReadFile(unitPath)
	if err != nil {
		return fmt.Errorf("failed to read unit file %s: %w", unitPath, err)
	}

	// Modify the content line-by-line so we can skip ConditionPathExists lines
	// from the OCI path rewrite. The processes.d gate must always reference the
	// Agent package tree, not the standalone DDOT package tree.
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		// Remove "/ext/ddot" from all paths
		lines[i] = strings.ReplaceAll(line, "/ext/ddot", "")

		// For OCI packages, rewrite Agent package paths to DDOT package paths,
		// but skip ConditionPathExists lines so the processes.d gate keeps
		// pointing at the Agent install directory.
		if isOCI && !strings.HasPrefix(strings.TrimSpace(lines[i]), "ConditionPathExists=!") {
			lines[i] = strings.ReplaceAll(lines[i], "/opt/datadog-packages/datadog-agent/stable", "/opt/datadog-packages/datadog-agent-ddot/stable")
			lines[i] = strings.ReplaceAll(lines[i], "/opt/datadog-packages/datadog-agent/experiment", "/opt/datadog-packages/datadog-agent-ddot/experiment")
		}
	}
	modifiedContent := strings.Join(lines, "\n")

	// Write the modified content back
	if err := os.WriteFile(unitPath, []byte(modifiedContent), 0644); err != nil {
		return fmt.Errorf("failed to write modified unit file %s: %w", unitPath, err)
	}

	return nil
}
