// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyInstallEnvLogsEnabled pins how DD_LOGS_ENABLED from install_mac_os.sh lands in
// datadog.yaml: true and false set or replace logs_enabled, commented or not, and anything else
// leaves the configuration as it was.
func TestApplyInstallEnvLogsEnabled(t *testing.T) {
	for _, value := range []string{"true", "false", "", "TRUE", "1", "$(touch injected)", "true/; touch injected"} {
		for _, existing := range []string{"# logs_enabled: false\n", "logs_enabled: false\n", "logs_enabled: true\n", ""} {
			conf := t.TempDir()
			original := "site: datadoghq.com\n" + existing
			require.NoError(t, os.WriteFile(filepath.Join(conf, "datadog.yaml"), []byte(original), 0o600))

			require.NoError(t, applyInstallEnv(conf, installEnv{"DD_LOGS_ENABLED": value}))

			content, err := os.ReadFile(filepath.Join(conf, "datadog.yaml"))
			require.NoError(t, err)
			expected := original
			if value == "true" || value == "false" {
				expected = "site: datadoghq.com\n"
				if existing == "" {
					expected += "\n"
				}
				expected += "logs_enabled: " + value + "\n"
			}
			assert.Equal(t, expected, string(content), "value %q, existing %q", value, existing)
			assert.NoFileExists(t, filepath.Join(conf, "injected"))
		}
	}
}

// TestApplyInstallEnvSetsTheInstallScriptsSettings pins the other settings the install script
// forwards: the API key, the site, the infrastructure mode and the Chrome extension.
func TestApplyInstallEnvSetsTheInstallScriptsSettings(t *testing.T) {
	conf := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(conf, "datadog.yaml"), []byte("api_key: APIKEY\n# site: datadoghq.com\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(conf, "ai_usage_native_host.yaml"), []byte("# chrome_extension_id: abc\n"), 0o600))

	require.NoError(t, applyInstallEnv(conf, installEnv{
		"DD_API_KEY":                      "0123",
		"DD_SITE":                         "datad0g.com",
		"DD_INFRASTRUCTURE_MODE":          "basic",
		"DD_AI_USAGE_CHROME_EXTENSION_ID": "ext",
	}))

	content, err := os.ReadFile(filepath.Join(conf, "datadog.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "api_key: 0123\nsite: datad0g.com\n\ninfrastructure_mode: basic\n", string(content))
	nativeHost, err := os.ReadFile(filepath.Join(conf, "ai_usage_native_host.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "chrome_extension_id: ext\n", string(nativeHost))
}

// TestSetUpConfigOnAFreshInstall pins the first install: datadog.yaml comes from the example with
// the API key placeholder, the native host config points at a custom APM port, and the install info
// names the install method.
func TestSetUpConfigOnAFreshInstall(t *testing.T) {
	layout := stubDmgLayout(t)
	conf := layout.confDir()
	require.NoError(t, os.WriteFile(filepath.Join(conf, "datadog.yaml.example"), []byte("api_key:\napm_config:\n  receiver_port: 8127\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(conf, "ai_usage_native_host.yaml.example"), []byte("# trace_agent_url: http://127.0.0.1:8126\n"), 0o644))

	require.NoError(t, setUpConfig(t.Context(), layout, installEnv{"DD_INSTALL_METHOD": "install_script", "DD_INSTALL_SCRIPT_VERSION": "1.2.3"}))

	datadogYAML, err := os.ReadFile(filepath.Join(conf, "datadog.yaml"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(datadogYAML), "api_key: APIKEY\n"))
	nativeHost, err := os.ReadFile(filepath.Join(conf, "ai_usage_native_host.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "trace_agent_url: \"http://127.0.0.1:8127\"\n", string(nativeHost))
	installInfo, err := os.ReadFile(filepath.Join(conf, "install_info"))
	require.NoError(t, err)
	assert.Equal(t, "---\ninstall_method:\n  tool: install_script\n  tool_version: install_script\n  installer_version: install_script-1.2.3\n\n", string(installInfo))
	assert.DirExists(t, filepath.Join(conf, "checks.d"))
	info, err := os.Stat(filepath.Join(conf, "datadog.yaml"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o660), info.Mode().Perm())
}

// TestSetUpConfigRestoresTheSavedConfiguration pins the upgrade: what the preinstall script saved
// comes back, conf.d over the shipped files and checks.d without replacing anything.
func TestSetUpConfigRestoresTheSavedConfiguration(t *testing.T) {
	layout := stubDmgLayout(t)
	conf, saved := layout.confDir(), layout.savedConfDir()
	for path, content := range map[string]string{
		filepath.Join(saved, "datadog.yaml"):               "api_key: saved\n",
		filepath.Join(saved, "conf.d", "disk.d", "c.yaml"): "saved check\n",
		filepath.Join(saved, "checks.d", "custom.py"):      "saved custom\n",
		filepath.Join(conf, "conf.d", "disk.d", "c.yaml"):  "shipped check\n",
		filepath.Join(conf, "checks.d", "custom.py"):       "newer custom\n",
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	require.NoError(t, setUpConfig(t.Context(), layout, installEnv{}))

	for path, expected := range map[string]string{
		filepath.Join(conf, "datadog.yaml"):               "api_key: saved\n",
		filepath.Join(conf, "conf.d", "disk.d", "c.yaml"): "saved check\n",
		filepath.Join(conf, "checks.d", "custom.py"):      "newer custom\n",
	} {
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, expected, string(content), path)
	}
}

// TestEnsureLogLink pins the /var/log/datadog link: created when missing, an old real directory
// migrated into the install root, and an existing link left alone.
func TestEnsureLogLink(t *testing.T) {
	layout := stubDmgLayout(t)
	require.NoError(t, os.MkdirAll(layout.logDir(), 0o755))

	require.NoError(t, os.MkdirAll(layout.logLink, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(layout.logLink, "agent.log"), []byte("old"), 0o644))
	ensureLogLink(layout)
	target, err := os.Readlink(layout.logLink)
	require.NoError(t, err)
	assert.Equal(t, layout.logDir(), target)
	assert.FileExists(t, filepath.Join(layout.logDir(), "agent.log"))

	ensureLogLink(layout)
	target, err = os.Readlink(layout.logLink)
	require.NoError(t, err)
	assert.Equal(t, layout.logDir(), target)
}

// TestRemoveOldInstallFiles pins the cleanup of what earlier versions left, and that the installer
// daemon's remote config database in the Agent's run directory is kept.
func TestRemoveOldInstallFiles(t *testing.T) {
	layout := stubDmgLayout(t)
	stale := []string{
		filepath.Join(layout.installDir, "agent", "f"),
		filepath.Join(layout.runDir(), "installer.sock"),
		filepath.Join(layout.runDir(), "installer_tasks.db"),
		filepath.Join(layout.packagesDir, "logs", "updater.log"),
		filepath.Join(layout.packagesRunDir, "remote-config-installer.db"),
	}
	kept := filepath.Join(layout.runDir(), "remote-config-installer.db")
	for _, path := range append(stale, kept) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, nil, 0o644))
	}

	removeOldInstallFiles(layout)

	for _, path := range stale {
		assert.NoFileExists(t, path)
	}
	assert.NoDirExists(t, filepath.Join(layout.packagesDir, "logs"))
	assert.FileExists(t, kept)
}

// TestInstallLinks pins the user-facing commands, replacing whatever was there.
func TestInstallLinks(t *testing.T) {
	layout := stubDmgLayout(t)
	require.NoError(t, os.MkdirAll(layout.linkDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(layout.linkDir, "datadog-agent"), []byte("stale"), 0o755))

	installLinks(layout)

	for link, target := range map[string]string{
		"datadog-agent":     filepath.Join(layout.installDir, "bin", "agent", "agent"),
		"datadog-installer": filepath.Join(layout.installDir, "embedded", "bin", "installer"),
	} {
		got, err := os.Readlink(filepath.Join(layout.linkDir, link))
		require.NoError(t, err, link)
		assert.Equal(t, target, got)
	}
}

// TestSetUpDMGInstall runs the whole set-up against a temporary install: the application moves to
// /Applications, the directories and configuration are in place, and the GUI LaunchAgent is written
// with the menu bar icon when the install asked for it.
func TestSetUpDMGInstall(t *testing.T) {
	layout := stubDmgLayout(t)
	require.NoError(t, os.MkdirAll(filepath.Join(layout.installDir, layout.appName()), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(layout.confDir(), guiLabel+".plist.example"),
		[]byte("<array>\n    <string>gui</string>\n    <string>--headless</string>\n</array>\n"), 0o644))
	require.NoError(t, os.MkdirAll(layout.stagingDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(layout.stagingDir, "env"), []byte("DD_API_KEY=k\nDD_GUI_APP_MENU_ENABLED=true\n"), 0o600))

	require.NoError(t, setUpDMGInstall(t.Context(), layout))
	startDMGGUI(t.Context(), layout, readInstallEnv(layout))

	assert.DirExists(t, filepath.Join(layout.applicationsDir, layout.appName()))
	assert.DirExists(t, filepath.Join(layout.runDir(), "ipc"))
	info, err := os.Stat(filepath.Join(layout.runDir(), "ipc"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSticky, "the ipc directory must be sticky")
	datadogYAML, err := os.ReadFile(filepath.Join(layout.confDir(), "datadog.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(datadogYAML), "api_key: k\n")
	gui, err := os.ReadFile(filepath.Join(layout.launchAgentsDir, guiLabel+".plist"))
	require.NoError(t, err)
	assert.NotContains(t, string(gui), "--headless")
	assert.NoDirExists(t, layout.stagingDir, "the staging directory must be removed")
}
