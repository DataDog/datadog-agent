// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package packages

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unsafe"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	textunicode "golang.org/x/text/encoding/unicode"
)

// End User Device Monitoring (eudm) extension.
// Features:
// - AI Usage Chrome Native Messaging host / desktop monitor
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
	// aiUsageUsersGroupSID is BUILTIN\Users, the principal the desktop monitor scheduled task
	// runs as (in the interactive user session at LeastPrivilege).
	aiUsageUsersGroupSID = "S-1-5-32-545"

	aiUsageDefaultReceiverPort = 8126

	aiUsageHostProcessTerminationWait   = 5 * time.Second
	aiUsageHostProcessTerminationStatus = 1
	aiUsageTaskXMLNamespace             = "http://schemas.microsoft.com/windows/2004/02/mit/task"

	// aiUsageChromeRegKeyPath and aiUsageChromeRegKeyPathWow are the machine-wide Chrome
	// NativeMessagingHosts registration keys. Chrome reads the (default) value to find the
	// host manifest JSON.
	aiUsageChromeRegKeyPath    = `Software\Google\Chrome\NativeMessagingHosts\` + aiUsageNativeHostName
	aiUsageChromeRegKeyPathWow = `Software\WOW6432Node\Google\Chrome\NativeMessagingHosts\` + aiUsageNativeHostName
)

// aiUsageChromeRegKeyPaths are the two machine-wide Chrome NativeMessagingHosts registration
// keys (native and WOW6432Node) that are created, read, and deleted together.
var aiUsageChromeRegKeyPaths = []string{aiUsageChromeRegKeyPath, aiUsageChromeRegKeyPathWow}

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

// preInstallEUDMExtension quiesces the AI Usage host before the extension layer is extracted: it
// clears the machine-wide Chrome registration (so Chrome does not spawn the old host while the new
// layer is being laid down), removes the stale scheduled task, and stops any running host
// processes. installSingle always runs this immediately before postInstallEUDMExtension.
func preInstallEUDMExtension(ctx HookContext) error {
	deleteAIUsageChromeRegistry()
	removeAIUsageScheduledTask(ctx.Context)
	stopAIUsageHostProcesses(ctx.Context)
	return nil
}

// postInstallEUDMExtension sets up the eudm extension's AI Usage native host after the layer is extracted.
func postInstallEUDMExtension(ctx HookContext) error {
	if paths.DatadogDataDir == "" {
		return errors.New("cannot install AI Usage extension: Agent data directory is unknown")
	}
	extensionPath := aiUsageExtensionPath(ctx)

	// The native host runs in place from the extracted extension layer. That directory is world
	// read/execute, so Chrome and the desktop-monitor task (which launch it as the interactive
	// browser user) can execute it without copying it elsewhere or changing ACLs. The generated
	// manifest is written into the same layer dir. postInstall re-runs on every upgrade, repointing
	// the manifest/registration/task at the new layer, so the versioned path stays valid.
	binaryPath := filepath.Join(extensionPath, aiUsageBinaryName)
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("AI Usage native host binary not found at %s: %w", binaryPath, err)
	}
	manifestPath := filepath.Join(extensionPath, aiUsageNativeHostName+".json")

	// The Chrome registration was already cleared in preInstallEUDMExtension (which installSingle
	// runs immediately before this hook); it is rewritten in step 3 once the manifest is in place.
	success := false
	defer func() {
		if success {
			return
		}
		// The hook failed. installSingle removes the extracted extension directory (binary +
		// manifest) on error, so here we only undo the machine-wide state: the Chrome registration
		// and the scheduled task.
		deleteAIUsageChromeRegistry()
		removeAIUsageScheduledTask(ctx.Context)
	}()

	// 1) Generate ai_usage_native_host.yaml in ProgramData (best effort; preserve an existing file),
	// then grant Everyone read/execute on it. C:\ProgramData\Datadog is ACL-restricted, so the
	// config would otherwise be unreadable by the interactive browser user that Chrome and the
	// desktop-monitor task launch the host as.
	configPath := filepath.Join(paths.DatadogDataDir, aiUsageConfigName)
	examplePath := filepath.Join(extensionPath, aiUsageConfigName+".example")
	if err := writeAIUsageConfig(examplePath, configPath); err != nil {
		return fmt.Errorf("failed to write %s: %w", aiUsageConfigName, err)
	}
	if err := grantAIUsageConfigWorldRead(configPath); err != nil {
		log.Warnf("AI Usage: failed to grant read access on %q: %v", configPath, err)
	}
	// Also lay down the editable sample config next to the active one (mirrors what the MSI used
	// to install at C:\ProgramData\Datadog\ai_usage_native_host.yaml.example). Best effort.
	if err := copyAIUsageFile(examplePath, configPath+".example", 0o644); err != nil {
		log.Warnf("AI Usage: failed to copy example config to %q: %v", configPath+".example", err)
	}

	// 2) Write the Chrome host manifest JSON into the extension layer, pointing at the host binary.
	extensionID := readAIUsageChromeExtensionID(configPath, examplePath)
	if err := writeAIUsageManifest(manifestPath, binaryPath, extensionID); err != nil {
		return fmt.Errorf("failed to write AI Usage native messaging manifest: %w", err)
	}

	// 3) Register the two Chrome NativeMessagingHosts registry entries pointing at the manifest.
	if err := writeAIUsageChromeRegistry(manifestPath); err != nil {
		return fmt.Errorf("failed to register Chrome native messaging host: %w", err)
	}

	// 4) Register and start the logon-triggered desktop monitor scheduled task.
	if err := configureAIUsageScheduledTask(ctx.Context, binaryPath, configPath); err != nil {
		return fmt.Errorf("failed to configure AI Usage desktop monitor task: %w", err)
	}

	success = true
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

// preRemoveEUDMExtension tears down the AI Usage native host before extension files are removed.
// All steps are best effort so removal is not blocked.
func preRemoveEUDMExtension(ctx HookContext) error {
	// Tear down the machine-wide state and stop the running host. The binary and manifest live in
	// the extension layer directory, which is removed with the extension itself, so there are no
	// extra files to clean up here. The user-editable ai_usage_native_host.yaml under
	// ProgramData\Datadog is preserved (mirrors ddot preserving otel-config.yaml).
	removeAIUsageScheduledTask(ctx.Context)
	deleteAIUsageChromeRegistry()
	stopAIUsageHostProcesses(ctx.Context)
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

// grantAIUsageConfigWorldRead sets an explicit DACL on the config so Everyone can read/execute it
// while SYSTEM and Administrators keep full control. C:\ProgramData\Datadog is ACL-restricted, so
// the config must be granted read access explicitly for the native host (which Chrome and the
// desktop-monitor task launch as the interactive browser user) to read it. Mirrors the SDDL +
// SetNamedSecurityInfo approach used for the DDOT service (no icacls shell-out).
func grantAIUsageConfigWorldRead(path string) error {
	// SY = SYSTEM, BA = BUILTIN\Administrators (GA = full control); WD = Everyone (GR|GX = generic
	// read + execute).
	const sddl = `D:(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGX;;;WD)`
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("could not parse SDDL: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("could not extract DACL: %w", err)
	}
	// PROTECTED_DACL_SECURITY_INFORMATION drops the restrictive inherited ACEs so only the explicit
	// entries above apply.
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
}

func deleteAIUsageChromeRegistry() {
	for _, path := range aiUsageChromeRegKeyPaths {
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
	if wait == uint32(windows.WAIT_TIMEOUT) {
		return errors.New("timed out waiting for process exit")
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

	manifest := aiUsageChromeManifest{
		Name:           aiUsageNativeHostName,
		Description:    "Datadog AI usage native messaging host",
		Path:           hostExe,
		Type:           "stdio",
		AllowedOrigins: []string{fmt.Sprintf("chrome-extension://%s/", extensionID)},
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal AI Usage manifest: %w", err)
	}
	return os.WriteFile(manifestPath, append(data, '\n'), 0o644)
}

// aiUsageChromeManifest is the Chrome native messaging host manifest.
type aiUsageChromeManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

// writeAIUsageChromeRegistry creates the two HKLM NativeMessagingHosts keys, with the (default)
// value set to the manifest path, in the 64-bit registry view.
func writeAIUsageChromeRegistry(manifestPath string) error {
	for _, path := range aiUsageChromeRegKeyPaths {
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
	taskXML, err := buildAIUsageTaskXML(hostPath, configPath)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("could not build task XML: %w", err)
	}
	encodedXML, err := encodeAIUsageTaskXML(taskXML)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("could not encode task XML: %w", err)
	}
	if _, err := tmp.Write(encodedXML); err != nil {
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
func buildAIUsageTaskXML(hostPath, configPath string) (string, error) {
	task := aiUsageScheduledTaskXML{
		Version: "1.4",
		XMLNS:   aiUsageTaskXMLNamespace,
		RegistrationInfo: aiUsageTaskRegistrationInfo{
			Author:      "Datadog",
			Description: aiUsageTaskDescription,
		},
		Triggers: aiUsageTaskTriggers{
			LogonTrigger: aiUsageTaskLogonTrigger{Enabled: true},
		},
		Principals: aiUsageTaskPrincipals{
			Principal: aiUsageTaskPrincipal{
				ID:       "Author",
				GroupID:  aiUsageUsersGroupSID,
				RunLevel: "LeastPrivilege",
			},
		},
		Settings: aiUsageTaskSettings{
			MultipleInstancesPolicy:    "Parallel",
			DisallowStartIfOnBatteries: false,
			StopIfGoingOnBatteries:     false,
			AllowHardTerminate:         true,
			StartWhenAvailable:         false,
			RunOnlyIfNetworkAvailable:  false,
			AllowStartOnDemand:         true,
			Enabled:                    true,
			Hidden:                     false,
			RunOnlyIfIdle:              false,
			WakeToRun:                  false,
			RestartOnFailure: aiUsageTaskRestartOnFailure{
				Interval: "PT1M",
				Count:    3,
			},
			ExecutionTimeLimit: "PT0S",
			Priority:           7,
		},
		Actions: aiUsageTaskActions{
			Context: "Author",
			Exec: aiUsageTaskExec{
				Command:   hostPath,
				Arguments: `--desktop-monitor --config "` + configPath + `"`,
			},
		},
	}

	data, err := xml.Marshal(task)
	if err != nil {
		return "", err
	}
	return `<?xml version="1.0" encoding="UTF-16"?>` + string(data), nil
}

// encodeAIUsageTaskXML encodes Task Scheduler XML as UTF-16LE with a BOM.
func encodeAIUsageTaskXML(taskXML string) ([]byte, error) {
	return textunicode.UTF16(textunicode.LittleEndian, textunicode.UseBOM).NewEncoder().Bytes([]byte(taskXML))
}

type aiUsageScheduledTaskXML struct {
	XMLName          xml.Name                    `xml:"Task"`
	Version          string                      `xml:"version,attr"`
	XMLNS            string                      `xml:"xmlns,attr"`
	RegistrationInfo aiUsageTaskRegistrationInfo `xml:"RegistrationInfo"`
	Triggers         aiUsageTaskTriggers         `xml:"Triggers"`
	Principals       aiUsageTaskPrincipals       `xml:"Principals"`
	Settings         aiUsageTaskSettings         `xml:"Settings"`
	Actions          aiUsageTaskActions          `xml:"Actions"`
}

type aiUsageTaskRegistrationInfo struct {
	Author      string `xml:"Author"`
	Description string `xml:"Description"`
}

type aiUsageTaskTriggers struct {
	LogonTrigger aiUsageTaskLogonTrigger `xml:"LogonTrigger"`
}

type aiUsageTaskLogonTrigger struct {
	Enabled bool `xml:"Enabled"`
}

type aiUsageTaskPrincipals struct {
	Principal aiUsageTaskPrincipal `xml:"Principal"`
}

type aiUsageTaskPrincipal struct {
	ID       string `xml:"id,attr"`
	GroupID  string `xml:"GroupId"`
	RunLevel string `xml:"RunLevel"`
}

type aiUsageTaskSettings struct {
	MultipleInstancesPolicy    string                      `xml:"MultipleInstancesPolicy"`
	DisallowStartIfOnBatteries bool                        `xml:"DisallowStartIfOnBatteries"`
	StopIfGoingOnBatteries     bool                        `xml:"StopIfGoingOnBatteries"`
	AllowHardTerminate         bool                        `xml:"AllowHardTerminate"`
	StartWhenAvailable         bool                        `xml:"StartWhenAvailable"`
	RunOnlyIfNetworkAvailable  bool                        `xml:"RunOnlyIfNetworkAvailable"`
	AllowStartOnDemand         bool                        `xml:"AllowStartOnDemand"`
	Enabled                    bool                        `xml:"Enabled"`
	Hidden                     bool                        `xml:"Hidden"`
	RunOnlyIfIdle              bool                        `xml:"RunOnlyIfIdle"`
	WakeToRun                  bool                        `xml:"WakeToRun"`
	RestartOnFailure           aiUsageTaskRestartOnFailure `xml:"RestartOnFailure"`
	ExecutionTimeLimit         string                      `xml:"ExecutionTimeLimit"`
	Priority                   int                         `xml:"Priority"`
}

type aiUsageTaskRestartOnFailure struct {
	Interval string `xml:"Interval"`
	Count    int    `xml:"Count"`
}

type aiUsageTaskActions struct {
	Context string          `xml:"Context,attr"`
	Exec    aiUsageTaskExec `xml:"Exec"`
}

type aiUsageTaskExec struct {
	Command   string `xml:"Command"`
	Arguments string `xml:"Arguments"`
}
