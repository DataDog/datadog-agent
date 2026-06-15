// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	windowssvc "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/windows"
	windowsuser "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user/windows"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

var datadogAgentDDOTPackage = hooks{
	preInstall:  preInstallDatadogAgentDDOT,
	postInstall: postInstallDatadogAgentDdot,
	preRemove:   preRemoveDatadogAgentDdot,
}

const (
	agentDDOTPackage = "datadog-agent-ddot"
	otelServiceName  = "datadog-otel-agent"
	coreAgentService = "datadogagent"

	// Basename of the dd-procmgr process definition for DDOT (matches Linux OCI extension hook).
	ddotProcmgrConfigFileName      = "datadog-agent-ddot.yaml"
	ddProcmgrServiceName           = "dd-procmgr-service"
	ddotProcmgrBaselineEnvFileName = "ddot-procmgr-baseline.env"
)

// windowsProcmgrBaselineEnvKeys are copied from the installer's process environment into
// ddot-procmgr-baseline.env so dd-procmgr children inherit a minimal Windows baseline.
var windowsProcmgrBaselineEnvKeys = []string{
	"SystemRoot", "WINDIR", "SystemDrive", "ProgramData", "ProgramFiles", "ProgramFiles(x86)", "ProgramW6432",
	"CommonProgramFiles", "CommonProgramFiles(x86)", "CommonProgramW6432", "PUBLIC", "TEMP", "TMP",
	"Path", "PATHEXT", "LOCALAPPDATA", "APPDATA", "USERPROFILE", "ComSpec",
}

// preInstallDatadogAgentDDOT performs pre-installation steps for DDOT on Windows
func preInstallDatadogAgentDDOT(ctx HookContext) error {
	return preInstallDDOTExtension(ctx)
}

// postInstallDatadogAgentDdot performs post-installation steps for the DDOT package on Windows
func postInstallDatadogAgentDdot(ctx HookContext) (err error) {
	// 1) Write otel-config.yaml with API key/site substitutions
	if err = writeOTelConfigWindows(ctx); err != nil {
		return fmt.Errorf("could not write otel-config.yaml: %w", err)
	}
	// 2) Enable otelcollector in datadog.yaml
	if err = enableOtelCollectorConfigWindows(ctx); err != nil {
		return fmt.Errorf("failed to enable otelcollector: %w", err)
	}
	// 3) Restart main Agent services to pick up config changes
	if err = windowssvc.NewWinServiceManager().RestartAgentServices(ctx.Context); err != nil {
		return fmt.Errorf("failed to restart agent services: %w", err)
	}
	// 4) Ensure DDOT service exists/updated, then start it (best-effort)
	if err = ensureDDOTService(); err != nil {
		return fmt.Errorf("failed to install ddot service: %w", err)
	}
	// Start DDOT only when core Agent is running (handle StartPending) and credentials exist
	running, _ := winutil.IsServiceRunning(coreAgentService)
	if !running {
		// If core Agent is still starting, wait briefly for it to leave StartPending
		ctxCA, cancelCA := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelCA()
		if st, err := winutil.WaitForPendingStateChange(ctxCA, coreAgentService, svc.StartPending); err != nil || st != svc.Running {
			log.Warnf("DDOT: skipping service start (core Agent not running; state=%d, err=%v)", st, err)
			return nil
		}
	}
	ak, err := readAPIKeyFromDatadogYAML()
	if err != nil {
		log.Warnf("DDOT: skipping service start: %v", err)
		return nil
	}
	if ak == "" {
		log.Warnf("DDOT: skipping service start (no API key configured)")
		return nil
	}
	if err = startServiceIfExists(otelServiceName); err != nil {
		log.Warnf("DDOT: failed to start service: %v", err)
		return nil
	}
	// Fail fast if the service exits or transitions away from StartPending
	ctxWait, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	state, err := winutil.WaitForPendingStateChange(ctxWait, otelServiceName, svc.StartPending)
	if err != nil {
		log.Warnf("DDOT: service %q did not reach Running state: %s", otelServiceName, err)
		return nil
	}
	if state != svc.Running {
		log.Warnf("DDOT: service %q transitioned to state %d instead of Running", otelServiceName, state)
		return nil
	}
	return nil
}

// readAPIKeyFromDatadogYAML reads the api_key from ProgramData datadog.yaml, returns empty string if unset/unknown
func readAPIKeyFromDatadogYAML() (string, error) {
	ddYaml := filepath.Join(paths.DatadogDataDir, "datadog.yaml")
	data, err := os.ReadFile(ddYaml)
	if err != nil {
		return "", fmt.Errorf("failed to read datadog.yaml from %s: %w", ddYaml, err)
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("failed to parse datadog.yaml: %w", err)
	}
	if v, ok := cfg["api_key"].(string); ok && v != "" {
		return v, nil
	}
	return "", errors.New("api_key not found or empty in datadog.yaml")
}

// yamlNestedStringKeyMap returns a string-keyed map for a YAML-decoded nested object.
// yaml.v2 stores nested maps as map[interface{}]interface{} when the parent is map[string]any,
// so a plain .(map[string]any) on cfg["process_manager"] misses valid configs.
func yamlNestedStringKeyMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[interface{}]interface{}:
		out := make(map[string]any, len(m))
		for k, val := range m {
			ks, ok := k.(string)
			if !ok {
				continue
			}
			out[ks] = val
		}
		return out, true
	default:
		return nil, false
	}
}

// processManagerEnabledFromDatadogYAML returns whether the process manager should run for DDOT hooks,
// matching pkg/config/setup defaults: default true, overridable by DD_PROCESS_MANAGER_ENABLED, then
// process_manager.enabled in ProgramData datadog.yaml. Missing process_manager section or missing
// enabled key means true (same as BindEnvAndSetDefault("process_manager.enabled", true)).
func processManagerEnabledFromDatadogYAML() (bool, error) {
	if v, ok := os.LookupEnv("DD_PROCESS_MANAGER_ENABLED"); ok && strings.TrimSpace(v) != "" {
		return yamlTruthy(v), nil
	}
	ddYaml := filepath.Join(paths.DatadogDataDir, "datadog.yaml")
	data, err := os.ReadFile(ddYaml)
	if err != nil {
		return false, err
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return false, err
	}
	return processManagerEnabledFromCfgMap(cfg), nil
}

func processManagerEnabledFromCfgMap(cfg map[string]any) bool {
	pm, ok := yamlNestedStringKeyMap(cfg["process_manager"])
	if !ok {
		return true
	}
	if _, has := pm["enabled"]; !has {
		return true
	}
	return yamlTruthy(pm["enabled"])
}

func yamlTruthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(x, "true") || x == "1" || strings.EqualFold(x, "yes")
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case uint64:
		return x != 0
	default:
		return false
	}
}

// preRemoveDatadogAgentDdot performs pre-removal steps for the DDOT package on Windows
// All the steps are allowed to fail
func preRemoveDatadogAgentDdot(ctx HookContext) error {
	_ = stopServiceIfExists(otelServiceName)
	_ = deleteServiceIfExists(otelServiceName)

	if !ctx.Upgrade {
		// Preserve otel-config.yaml; only disable the feature in datadog.yaml
		if err := disableOtelCollectorConfigWindows(); err != nil {
			log.Warnf("failed to disable otelcollector in datadog.yaml: %s", err)
		}
		// Restart core agent to pick up reverted config
		if err := windowssvc.NewWinServiceManager().RestartAgentServices(ctx.Context); err != nil {
			log.Warnf("failed to restart agent services: %s", err)
		}
	}
	return nil
}

// writeOTelConfigWindows creates otel-config.yaml by substituting API key and site values from datadog.yaml, fallback with env variables.
func writeOTelConfigWindows(ctx HookContext) error {
	ddYaml := filepath.Join(paths.DatadogDataDir, "datadog.yaml")
	// Prefer packaged example/template from the installed package repository
	cfgTemplate := filepath.Join(paths.PackagesPath, agentDDOTPackage, "stable", "etc", "datadog-agent", "otel-config.yaml.example")
	// Fallback to local ProgramData example/template if needed
	if _, err := os.Stat(cfgTemplate); err != nil {
		alt := filepath.Join(paths.DatadogDataDir, "otel-config.yaml.example")
		if _, err2 := os.Stat(alt); err2 == nil {
			cfgTemplate = alt
		}
	}
	out := filepath.Join(paths.DatadogDataDir, "otel-config.yaml")
	return writeOTelConfigCommon(ctx, ddYaml, cfgTemplate, out, true, 0o600)
}

// enableOtelCollectorConfigWindows adds otelcollector.enabled and agent_ipc defaults to datadog.yaml
func enableOtelCollectorConfigWindows(ctx HookContext) error {
	ddYaml := filepath.Join(paths.DatadogDataDir, "datadog.yaml")
	return enableOTelCollectorConfigInDatadogYAML(ctx, ddYaml)
}

// disableOtelCollectorConfigWindows removes otelcollector and agent_ipc from datadog.yaml
func disableOtelCollectorConfigWindows() error {
	ddYaml := filepath.Join(paths.DatadogDataDir, "datadog.yaml")
	return disableOtelCollectorConfigCommon(ddYaml)
}

// ensureDDOTService ensures the DDOT service exists and is configured correctly
func ensureDDOTService() error {
	bin := filepath.Join(paths.PackagesPath, agentDDOTPackage, "stable", "embedded", "bin", "otel-agent.exe")
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(otelServiceName)
	if err == nil {
		defer s.Close()
		// update existing, remove SCM dependency if any
		cfg, errC := s.Config()
		if errC != nil {
			return errC
		}
		changed := false
		if cfg.StartType != mgr.StartManual {
			cfg.StartType = mgr.StartManual
			changed = true
		}
		if len(cfg.Dependencies) > 0 {
			// drop SCM dependency
			cfg.Dependencies = nil
			changed = true
		}
		if changed {
			if errU := s.UpdateConfig(cfg); errU != nil {
				return errU
			}
		}
		// Ensure service runs under the same account as the core Agent
		if err := configureDDOTServiceCredentials(s); err != nil {
			return err
		}
		// Best-effort: align service DACL to allow the core Agent user to control OTEL service
		configureDDOTServicePermissions(s)
		if err := setDDOTServiceEnvVars(); err != nil {
			return err
		}
		return nil
	}

	s, err = m.CreateService(otelServiceName, bin, mgr.Config{
		DisplayName:      "Datadog Distribution of OpenTelemetry Collector",
		Description:      "Datadog OpenTelemetry Collector",
		StartType:        mgr.StartManual,
		ServiceStartName: "", // LocalSystem
	})
	if err != nil {
		return err
	}
	defer s.Close()
	// Align credentials to ddagentuser (or equivalent) like other Agent services
	if err := configureDDOTServiceCredentials(s); err != nil {
		return err
	}
	// Best-effort: align service DACL to allow the core Agent user to control OTEL service
	configureDDOTServicePermissions(s)
	if err := setDDOTServiceEnvVars(); err != nil {
		return err
	}
	return nil
}

// configureDDOTServiceCredentials updates the service credentials to match the core Agent service user.
// For LocalSystem/LocalService/NetworkService the password is empty string.
// For other accounts, the password is retrieved from LSA; if absent, a NULL password is used.
func configureDDOTServiceCredentials(s *mgr.Service) error {
	coreUser, err := winutil.GetServiceUser(coreAgentService)
	if err != nil || coreUser == "" {
		return fmt.Errorf("DDOT: could not determine core Agent service user: %w", err)
	}

	noChange := uint32(windows.SERVICE_NO_CHANGE)
	acctPtr := windows.StringToUTF16Ptr(coreUser)
	var pwdPtr *uint16

	// Prefer SID-based detection for well-known accounts (locale-independent).
	// If we cannot resolve the SID, fail installation as the environment is not in a stable state.
	sid, errSID := winutil.GetServiceUserSID(coreAgentService)
	if errSID != nil {
		return fmt.Errorf("DDOT: could not resolve SID for service user %q: %w", coreUser, errSID)
	}
	if windowsuser.IsSupportedWellKnownAccount(sid) {
		pwdPtr = windows.StringToUTF16Ptr("")
	} else {
		// Retrieve password from LSA; if not present, use NULL (covers gMSA and accounts without password)
		pass, errLSA := windowsuser.GetAgentUserPasswordFromLSA()
		if errLSA != nil && !errors.Is(errLSA, windowsuser.ErrPrivateDataNotFound) {
			return fmt.Errorf("failed to read agent user password from LSA: %w", errLSA)
		}
		if pass != "" {
			pwdPtr = windows.StringToUTF16Ptr(pass)
		} else {
			pwdPtr = nil
		}
	}

	if err := windows.ChangeServiceConfig(s.Handle, noChange, noChange, noChange, nil, nil, nil, nil, acctPtr, pwdPtr, nil); err != nil {
		log.Warnf("DDOT: failed to set credentials for %q to %q: %v", otelServiceName, coreUser, err)
		return nil
	}
	return nil
}

// configureDDOTServicePermissions sets the DDOT service DACL to match MSI semantics used
// for other Agent services:
// - Grants the core Agent service user (dd-agent) SERVICE_START | SERVICE_STOP | GENERIC_READ.
// - Retains full control for LocalSystem (SY) and Builtin Administrators (BA).
// - Removes permissive access for Everyone by not including a WD ACE.
// Notes:
//   - We apply the DACL via SDDL in one call (replace-style). This mirrors MSI intent without
//     requiring a read/modify/write of the existing ACL.
//   - We do NOT mark the DACL as protected (no D:P), so inheritance is not forcibly blocked.
//   - This alignment is implemented here (Go) because the DDOT service is delivered via OCI,
//     outside the MSI service tables where other services receive their ACLs.
func configureDDOTServicePermissions(s *mgr.Service) {
	// Resolve the core Agent service account SID (locale-independent). This SID is used
	// to grant the dd-agent user explicit start/stop/read access on the DDOT service.
	sid, err := winutil.GetServiceUserSID(coreAgentService)
	if err != nil || sid == nil {
		log.Warnf("DDOT: could not resolve SID for core Agent user to set service DACL: %v", err)
		return
	}
	sidStr := sid.String()
	if sidStr == "" {
		log.Warnf("DDOT: could not stringify SID for core Agent user")
		return
	}

	// If the core Agent runs as LocalSystem, SY already has full control on services.
	// No additional ACEs are required; leave the service DACL unchanged.
	lsSid, err2 := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err2 == nil && windows.EqualSid(sid, lsSid) {
		return
	}

	// MSI parity SDDL:
	// - (A;;GA;;;SY)   LocalSystem full control
	// - (A;;GA;;;BA)   Builtin Administrators full control
	// - (A;;0x80000030;;;<dd-agent SID>) dd-agent: START|STOP|GENERIC_READ
	//   0x80000030 = SERVICE_START (0x10) | SERVICE_STOP (0x20) | GENERIC_READ (0x80000000)
	//   We intentionally omit Everyone (WD).
	sddl := fmt.Sprintf(
		`D:(A;;GA;;;SY)(A;;GA;;;BA)(A;;0x80000030;;;%s)`,
		sidStr,
	)

	// Convert SDDL, extract the DACL and apply it to the service using SetNamedSecurityInfo.
	// Any failure logs a warning and returns; permissions alignment is best-effort and
	// should not block installation or service availability.
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil || sd == nil {
		log.Warnf("DDOT: failed to convert SDDL for service DACL: %v", err)
		return
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		log.Warnf("DDOT: failed to retrieve DACL from security descriptor: %v", err)
		return
	}
	if err := windows.SetNamedSecurityInfo(
		s.Name,
		windows.SE_SERVICE,
		windows.DACL_SECURITY_INFORMATION,
		nil, // owner unchanged
		nil, // group unchanged
		dacl,
		nil, // SACL unchanged
	); err != nil {
		log.Warnf("DDOT: failed to set service DACL for %q: %v", s.Name, err)
		return
	}
}

// setDDOTServiceEnvVars writes the DDOT service environment variables to the registry.
func setDDOTServiceEnvVars() error {
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Services\`+otelServiceName,
		registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("failed to open service registry key: %w", err)
	}
	defer key.Close()
	return key.SetStringsValue("Environment", []string{"DD_OTELCOLLECTOR_INSTALLATION_METHOD=bare-metal"})
}

// stopServiceIfExists stops the service if it exists
func stopServiceIfExists(name string) error {
	// Use robust stop; ignore 'service does not exist'
	if err := winutil.StopService(name); err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil
		}
		return err
	}
	return nil
}

// startServiceIfExists starts the service if it exists
func startServiceIfExists(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return nil
	}
	defer s.Close()
	return s.Start()
}

// deleteServiceIfExists deletes the service if it exists
func deleteServiceIfExists(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return nil
	}
	defer s.Close()
	return s.Delete()
}

//////////////////////////////
/// DDOT EXTENSION METHODS ///
//////////////////////////////

// preInstallDDOTExtension stops the existing DDOT service before extension installation
func preInstallDDOTExtension(_ HookContext) error {
	// Best effort stop and delete existing service
	_ = stopServiceIfExists(otelServiceName)
	_ = deleteServiceIfExists(otelServiceName)
	return nil
}

// postInstallDDOTExtension sets up the DDOT extension after files are extracted
func postInstallDDOTExtension(ctx HookContext) error {
	// Resolve the package path symlink to the real versioned directory.
	// ctx.PackagePath may point to a "stable" or "experiment" symlink; using the
	// versioned path ensures the service binary path remains valid after the symlink
	// is updated on promote or stop-experiment.
	packagePath, err := filepath.EvalSymlinks(ctx.PackagePath)
	if err != nil {
		packagePath = ctx.PackagePath
	}
	extensionPath := filepath.Join(packagePath, "ext", ctx.Extension)

	if err := writeOTelConfigWindowsExtension(ctx, extensionPath); err != nil {
		return fmt.Errorf("failed to write otel-config.yaml: %w", err)
	}

	if err := enableOtelCollectorConfigWindows(ctx); err != nil {
		return fmt.Errorf("failed to enable otelcollector: %w", err)
	}

	binaryPath := filepath.Join(extensionPath, "embedded", "bin", "otel-agent.exe")
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("DDOT binary not found at %s: %w", binaryPath, err)
	}

	procmgrEnabled, err := processManagerEnabledFromDatadogYAML()
	if err != nil {
		log.Warnf("DDOT: could not read process_manager from datadog.yaml (%v); not writing processes.d", err)
		procmgrEnabled = false
	}
	if procmgrEnabled {
		if err := writeDDOTProcmgrConfigWindows(packagePath); err != nil {
			return fmt.Errorf("failed to write DDOT process manager config: %w", err)
		}
	} else if err := removeDDOTProcmgrConfigWindows(packagePath); err != nil {
		log.Warnf("DDOT: could not remove stale process manager config: %v", err)
	}

	// Register the legacy SCM service (Manual, not started) so operators can fall back without reinstalling.
	if err := ensureDDOTServiceForExtension(binaryPath); err != nil {
		return fmt.Errorf("failed to register DDOT Windows service for rollback: %w", err)
	}
	if err := stopServiceIfExists(otelServiceName); err != nil {
		log.Warnf("DDOT: could not ensure %s is stopped after registration: %v", otelServiceName, err)
	}
	// No RestartService(dd-procmgr-service) here: InstallExtensions restarts Datadog agent services
	// after all extension hooks return, which reloads dd-procmgr and picks up processes.d.
	return nil
}

// preRemoveDDOTExtension removes DDOT process manager config, restarts dd-procmgr-service to drop
// supervised DDOT, then stops/removes the legacy SCM entry and disables otelcollector in datadog.yaml.
func preRemoveDDOTExtension(ctx HookContext) error {
	procmgrEnabled, err := processManagerEnabledFromDatadogYAML()
	if err != nil {
		log.Warnf("DDOT: could not read process_manager from datadog.yaml (%v); skipping dd-procmgr restart after config removal", err)
		procmgrEnabled = false
	}
	packagePath := ctx.PackagePath
	if resolved, err := filepath.EvalSymlinks(ctx.PackagePath); err == nil {
		packagePath = resolved
	}
	if err := removeDDOTProcmgrConfigWindows(packagePath); err != nil {
		log.Warnf("failed to remove DDOT process manager config: %v", err)
	}
	if procmgrEnabled {
		if err := winutil.RestartServiceWithTimeout(ddProcmgrServiceName, 120*time.Second); err != nil {
			log.Warnf("DDOT: failed to restart %s after removing DDOT process manager config: %v", ddProcmgrServiceName, err)
		}
	}
	if err := stopServiceIfExists(otelServiceName); err != nil {
		log.Warnf("failed to stop DDOT service: %s", err)
	}
	if err := deleteServiceIfExists(otelServiceName); err != nil {
		log.Warnf("failed to delete DDOT service: %s", err)
	}
	if err := disableOtelCollectorConfigWindows(); err != nil {
		log.Warnf("failed to disable otelcollector in datadog.yaml: %s", err)
	}
	return nil
}

// writeOTelConfigWindowsExtension writes otel-config.yaml for extension
func writeOTelConfigWindowsExtension(ctx HookContext, extensionPath string) error {
	ddYaml := filepath.Join(paths.DatadogDataDir, "datadog.yaml")
	templatePath := filepath.Join(extensionPath, "etc", "datadog-agent", "otel-config.yaml.example")
	outPath := filepath.Join(paths.DatadogDataDir, "otel-config.yaml")
	return writeOTelConfigCommon(ctx, ddYaml, templatePath, outPath, true, 0o640)
}

// writeDDOTProcmgrBaselineEnvWindows writes systemd-style KEY=value lines for a minimal Windows
// environment (PATH, SystemRoot, …) from the current process. dd-procmgr clears the child env;
// processes.d environment_file merges this file so otel-agent behaves like an SCM-started service.
func writeDDOTProcmgrBaselineEnvWindows(etcDir string) error {
	if err := os.MkdirAll(etcDir, 0o755); err != nil {
		return fmt.Errorf("create etc dir for baseline env: %w", err)
	}
	var b strings.Builder
	for _, k := range windowsProcmgrBaselineEnvKeys {
		if v, ok := os.LookupEnv(k); ok && v != "" {
			fmt.Fprintf(&b, "%s=%s\n", k, v)
		}
	}
	p := filepath.Join(etcDir, ddotProcmgrBaselineEnvFileName)
	return os.WriteFile(p, []byte(b.String()), 0o644)
}

// writeDDOTProcmgrConfigWindows writes datadog-agent-ddot.yaml next to the MSI install layout so
// dd-procmgrd picks it up (default_config_dir is InstallPath\processes.d on Windows).
func writeDDOTProcmgrConfigWindows(installRootResolved string) error {
	otelExe := filepath.Join(installRootResolved, "ext", "ddot", "embedded", "bin", "otel-agent.exe")
	if _, err := os.Stat(otelExe); err != nil {
		return nil
	}
	installPF := paths.DatadogProgramFilesDir
	if installPF == "" {
		return errors.New("DatadogProgramFilesDir is empty; cannot write processes.d for DDOT")
	}
	processesDir := filepath.Join(installPF, "processes.d")
	if err := os.MkdirAll(processesDir, 0o755); err != nil {
		return fmt.Errorf("create processes.d: %w", err)
	}

	fleetPolicies := filepath.Join(paths.ConfigsPath, "datadog-agent", "stable")

	config := embedded.DDOTWindowsProcmgrConfig
	config = strings.ReplaceAll(config, "__DDOT_INSTALL_ROOT__", filepath.ToSlash(filepath.Clean(installRootResolved)))
	config = strings.ReplaceAll(config, "__DDOT_ETC_ROOT__", filepath.ToSlash(filepath.Clean(paths.DatadogDataDir)))
	config = strings.ReplaceAll(config, "__DDOT_FLEET_POLICIES_DIR__", filepath.ToSlash(filepath.Clean(fleetPolicies)))

	if err := writeDDOTProcmgrBaselineEnvWindows(paths.DatadogDataDir); err != nil {
		return fmt.Errorf("write DDOT procmgr baseline env: %w", err)
	}

	path := filepath.Join(processesDir, ddotProcmgrConfigFileName)
	return os.WriteFile(path, []byte(config), 0o644)
}

func removeDDOTProcmgrConfigWindows(packageRootResolved string) error {
	if installPF := paths.DatadogProgramFilesDir; installPF != "" {
		p := filepath.Join(installPF, "processes.d", ddotProcmgrConfigFileName)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if paths.DatadogDataDir != "" {
		_ = os.Remove(filepath.Join(paths.DatadogDataDir, ddotProcmgrBaselineEnvFileName))
	}
	legacy := filepath.Join(packageRootResolved, "processes.d", ddotProcmgrConfigFileName)
	if err := os.Remove(legacy); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ensureDDOTServiceForExtension registers or updates the datadog-otel-agent Windows service as
// Manual start. It never starts the service — default supervision is dd-procmgrd; the SCM entry
// exists for optional rollback (e.g. stop procmgr-managed DDOT, then Start-Service).
func ensureDDOTServiceForExtension(binaryPath string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(otelServiceName)
	if err == nil {
		defer s.Close()
		cfg, errC := s.Config()
		if errC != nil {
			return errC
		}
		changed := false
		if cfg.BinaryPathName != binaryPath {
			cfg.BinaryPathName = binaryPath
			changed = true
		}
		if cfg.StartType != mgr.StartManual {
			cfg.StartType = mgr.StartManual
			changed = true
		}
		if len(cfg.Dependencies) > 0 {
			cfg.Dependencies = nil
			changed = true
		}
		if changed {
			if errU := s.UpdateConfig(cfg); errU != nil {
				return errU
			}
		}
		if err := configureDDOTServiceCredentials(s); err != nil {
			return err
		}
		configureDDOTServicePermissions(s)
		if err := setDDOTServiceEnvVars(); err != nil {
			return err
		}
		return nil
	}

	s, err = m.CreateService(otelServiceName, binaryPath, mgr.Config{
		DisplayName:      "Datadog Distribution of OpenTelemetry Collector",
		Description:      "Datadog OpenTelemetry Collector",
		StartType:        mgr.StartManual,
		ServiceStartName: "",
	})
	if err != nil {
		return err
	}
	defer s.Close()
	if err := configureDDOTServiceCredentials(s); err != nil {
		return err
	}
	configureDDOTServicePermissions(s)
	if err := setDDOTServiceEnvVars(); err != nil {
		return err
	}
	return nil
}
