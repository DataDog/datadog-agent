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
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/procmgr"
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

	ddotProcmgrYAMLStable = "datadog-agent-ddot.yaml"
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

	// agentDDOTService are the legacy systemd services that came with the
	// standalone DDOT package. DDOT execution now converges on dd-procmgrd;
	// these units remain for backwards compatibility and rollback handling.
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
	if err := syncDDOTProcmgrAfterStandalonePackageInstall(ctx); err != nil {
		return fmt.Errorf("failed to sync ddot process manager config: %w", err)
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
	// For backwards compatibility, remove "/ext/ddot" from the unit path
	if err := modifyDDOTUnitFileForBackwardsCompatibility(ctx, agentDDOTService.SystemdMainUnitStable, false); err != nil {
		return fmt.Errorf("failed to modify unit file for backwards compatibility: %s", err)
	}
	if err := systemd.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd: %s", err)
	}
	if err := syncDDOTProcmgrAfterStandalonePackageInstall(ctx); err != nil {
		return fmt.Errorf("failed to sync ddot process manager config: %w", err)
	}

	return nil
}

// preRemoveDatadogAgentDDOT performs pre-removal steps for the DDOT package
// All the steps are allowed to fail
func preRemoveDatadogAgentDDOT(ctx HookContext) error {
	if err := syncDDOTProcmgrStop(ctx, true); err != nil {
		log.Warnf("failed to remove DDOT procmgr stable config: %s", err)
	}
	if err := syncDDOTProcmgrStop(ctx, false); err != nil {
		log.Warnf("failed to remove DDOT procmgr experiment config: %s", err)
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

// processes.d and marker files: GetServiceManagerType == ProcmgrType requires the
// global gate (DD_PROCMGR_ENABLED or .procmgr-enabled); writing DDOT YAML also
// requires the DDOT gate (DD_PROCMGR_DDOT_ENABLED or .procmgr-ddot-enabled).
// Otherwise the ddot systemd unit owns DDOT (ConditionPathExists=! when YAML is absent).

func ddotProcmgrProcessesDir(ctx HookContext, stable bool) string {
	if ctx.PackageType == PackageTypeOCI {
		ver := "stable"
		if !stable {
			ver = "experiment"
		}
		return filepath.Join(paths.PackagesPath, "datadog-agent", ver, "processes.d")
	}
	return filepath.Join("/opt/datadog-agent", "processes.d")
}

// ociAgentStableAndExperimentProcessesDirsEquivalent reports whether the fleet
// OCI stable and experiment trees use the same processes.d directory on disk.
// After repository.PromoteExperiment both symlinks often target the same version
// directory, so cleaning "experiment/processes.d" would remove the DDOT YAML
// just written under "stable/processes.d".
func ociAgentStableAndExperimentProcessesDirsEquivalent() (bool, error) {
	stableDir := filepath.Join(paths.PackagesPath, "datadog-agent", "stable", "processes.d")
	expDir := filepath.Join(paths.PackagesPath, "datadog-agent", "experiment", "processes.d")
	stableResolved, err := filepath.EvalSymlinks(stableDir)
	if err != nil {
		return false, err
	}
	expResolved, err := filepath.EvalSymlinks(expDir)
	if err != nil {
		return false, err
	}
	return stableResolved == expResolved, nil
}

func procmgrBinaryExists(ctx HookContext, stable bool) bool {
	var binPath string
	if ctx.PackageType == PackageTypeOCI {
		ver := "stable"
		if !stable {
			ver = "experiment"
		}
		binPath = filepath.Join(paths.PackagesPath, "datadog-agent", ver, "embedded", "bin", "dd-procmgrd")
	} else {
		binPath = filepath.Join("/opt/datadog-agent", "embedded", "bin", "dd-procmgrd")
	}
	_, err := os.Stat(binPath)
	return err == nil
}

// adjustDDOTProcessYAML tweaks embedded YAML for extension vs standalone DDOT
// package layout differences.
func adjustDDOTProcessYAML(content string, ctx HookContext, standalone bool) string {
	if standalone && (ctx.PackageType == PackageTypeDEB || ctx.PackageType == PackageTypeRPM) {
		// Standalone datadog-agent-ddot DEB/RPM packages install otel-agent under
		// embedded/bin (no ext/ddot). The DDOT extension keeps binaries under ext/ddot.
		content = strings.ReplaceAll(content, "/ext/ddot", "")
	}
	if standalone && ctx.PackageType == PackageTypeOCI {
		// Standalone OCI DDOT package installs under datadog-agent-ddot and
		// does not use /ext/ddot in embedded/bin paths.
		content = strings.ReplaceAll(content, "/ext/ddot", "")
		content = strings.ReplaceAll(content, "/opt/datadog-packages/datadog-agent/stable", "/opt/datadog-packages/datadog-agent-ddot/stable")
		content = strings.ReplaceAll(content, "/opt/datadog-packages/datadog-agent/experiment", "/opt/datadog-packages/datadog-agent-ddot/experiment")
	}
	return content
}

func ddotEmbeddedUnitType(ctx HookContext) embedded.SystemdUnitType {
	if ctx.PackageType == PackageTypeOCI {
		return embedded.SystemdUnitTypeOCI
	}
	return embedded.SystemdUnitTypeDebRpm
}

func procmgrOwnsDDOT(ctx HookContext, stable bool) bool {
	return service.GetServiceManagerType() == service.ProcmgrType &&
		procmgr.ProcessGateOpen(procmgr.DDOTEnvVar, procmgr.DDOTMarkerPath) &&
		procmgrBinaryExists(ctx, stable)
}

// applyDDOTProcmgrProcessesYAML updates processes.d for DDOT only (no systemd
// restart). Removes the YAML when procmgr does not own DDOT; otherwise writes it.
func applyDDOTProcmgrProcessesYAML(ctx HookContext, stable, standalone bool) (ownsDDOT bool, err error) {
	if service.BaseServiceManagerType() != service.SystemdType {
		return false, nil
	}
	dir := ddotProcmgrProcessesDir(ctx, stable)
	yamlName := ddotProcmgrYAMLStable
	ownsDDOT = procmgrOwnsDDOT(ctx, stable)
	if !ownsDDOT {
		procmgr.RemoveConfig(dir, yamlName)
		return false, nil
	}
	ambiantCapabilitiesSupported, aerr := isAmbiantCapabilitiesSupported()
	if aerr != nil {
		log.Errorf("failed to check if ambiant capabilities are supported: %v", aerr)
		ambiantCapabilitiesSupported = true // Assume true if we can't check
	}
	raw, err := embedded.GetDDOTProcessConfig(ddotEmbeddedUnitType(ctx), stable, ambiantCapabilitiesSupported)
	if err != nil {
		return true, fmt.Errorf("ddot procmgr yaml: %w", err)
	}
	content := adjustDDOTProcessYAML(string(raw), ctx, standalone)
	if err := procmgr.WriteConfig(dir, yamlName, content); err != nil {
		return true, err
	}
	return true, nil
}

// syncDDOTProcmgrState applies DDOT processes.d YAML then restarts the matching
// dd-procmgrd unit so the daemon reloads configuration.
func syncDDOTProcmgrState(ctx HookContext, stable, standalone bool) (bool, error) {
	ownsDDOT, err := applyDDOTProcmgrProcessesYAML(ctx, stable, standalone)
	if err != nil {
		return false, err
	}
	if !ownsDDOT {
		if procmgrBinaryExists(ctx, stable) {
			if err := procmgr.RestartDaemon(ctx, !stable); err != nil {
				log.Warnf("failed to restart dd-procmgrd after dropping DDOT config: %v", err)
			}
		}
		return false, nil
	}
	return ownsDDOT, procmgr.RestartDaemon(ctx, !stable)
}

func syncDDOTProcmgrStop(ctx HookContext, stable bool) error {
	if service.BaseServiceManagerType() != service.SystemdType {
		return nil
	}
	dir := ddotProcmgrProcessesDir(ctx, stable)
	yamlName := ddotProcmgrYAMLStable
	procmgr.RemoveConfig(dir, yamlName)
	if !procmgrBinaryExists(ctx, stable) {
		return nil
	}
	return procmgr.RestartDaemon(ctx, !stable)
}

// writeProcmgrDDOTEnabledMarkerIfSystemd writes the global and DDOT procmgr
// marker files on systemd hosts (same env-or-marker semantics as service gates).
func writeProcmgrDDOTEnabledMarkerIfSystemd(ctx HookContext) error {
	if err := writeProcmgrGlobalMarkerIfSystemd(ctx); err != nil {
		return err
	}
	if service.BaseServiceManagerType() != service.SystemdType {
		return nil
	}
	if raw, ok := os.LookupEnv(procmgr.DDOTEnvVar); ok && !procmgr.EnvTruthy(raw) {
		_ = os.Remove(procmgr.DDOTMarkerPath)
		return nil
	}
	if err := writeProcmgrMarker(ctx, procmgr.DDOTMarkerPath); err != nil {
		return fmt.Errorf("write ddot procmgr marker: %w", err)
	}
	return nil
}

func removeProcmgrDDOTMarker() {
	_ = os.Remove(procmgr.DDOTMarkerPath)
}

// ddotProcmgrYAMLBodyUsesStandaloneOCIPackage reports whether existing DDOT
// procmgr YAML targets the standalone datadog-agent-ddot OCI package paths.
// Only the command and condition_path_exists entries are inspected so we do
// not match unrelated lines (env, comments) if the path appears elsewhere.
func ddotProcmgrYAMLBodyUsesStandaloneOCIPackage(data []byte) bool {
	const marker = "datadog-packages/datadog-agent-ddot/"
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "command:") && !strings.HasPrefix(line, "condition_path_exists:") {
			continue
		}
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

// ociStableDDOTProcmgrYAMLIsStandaloneLayout reports whether stable processes.d
// already contains DDOT YAML for the standalone datadog-agent-ddot OCI layout
// (not the agent extension layout). Used on agent promote so we do not replace
// standalone paths with extension paths.
func ociStableDDOTProcmgrYAMLIsStandaloneLayout() bool {
	dir := filepath.Join(paths.PackagesPath, "datadog-agent", "stable", "processes.d")
	data, err := os.ReadFile(filepath.Join(dir, ddotProcmgrYAMLStable))
	if err != nil {
		return false
	}
	return ddotProcmgrYAMLBodyUsesStandaloneOCIPackage(data)
}

// syncDDOTProcmgrAfterAgentPromotion refreshes stable DDOT YAML in processes.d
// after OCI promote (YAML only—stable agent may still be down). If stable and
// experiment package paths differ, also removes experiment DDOT YAML and restarts
// experiment procmgr; skipped when both paths are the same directory.
func syncDDOTProcmgrAfterAgentPromotion(ctx HookContext) error {
	if ctx.PackageType != PackageTypeOCI {
		return nil
	}
	if service.BaseServiceManagerType() != service.SystemdType {
		return nil
	}
	stableOCI := filepath.Join(paths.PackagesPath, "datadog-agent", "stable")
	if _, err := os.Stat(stableOCI); err != nil {
		return nil
	}
	stableCtx := ctx
	stableCtx.PackagePath = stableOCI
	standalone := ociStableDDOTProcmgrYAMLIsStandaloneLayout()
	if _, err := applyDDOTProcmgrProcessesYAML(stableCtx, true, standalone); err != nil {
		return err
	}
	equiv, err := ociAgentStableAndExperimentProcessesDirsEquivalent()
	if err != nil {
		log.Warnf("skipping experiment DDOT procmgr cleanup after promote: resolve processes.d paths: %v", err)
		return nil
	}
	if equiv {
		return nil
	}
	expCtx := ctx
	expCtx.PackagePath = filepath.Join(paths.PackagesPath, "datadog-agent", "experiment")
	return syncDDOTProcmgrStop(expCtx, false)
}

// syncDDOTProcmgrAfterExtension writes processes.d DDOT config after the DDOT
// extension is installed. The standalone datadog-agent-ddot package does not
// use this path (systemd-only).
func syncDDOTProcmgrAfterExtension(ctx HookContext) error {
	if err := writeProcmgrDDOTEnabledMarkerIfSystemd(ctx); err != nil {
		return err
	}
	switch ctx.PackageType {
	case PackageTypeOCI:
		stable := filepath.Base(ctx.PackagePath) != "experiment"
		_, err := syncDDOTProcmgrState(ctx, stable, false)
		return err
	case PackageTypeDEB, PackageTypeRPM:
		_, err := syncDDOTProcmgrState(ctx, true, false)
		return err
	default:
		return nil
	}
}

func syncDDOTProcmgrAfterStandalonePackageInstall(ctx HookContext) error {
	if err := writeProcmgrDDOTEnabledMarkerIfSystemd(ctx); err != nil {
		return err
	}
	ownsDDOT, err := syncDDOTProcmgrState(ctx, true, true)
	if err != nil {
		return err
	}
	if !ownsDDOT {
		// Keep systemd ownership when procmgr path is not active.
		if err := agentDDOTService.EnableStable(ctx); err != nil {
			return fmt.Errorf("failed to enable legacy ddot systemd unit: %w", err)
		}
		if err := agentDDOTService.RestartStable(ctx); err != nil {
			return fmt.Errorf("failed to restart legacy ddot systemd unit: %w", err)
		}
		return nil
	}
	// Ensure convergence on process manager ownership.
	if err := agentDDOTService.StopStable(ctx); err != nil {
		log.Warnf("failed to stop legacy ddot systemd unit: %s", err)
	}
	if err := agentDDOTService.DisableStable(ctx); err != nil {
		log.Warnf("failed to disable legacy ddot systemd unit: %s", err)
	}
	return nil
}

// removeDDOTExtensionProcmgrYAML drops DDOT process definitions from processes.d
// when the extension is removed.
func removeDDOTExtensionProcmgrYAML(ctx HookContext) {
	if service.BaseServiceManagerType() != service.SystemdType {
		return
	}
	_ = syncDDOTProcmgrStop(ctx, true)
	_ = syncDDOTProcmgrStop(ctx, false)
}

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

	// Sync processes.d / procmgr markers and restart dd-procmgrd when gates allow.
	if err := syncDDOTProcmgrAfterExtension(ctx); err != nil {
		return fmt.Errorf("failed to sync ddot process manager config: %w", err)
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

	removeDDOTExtensionProcmgrYAML(ctx)
	removeProcmgrDDOTMarker()

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
