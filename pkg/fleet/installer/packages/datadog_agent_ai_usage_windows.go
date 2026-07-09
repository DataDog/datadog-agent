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
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// AI Usage Chrome Native Messaging host / desktop monitor extension.
//
// This extension replaces the setup that used to be performed by the Windows MSI
// (see tools/windows/DatadogAgentInstaller). It lays down, from the extracted
// extension layer (ext/ai-usage):
//   - the two Chrome NativeMessagingHosts HKLM registry entries (native + WOW6432Node),
//   - the generated ai_usage_native_host.yaml config (with trace_agent_url pointing at
//     the local trace receiver),
//   - the Chrome host manifest JSON (with the resolved chrome_extension_id),
//   - the "Datadog AI Usage Agent" scheduled task (logon-triggered desktop monitor).
const (
	// aiUsageNativeHostName is the Chrome native messaging host name. It is used for the
	// HKLM registry key, the manifest filename, and the manifest "name" field.
	aiUsageNativeHostName = "com.datadoghq.ai_usage_agent.native_host"
	// aiUsageObsoleteNativeHostName is a previous host name whose manifest is cleaned up.
	aiUsageObsoleteNativeHostName = "com.datadoghq.ai_prompt_logger.native_host"
	// aiUsageFallbackChromeExtensionID is used when no chrome_extension_id override is
	// configured in ai_usage_native_host.yaml(.example).
	aiUsageFallbackChromeExtensionID = "gkmbhgbippkmmmidcikijiblbagbjgjj"

	aiUsageBinaryName = "ai-usage-agent-native-host.exe"
	aiUsageConfigName = "ai_usage_native_host.yaml"

	aiUsageTaskName        = "Datadog AI Usage Agent"
	aiUsageTaskDescription = "Starts the Datadog AI Usage Agent desktop monitor in the interactive user session."
	// aiUsageUsersGroupSID is BUILTIN\Users. The desktop monitor task runs in the
	// interactive user session at LeastPrivilege, and Chrome launches the native host
	// as the browser user, so BUILTIN\Users needs read/execute access.
	aiUsageUsersGroupSID = "S-1-5-32-545"

	aiUsageDefaultReceiverPort = 8126

	aiUsageBinaryReplaceRetryInterval   = 500 * time.Millisecond
	aiUsageBinaryReplaceMaxElapsedTime  = 30 * time.Second
	aiUsageHostProcessTerminationWait   = 5 * time.Second
	aiUsageHostProcessTerminationStatus = 1

	// aiUsageChromeRegKeyPath and aiUsageChromeRegKeyPathWow are the machine-wide Chrome
	// NativeMessagingHosts registration keys. Chrome reads the (default) value to find the
	// host manifest JSON.
	aiUsageChromeRegKeyPath    = `Software\Google\Chrome\NativeMessagingHosts\` + aiUsageNativeHostName
	aiUsageChromeRegKeyPathWow = `Software\WOW6432Node\Google\Chrome\NativeMessagingHosts\` + aiUsageNativeHostName
)

// aiUsageTraceAgentURLRe matches the trace_agent_url line (commented or not) in the
// ai_usage_native_host.yaml.example template.
var aiUsageTraceAgentURLRe = regexp.MustCompile(`(?m)^[ #]*trace_agent_url:.*$`)

// aiUsageNativeHostConfig is the subset of ai_usage_native_host.yaml we need to read.
type aiUsageNativeHostConfig struct {
	ChromeExtensionID string `yaml:"chrome_extension_id"`
}

// aiUsageDatadogConfig is the subset of datadog.yaml we need to read.
type aiUsageDatadogConfig struct {
	APMConfig struct {
		ReceiverPort *int `yaml:"receiver_port"`
	} `yaml:"apm_config"`
}

// aiUsageExtensionPath resolves the versioned extension directory. ctx.PackagePath may be a
// "stable"/"experiment" symlink; resolving it keeps the scheduled-task command and Chrome
// manifest exe path valid after the symlink is repointed on promote/stop-experiment.
func aiUsageExtensionPath(ctx HookContext) string {
	packagePath := ctx.PackagePath
	if resolved, err := filepath.EvalSymlinks(ctx.PackagePath); err == nil {
		packagePath = resolved
	}
	return filepath.Join(packagePath, "ext", ctx.Extension)
}

// preInstallAIUsageExtension removes any stale scheduled task before extension files are laid down.
func preInstallAIUsageExtension(ctx HookContext) error {
	removeAIUsageScheduledTask(ctx.Context)
	return nil
}

// postInstallAIUsageExtension sets up the AI Usage native host after the extension layer is extracted.
func postInstallAIUsageExtension(ctx HookContext) error {
	if paths.DatadogProgramFilesDir == "" {
		return errors.New("cannot install AI Usage extension: Agent install directory is unknown")
	}
	extensionPath := aiUsageExtensionPath(ctx)
	srcBinary := filepath.Join(extensionPath, aiUsageBinaryName)
	if _, err := os.Stat(srcBinary); err != nil {
		return fmt.Errorf("AI Usage native host binary not found at %s: %w", srcBinary, err)
	}

	// 1) Copy the native host binary into the Agent's Program Files bin directory. Chrome and the
	// desktop-monitor task launch it as the interactive user, and Program Files grants
	// BUILTIN\Users read+execute by default — so the binary must live there rather than under the
	// ACL-restricted installer packages directory.
	binaryPath := filepath.Join(paths.DatadogProgramFilesDir, "bin", "agent", aiUsageBinaryName)
	restoreChromeRegistration := suspendAIUsageChromeNativeHostRegistration()
	chromeRegistrationReplaced := false
	defer func() {
		if !chromeRegistrationReplaced {
			restoreChromeRegistration()
		}
	}()
	stopAIUsageHostProcesses(ctx.Context)
	if err := copyAIUsageFileReplacingHost(ctx.Context, srcBinary, binaryPath, 0o755); err != nil {
		return fmt.Errorf("failed to copy AI Usage native host binary: %w", err)
	}

	// 2) Generate ai_usage_native_host.yaml in ProgramData (best effort; preserve an existing file),
	// then grant BUILTIN\Users read since Chrome launches the host as the browser user.
	configPath := filepath.Join(paths.DatadogDataDir, aiUsageConfigName)
	examplePath := filepath.Join(extensionPath, aiUsageConfigName+".example")
	if err := writeAIUsageConfig(examplePath, configPath); err != nil {
		return fmt.Errorf("failed to write %s: %w", aiUsageConfigName, err)
	}
	if _, err := os.Stat(configPath); err == nil {
		grantAIUsageUsersAccess(ctx.Context, configPath, "(R)")
	}

	// 3) Write the Chrome host manifest JSON next to the binary (bin\agent\dist), pointing at the
	// copied binary. Program Files inheritance makes it user-readable.
	manifestPath := filepath.Join(paths.DatadogProgramFilesDir, "bin", "agent", "dist", aiUsageNativeHostName+".json")
	extensionID := readAIUsageChromeExtensionID(configPath, examplePath)
	if err := writeAIUsageManifest(manifestPath, binaryPath, extensionID); err != nil {
		return fmt.Errorf("failed to write AI Usage native messaging manifest: %w", err)
	}

	// 4) Register the two Chrome NativeMessagingHosts registry entries pointing at the manifest.
	if err := writeAIUsageChromeRegistry(manifestPath); err != nil {
		return fmt.Errorf("failed to register Chrome native messaging host: %w", err)
	}
	chromeRegistrationReplaced = true

	// 5) Register and start the logon-triggered desktop monitor scheduled task.
	if err := configureAIUsageScheduledTask(ctx.Context, binaryPath, configPath); err != nil {
		return fmt.Errorf("failed to configure AI Usage desktop monitor task: %w", err)
	}

	return nil
}

// copyAIUsageFile copies src to dst (creating dst's parent directory), replacing any existing file.
func copyAIUsageFile(src, dst string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, perm)
}

func copyAIUsageFileReplacingHost(ctx context.Context, src, dst string, perm os.FileMode) error {
	if ctx == nil {
		ctx = context.Background()
	}

	deadline := time.Now().Add(aiUsageBinaryReplaceMaxElapsedTime)
	attempts := 0
	for {
		attempts++
		err := copyAIUsageFile(src, dst, perm)
		if err == nil {
			if attempts > 1 {
				log.Infof("AI Usage: copied native host binary after %d attempts", attempts)
			}
			return nil
		}
		if !isRetryableAIUsageBinaryReplaceError(err) || time.Now().After(deadline) {
			return err
		}

		log.Warnf("AI Usage: native host binary is still locked, stopping running hosts before retrying copy: %v", err)
		stopAIUsageHostProcesses(ctx)

		timer := time.NewTimer(aiUsageBinaryReplaceRetryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func isRetryableAIUsageBinaryReplaceError(err error) bool {
	return errors.Is(err, fs.ErrPermission) ||
		errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}

// preRemoveAIUsageExtension tears down the AI Usage native host before extension files are removed.
// All steps are best effort so removal is not blocked.
func preRemoveAIUsageExtension(ctx HookContext) error {
	removeAIUsageScheduledTask(ctx.Context)
	deleteAIUsageChromeRegistry()

	agentBinDir := filepath.Join(paths.DatadogProgramFilesDir, "bin", "agent")
	for _, p := range []string{
		filepath.Join(agentBinDir, "dist", aiUsageNativeHostName+".json"),
		filepath.Join(agentBinDir, aiUsageBinaryName),
	} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			log.Warnf("AI Usage: failed to remove %q: %v", p, err)
		}
	}
	// Preserve the user-editable ai_usage_native_host.yaml (mirrors ddot preserving otel-config.yaml).
	return nil
}

// writeAIUsageConfig renders ai_usage_native_host.yaml from the example template, substituting
// trace_agent_url with the local trace receiver URL. An existing config is preserved.
func writeAIUsageConfig(examplePath, configPath string) error {
	if _, err := os.Stat(configPath); err == nil {
		log.Debugf("AI Usage: %s already exists, not modifying it", configPath)
		return nil
	}
	example, err := os.ReadFile(examplePath)
	if err != nil {
		return fmt.Errorf("could not read example config %s: %w", examplePath, err)
	}
	port := readAIUsageReceiverPort()
	rendered := aiUsageTraceAgentURLRe.ReplaceAllString(
		string(example),
		fmt.Sprintf(`trace_agent_url: "http://127.0.0.1:%d"`, port),
	)
	if err := os.WriteFile(configPath, []byte(rendered), 0o644); err != nil {
		return fmt.Errorf("could not write %s: %w", configPath, err)
	}
	return nil
}

type aiUsageChromeRegistryBackup struct {
	path        string
	exists      bool
	value       string
	valueExists bool
}

func suspendAIUsageChromeNativeHostRegistration() func() {
	backups := make([]aiUsageChromeRegistryBackup, 0, 2)
	for _, path := range []string{aiUsageChromeRegKeyPath, aiUsageChromeRegKeyPathWow} {
		backup := aiUsageChromeRegistryBackup{path: path}
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE|registry.WOW64_64KEY)
		if err == nil {
			backup.exists = true
			if value, _, err := key.GetStringValue(""); err == nil {
				backup.value = value
				backup.valueExists = true
			} else if err != registry.ErrNotExist {
				log.Warnf("AI Usage: failed to read Chrome registry value for %q before replacement: %v", path, err)
			}
			key.Close()
		} else if err != registry.ErrNotExist {
			log.Warnf("AI Usage: failed to read Chrome registry key %q before replacement: %v", path, err)
		}
		backups = append(backups, backup)
	}

	// Prevent Chrome from spawning a fresh host while the old executable is being terminated
	// and replaced. The keys are written again after the new manifest is in place.
	deleteAIUsageChromeRegistry()

	return func() {
		for _, backup := range backups {
			if !backup.exists {
				if err := registry.DeleteKey(registry.LOCAL_MACHINE, backup.path); err != nil && err != registry.ErrNotExist {
					log.Warnf("AI Usage: failed to clear Chrome registry key %q after replacement failure: %v", backup.path, err)
				}
				continue
			}
			key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, backup.path, registry.SET_VALUE|registry.WOW64_64KEY)
			if err != nil {
				log.Warnf("AI Usage: failed to restore Chrome registry key %q after replacement failure: %v", backup.path, err)
				continue
			}
			if backup.valueExists {
				if err := key.SetStringValue("", backup.value); err != nil {
					log.Warnf("AI Usage: failed to restore Chrome registry value for %q after replacement failure: %v", backup.path, err)
				}
			}
			key.Close()
		}
	}
}

func deleteAIUsageChromeRegistry() {
	for _, path := range []string{aiUsageChromeRegKeyPath, aiUsageChromeRegKeyPathWow} {
		if err := registry.DeleteKey(registry.LOCAL_MACHINE, path); err != nil && err != registry.ErrNotExist {
			log.Warnf("AI Usage: failed to delete Chrome registry key %q: %v", path, err)
		}
	}
}

func stopAIUsageHostProcesses(ctx context.Context) {
	pids, err := aiUsageHostProcessIDs()
	if err != nil {
		log.Warnf("AI Usage: failed to enumerate native host processes before replacement: %v", err)
		return
	}
	for _, pid := range pids {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				log.Warnf("AI Usage: context canceled before stopping native host process %d: %v", pid, err)
				return
			}
		}
		if err := stopAIUsageHostProcess(pid); err != nil {
			log.Warnf("AI Usage: failed to stop native host process %d before replacement: %v", pid, err)
		}
	}
}

func aiUsageHostProcessIDs() ([]uint32, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	var pids []uint32
	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), aiUsageBinaryName) {
			pids = append(pids, entry.ProcessID)
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	if err != nil && err != windows.ERROR_NO_MORE_FILES {
		return nil, err
	}
	return pids, nil
}

func stopAIUsageHostProcess(pid uint32) error {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return nil
		}
		return err
	}
	defer windows.CloseHandle(handle)

	if err := windows.TerminateProcess(handle, aiUsageHostProcessTerminationStatus); err != nil {
		return err
	}

	wait, err := windows.WaitForSingleObject(handle, uint32(aiUsageHostProcessTerminationWait/time.Millisecond))
	if err != nil {
		return err
	}
	if wait == windows.WAIT_TIMEOUT {
		return fmt.Errorf("timed out waiting for process exit")
	}
	return nil
}

// readAIUsageReceiverPort reads apm_config.receiver_port from datadog.yaml, defaulting to 8126.
func readAIUsageReceiverPort() int {
	ddYaml := filepath.Join(paths.DatadogDataDir, "datadog.yaml")
	data, err := os.ReadFile(ddYaml)
	if err != nil {
		return aiUsageDefaultReceiverPort
	}
	var cfg aiUsageDatadogConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return aiUsageDefaultReceiverPort
	}
	if cfg.APMConfig.ReceiverPort != nil && *cfg.APMConfig.ReceiverPort > 0 {
		return *cfg.APMConfig.ReceiverPort
	}
	return aiUsageDefaultReceiverPort
}

// readAIUsageChromeExtensionID resolves the Chrome extension ID from the active config, then the
// example, falling back to the compiled-in default.
func readAIUsageChromeExtensionID(configPath, examplePath string) string {
	for _, p := range []string{configPath, examplePath} {
		if id := readAIUsageChromeExtensionIDFromFile(p); id != "" {
			return id
		}
	}
	log.Debugf("AI Usage: no chrome_extension_id override found; using fallback %s", aiUsageFallbackChromeExtensionID)
	return aiUsageFallbackChromeExtensionID
}

func readAIUsageChromeExtensionIDFromFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg aiUsageNativeHostConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.ChromeExtensionID)
}

// writeAIUsageManifest writes the Chrome native messaging host manifest, and cleans up the
// obsolete manifest if present.
func writeAIUsageManifest(manifestPath, hostExe, extensionID string) error {
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return fmt.Errorf("could not create manifest directory: %w", err)
	}
	obsolete := filepath.Join(filepath.Dir(manifestPath), aiUsageObsoleteNativeHostName+".json")
	if err := os.Remove(obsolete); err != nil && !os.IsNotExist(err) {
		log.Warnf("AI Usage: failed to delete obsolete manifest %q: %v", obsolete, err)
	}

	manifest := fmt.Sprintf(`{
  "name": "%s",
  "description": "Datadog AI usage native messaging host",
  "path": "%s",
  "type": "stdio",
  "allowed_origins": [
    "chrome-extension://%s/"
  ]
}
`, aiUsageNativeHostName, jsonEscape(hostExe), jsonEscape(extensionID))
	return os.WriteFile(manifestPath, []byte(manifest), 0o644)
}

// jsonEscape escapes backslashes and double quotes for embedding a string in a JSON literal.
func jsonEscape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

// writeAIUsageChromeRegistry creates the two HKLM NativeMessagingHosts keys, with the (default)
// value set to the manifest path, in the 64-bit registry view.
func writeAIUsageChromeRegistry(manifestPath string) error {
	for _, path := range []string{aiUsageChromeRegKeyPath, aiUsageChromeRegKeyPathWow} {
		key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, path, registry.SET_VALUE|registry.WOW64_64KEY)
		if err != nil {
			return fmt.Errorf("could not create registry key %q: %w", path, err)
		}
		if err := key.SetStringValue("", manifestPath); err != nil {
			key.Close()
			return fmt.Errorf("could not set registry value for %q: %w", path, err)
		}
		key.Close()
	}
	return nil
}

// grantAIUsageUsersAccess grants BUILTIN\Users the given icacls permission set on path (best effort).
// icacls /grant merges the ACE with the existing DACL rather than replacing it.
func grantAIUsageUsersAccess(ctx context.Context, path, perms string) {
	icacls := filepath.Join(os.Getenv("SystemRoot"), "System32", "icacls.exe")
	// e.g. icacls "<path>" /grant "*S-1-5-32-545:(RX)"
	if out, err := exec.CommandContext(ctx, icacls, path, "/grant", fmt.Sprintf("*%s:%s", aiUsageUsersGroupSID, perms)).CombinedOutput(); err != nil {
		log.Warnf("AI Usage: failed to grant Users access on %q: %v (%s)", path, err, strings.TrimSpace(string(out)))
	}
}

// configureAIUsageScheduledTask registers and starts the logon-triggered desktop monitor task.
func configureAIUsageScheduledTask(ctx context.Context, hostPath, configPath string) error {
	tmp, err := os.CreateTemp("", "datadog-ai-usage-agent-*.xml")
	if err != nil {
		return fmt.Errorf("could not create temporary task XML: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			log.Warnf("AI Usage: failed to remove temporary task XML %q: %v", tmpPath, err)
		}
	}()
	// Task Scheduler XML must be UTF-16LE with a BOM.
	xml := buildAIUsageTaskXML(hostPath, configPath)
	if _, err := tmp.Write(utf16LEWithBOM(xml)); err != nil {
		tmp.Close()
		return fmt.Errorf("could not write task XML: %w", err)
	}
	tmp.Close()

	schtasks := filepath.Join(os.Getenv("SystemRoot"), "System32", "schtasks.exe")
	if out, err := exec.CommandContext(ctx, schtasks, "/Create", "/TN", aiUsageTaskName, "/XML", tmpPath, "/F").CombinedOutput(); err != nil {
		return fmt.Errorf("schtasks /Create failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	// Start it now (best effort — it will otherwise start at next logon).
	if out, err := exec.CommandContext(ctx, schtasks, "/Run", "/TN", aiUsageTaskName).CombinedOutput(); err != nil {
		log.Warnf("AI Usage: schtasks /Run failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// removeAIUsageScheduledTask ends and deletes the desktop monitor task (best effort).
func removeAIUsageScheduledTask(ctx context.Context) {
	schtasks := filepath.Join(os.Getenv("SystemRoot"), "System32", "schtasks.exe")
	_ = exec.CommandContext(ctx, schtasks, "/End", "/TN", aiUsageTaskName).Run()
	_ = exec.CommandContext(ctx, schtasks, "/Delete", "/TN", aiUsageTaskName, "/F").Run()
}

// buildAIUsageTaskXML builds the Task Scheduler definition for the desktop monitor: a
// logon-triggered task running as BUILTIN\Users at least privilege, parallel instances,
// restart-on-failure, no execution time limit.
func buildAIUsageTaskXML(hostPath, configPath string) string {
	return `<?xml version="1.0" encoding="UTF-16"?>` +
		`<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">` +
		`<RegistrationInfo><Author>Datadog</Author><Description>` + xmlEscape(aiUsageTaskDescription) + `</Description></RegistrationInfo>` +
		`<Triggers><LogonTrigger><Enabled>true</Enabled></LogonTrigger></Triggers>` +
		`<Principals><Principal id="Author"><GroupId>` + aiUsageUsersGroupSID + `</GroupId><RunLevel>LeastPrivilege</RunLevel></Principal></Principals>` +
		`<Settings>` +
		`<MultipleInstancesPolicy>Parallel</MultipleInstancesPolicy>` +
		`<DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>` +
		`<StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>` +
		`<AllowHardTerminate>true</AllowHardTerminate>` +
		`<StartWhenAvailable>false</StartWhenAvailable>` +
		`<RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>` +
		`<AllowStartOnDemand>true</AllowStartOnDemand>` +
		`<Enabled>true</Enabled>` +
		`<Hidden>false</Hidden>` +
		`<RunOnlyIfIdle>false</RunOnlyIfIdle>` +
		`<WakeToRun>false</WakeToRun>` +
		`<RestartOnFailure><Interval>PT1M</Interval><Count>3</Count></RestartOnFailure>` +
		`<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>` +
		`<Priority>7</Priority>` +
		`</Settings>` +
		`<Actions Context="Author"><Exec><Command>` + xmlEscape(hostPath) + `</Command>` +
		`<Arguments>` + xmlEscape(`--desktop-monitor --config "`+configPath+`"`) + `</Arguments></Exec></Actions>` +
		`</Task>`
}

// xmlEscape escapes the five XML predefined entities.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// utf16LEWithBOM encodes s as UTF-16LE with a leading byte-order mark, as required by
// schtasks /XML.
func utf16LEWithBOM(s string) []byte {
	runes := utf16.Encode([]rune(s))
	out := make([]byte, 0, 2+len(runes)*2)
	out = append(out, 0xFF, 0xFE) // UTF-16LE BOM
	for _, r := range runes {
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}
