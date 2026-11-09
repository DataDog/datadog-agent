// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
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
		"DD_AI_USAGE_CHROME_EXTENSION_ID": strings.Repeat("abcdefghijklmnop", 2),
	}))

	content, err := os.ReadFile(filepath.Join(conf, "datadog.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "api_key: 0123\nsite: datad0g.com\n\ninfrastructure_mode: basic\n", string(content))
	nativeHost, err := os.ReadFile(filepath.Join(conf, "ai_usage_native_host.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "chrome_extension_id: "+strings.Repeat("abcdefghijklmnop", 2)+"\n", string(nativeHost))
}

// TestApplyInstallEnvIgnoresAnInvalidChromeExtensionID pins that an install-time value that is not a
// Chrome extension ID never reaches the native host configuration, which keeps the ID it had.
func TestApplyInstallEnvIgnoresAnInvalidChromeExtensionID(t *testing.T) {
	for _, id := range []string{"ext", strings.Repeat("A", 32), "x\nchrome_extension_id: " + strings.Repeat("p", 32), strings.Repeat("a", 33)} {
		conf := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(conf, "datadog.yaml"), []byte("api_key: APIKEY\n"), 0o600))
		original := "# chrome_extension_id: abc\n"
		require.NoError(t, os.WriteFile(filepath.Join(conf, "ai_usage_native_host.yaml"), []byte(original), 0o600))

		require.NoError(t, applyInstallEnv(conf, installEnv{"DD_AI_USAGE_CHROME_EXTENSION_ID": id}))

		nativeHost, err := os.ReadFile(filepath.Join(conf, "ai_usage_native_host.yaml"))
		require.NoError(t, err)
		assert.Equal(t, original, string(nativeHost), "%q must not be written", id)
	}
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
// daemon's live state in the Agent's run directory is kept.
func TestRemoveOldInstallFiles(t *testing.T) {
	layout := stubDmgLayout(t)
	stale := []string{
		filepath.Join(layout.installDir, "agent", "f"),
		filepath.Join(layout.packagesDir, "logs", "updater.log"),
		filepath.Join(layout.packagesRunDir, "installer.sock"),
		filepath.Join(layout.packagesRunDir, "installer_tasks.db"),
		filepath.Join(layout.packagesRunDir, "remote-config-installer.db"),
	}
	kept := []string{
		filepath.Join(layout.runDir(), "remote-config-installer.db"),
		filepath.Join(layout.runDir(), "installer.sock"),
		filepath.Join(layout.runDir(), "installer_tasks.db"),
		filepath.Join(layout.runDir(), "installer.pid"),
	}
	for _, path := range append(stale, kept...) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, nil, 0o644))
	}

	removeOldInstallFiles(layout)

	for _, path := range stale {
		assert.NoFileExists(t, path)
	}
	assert.NoDirExists(t, filepath.Join(layout.packagesDir, "logs"))
	for _, path := range kept {
		assert.FileExists(t, path)
	}
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

// TestStartDMGGUILoadsForTheConsoleSession pins how the GUI session is detected: the LaunchAgents are
// loaded into the console user's gui domain when launchd has one, and left for the next login when
// it doesn't. The installer runs outside the user's Aqua context, so the check asks launchd for the
// domain rather than for the user's manager name.
func TestStartDMGGUILoadsForTheConsoleSession(t *testing.T) {
	current, err := user.Current()
	require.NoError(t, err)
	for _, hasSession := range []bool{true, false} {
		layout := stubDmgLayout(t)
		require.NoError(t, os.WriteFile(filepath.Join(layout.confDir(), guiLabel+".plist.example"), []byte("<plist/>\n"), 0o644))
		var commands [][]string
		hostCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
			commands = append(commands, append([]string{name}, args...))
			if name == "launchctl" && len(args) == 2 && args[0] == "print" && !hasSession {
				return nil, errors.New("Could not find domain")
			}
			return nil, nil
		}

		startDMGGUI(t.Context(), layout, installEnv{})

		assert.Contains(t, commands, []string{"launchctl", "print", "gui/" + current.Uid})
		bootstrap := []string{"launchctl", "bootstrap", "gui/" + current.Uid, filepath.Join(layout.launchAgentsDir, guiLabel+".plist")}
		if hasSession {
			assert.Contains(t, commands, bootstrap, "the GUI must be loaded into the console session")
		} else {
			assert.NotContains(t, commands, bootstrap, "without a session the GUI waits for the next login")
		}
		for _, c := range commands {
			assert.NotContains(t, c, "managername")
		}
		assert.FileExists(t, filepath.Join(layout.launchAgentsDir, guiLabel+".plist"))
	}
}

// stubLookupUserIDs makes the given names resolve to the current user, and every other name fail
// as a home without an account would.
func stubLookupUserIDs(t *testing.T, names ...string) {
	t.Helper()
	original := lookupUserIDs
	lookupUserIDs = func(name string) (int, int, error) {
		for _, n := range names {
			if n == name {
				return os.Getuid(), os.Getgid(), nil
			}
		}
		return 0, 0, errors.New("unknown user " + name)
	}
	t.Cleanup(func() { lookupUserIDs = original })
}

// readNativeHostManifest returns the manifest written under home, checking it is a regular file
// owned by the current user and readable by everyone.
func readNativeHostManifest(t *testing.T, home string) nativeHostManifest {
	t.Helper()
	path := filepath.Join(home, nativeHostManifestDir, nativeHost+".json")
	info, err := os.Lstat(path)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular(), "the manifest must be a regular file")
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	assert.Equal(t, uint32(os.Getuid()), info.Sys().(*syscall.Stat_t).Uid)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	var manifest nativeHostManifest
	require.NoError(t, json.Unmarshal(content, &manifest))
	return manifest
}

// TestInstallNativeMessagingHosts pins which homes get the Chrome manifest and what it says: every
// home with an account gets one, whether or not Chrome has created its directories yet, the
// manifest names the launcher and the configured extension, and the old host's manifest goes.
func TestInstallNativeMessagingHosts(t *testing.T) {
	layout := stubDmgLayout(t)
	launcher := filepath.Join(layout.installDir, "embedded", "bin", "run_ai_usage_native_host.sh")
	require.NoError(t, os.MkdirAll(filepath.Dir(launcher), 0o755))
	require.NoError(t, os.WriteFile(launcher, []byte("#!/bin/sh\n"), 0o755))
	for _, name := range []string{"fresh", "existing", "Shared", "noaccount"} {
		require.NoError(t, os.MkdirAll(filepath.Join(layout.usersDir, name), 0o755))
	}
	existingDir := filepath.Join(layout.usersDir, "existing", nativeHostManifestDir)
	require.NoError(t, os.MkdirAll(existingDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(existingDir, oldNativeHost+".json"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(existingDir, nativeHost+".json"), []byte("stale"), 0o600))
	stubLookupUserIDs(t, "fresh", "existing", "Shared")
	extensionID := strings.Repeat("abcdefghijklmnop", 2)

	installNativeMessagingHosts(layout, installEnv{"DD_AI_USAGE_CHROME_EXTENSION_ID": extensionID + "\r"})

	for _, name := range []string{"fresh", "existing"} {
		home := filepath.Join(layout.usersDir, name)
		assert.Equal(t, nativeHostManifest{
			Name:           nativeHost,
			Description:    "Datadog AI usage native messaging host",
			Path:           launcher,
			Type:           "stdio",
			AllowedOrigins: []string{"chrome-extension://" + extensionID + "/"},
		}, readNativeHostManifest(t, home), name)
		entries, err := os.ReadDir(filepath.Join(home, nativeHostManifestDir))
		require.NoError(t, err)
		require.Len(t, entries, 1, "only the manifest must be left in %s", name)
	}
	assert.NoFileExists(t, filepath.Join(existingDir, oldNativeHost+".json"))
	for _, name := range []string{"Shared", "noaccount"} {
		assert.NoDirExists(t, filepath.Join(layout.usersDir, name, "Library"), name)
	}
}

// TestInstallNativeMessagingHostsExtensionID pins where the extension ID comes from: the install
// environment, then the native host configuration, then the default, and that anything which is
// not an extension ID falls back to the default rather than ending up in the manifest.
func TestInstallNativeMessagingHostsExtensionID(t *testing.T) {
	configured := strings.Repeat("p", 32)
	for _, tc := range []struct {
		name, env, config, want string
	}{
		{name: "from the environment", env: strings.Repeat("a", 32), config: configured, want: strings.Repeat("a", 32)},
		{name: "from the configuration", config: configured, want: configured},
		{name: "default", want: defaultChromeExtensionID},
		{name: "not an ID", env: `x/", "chrome-extension://evil`, want: defaultChromeExtensionID},
		{name: "not an ID, configured", env: "NOT-AN-ID", config: configured, want: configured},
		{name: "configured, not an ID", config: "NOT-AN-ID", want: defaultChromeExtensionID},
		{name: "uppercase", env: strings.Repeat("A", 32), want: defaultChromeExtensionID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layout := stubDmgLayout(t)
			launcher := filepath.Join(layout.installDir, "embedded", "bin", "run_ai_usage_native_host.sh")
			require.NoError(t, os.MkdirAll(filepath.Dir(launcher), 0o755))
			require.NoError(t, os.WriteFile(launcher, nil, 0o755))
			if tc.config != "" {
				require.NoError(t, os.WriteFile(filepath.Join(layout.confDir(), "ai_usage_native_host.yaml"),
					[]byte("chrome_extension_id: "+tc.config+"\n"), 0o644))
			}
			home := filepath.Join(layout.usersDir, "someone")
			require.NoError(t, os.MkdirAll(home, 0o755))
			stubLookupUserIDs(t, "someone")

			installNativeMessagingHosts(layout, installEnv{"DD_AI_USAGE_CHROME_EXTENSION_ID": tc.env})

			assert.Equal(t, []string{"chrome-extension://" + tc.want + "/"}, readNativeHostManifest(t, home).AllowedOrigins)
		})
	}
}

// TestInstallNativeMessagingHostsWithoutLauncher pins that no manifest is written when the
// launcher it would point Chrome at is missing.
func TestInstallNativeMessagingHostsWithoutLauncher(t *testing.T) {
	layout := stubDmgLayout(t)
	require.NoError(t, os.MkdirAll(filepath.Join(layout.usersDir, "someone"), 0o755))
	stubLookupUserIDs(t, "someone")

	installNativeMessagingHosts(layout, installEnv{})

	assert.NoDirExists(t, filepath.Join(layout.usersDir, "someone", "Library"))
}

// TestWriteNativeHostManifestDoesNotFollowLinks pins that the install, which runs as root, never
// writes through a link the user controls: a symlink or hard link left at the manifest path is
// replaced and its target left alone, a linked directory leading out of the home is refused, and
// so is a home that is itself a link.
func TestWriteNativeHostManifestDoesNotFollowLinks(t *testing.T) {
	manifest := []byte(`{"name":"test"}` + "\n")
	uid, gid := os.Getuid(), os.Getgid()

	plant := func(t *testing.T, link func(target, path string) error) {
		home, outside := t.TempDir(), t.TempDir()
		target := filepath.Join(outside, "sudoers")
		require.NoError(t, os.WriteFile(target, []byte("protected"), 0o644))
		dir := filepath.Join(home, nativeHostManifestDir)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, link(target, filepath.Join(dir, nativeHost+".json")))

		require.NoError(t, writeNativeHostManifest(home, uid, gid, manifest))

		content, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Equal(t, "protected", string(content), "the link target must be left alone")
		readNativeHostManifest(t, home)
	}
	t.Run("symlink", func(t *testing.T) { plant(t, os.Symlink) })
	t.Run("hard link", func(t *testing.T) { plant(t, os.Link) })

	t.Run("linked directory", func(t *testing.T) {
		home, outside := t.TempDir(), t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(home, "Library", "Application Support"), 0o755))
		require.NoError(t, os.Symlink(outside, filepath.Join(home, "Library", "Application Support", "Google")))

		assert.Error(t, writeNativeHostManifest(home, uid, gid, manifest))

		entries, err := os.ReadDir(outside)
		require.NoError(t, err)
		assert.Empty(t, entries, "nothing must be created outside the home")
	})

	t.Run("linked home", func(t *testing.T) {
		users, outside := t.TempDir(), t.TempDir()
		home := filepath.Join(users, "someone")
		require.NoError(t, os.Symlink(outside, home))

		assert.Error(t, writeNativeHostManifest(home, uid, gid, manifest))

		entries, err := os.ReadDir(outside)
		require.NoError(t, err)
		assert.Empty(t, entries, "nothing must be created through a linked home")
	})
}
