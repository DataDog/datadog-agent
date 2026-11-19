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
	"net"
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

			require.NoError(t, applyInstallEnv(openConfRoot(t, conf), installEnv{"DD_LOGS_ENABLED": value}))

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

	require.NoError(t, applyInstallEnv(openConfRoot(t, conf), installEnv{
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

		require.NoError(t, applyInstallEnv(openConfRoot(t, conf), installEnv{"DD_AI_USAGE_CHROME_EXTENSION_ID": id}))

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
		filepath.Join(layout.runDir(), "installer.pid"),
	}
	kept := []string{
		filepath.Join(layout.runDir(), "remote-config-installer.db"),
		filepath.Join(layout.runDir(), "installer.sock"),
		filepath.Join(layout.runDir(), "installer_tasks.db"),
		filepath.Join(layout.packagesRunDir, "installer.pid"),
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

// TestRestExperimentConfig pins that the install leaves etc-exp resting on etc: as a link on a
// fresh install, and as a link again over a host where a configuration experiment had deployed a
// real directory, so the installer daemon never reports a discarded experiment as running.
func TestRestExperimentConfig(t *testing.T) {
	for name, setUp := range map[string]func(t *testing.T, layout dmgLayout){
		"fresh install": func(*testing.T, dmgLayout) {},
		"deployed experiment": func(t *testing.T, layout dmgLayout) {
			require.NoError(t, os.MkdirAll(layout.expConfDir(), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(layout.expConfDir(), ".deployment-id"), []byte("experiment-1"), 0o640))
		},
		"resting link": func(t *testing.T, layout dmgLayout) {
			require.NoError(t, os.Symlink(layout.confDir(), layout.expConfDir()))
		},
	} {
		t.Run(name, func(t *testing.T) {
			layout := stubDmgLayout(t)
			setUp(t, layout)

			restExperimentConfig(context.Background(), layout)

			target, err := os.Readlink(layout.expConfDir())
			require.NoError(t, err, "etc-exp is not a link")
			assert.Equal(t, layout.confDir(), target)
			assert.FileExists(t, filepath.Join(layout.confDir(), "datadog.yaml.example"), "the stable configuration was touched")
		})
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

// agentCommandCall is one command secureConfig ran as an account.
type agentCommandCall struct {
	uid, gid int
	args     []string
}

// recordAgentCommands replaces agentCommand with one that records each call and runs nothing.
func recordAgentCommands(t *testing.T) *[]agentCommandCall {
	t.Helper()

	var calls []agentCommandCall
	original := agentCommand
	agentCommand = func(_ context.Context, uid, gid int, name string, args ...string) ([]byte, error) {
		calls = append(calls, agentCommandCall{uid: uid, gid: gid, args: append([]string{name}, args...)})
		return nil, nil
	}
	t.Cleanup(func() { agentCommand = original })
	return &calls
}

// TestSecureConfigSetsTheAccessControlListsAsTheAgent pins that the access control lists, which
// are applied by path, are applied by the Agent's account rather than by root.
func TestSecureConfigSetsTheAccessControlListsAsTheAgent(t *testing.T) {
	layout := stubDmgLayout(t)
	commands := stubUninstall(t)
	calls := recordAgentCommands(t)
	datadogYAML := filepath.Join(layout.confDir(), "datadog.yaml")
	require.NoError(t, os.WriteFile(datadogYAML, nil, 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(layout.confDir(), "conf.d"), 0o755))

	require.NoError(t, secureConfig(t.Context(), layout.confDir()))

	require.Len(t, *calls, 2)
	for _, call := range *calls {
		assert.Equal(t, os.Getuid(), call.uid)
		assert.Equal(t, os.Getgid(), call.gid)
		assert.Equal(t, []string{"/bin/chmod", "+a"}, call.args[:2])
	}
	assert.Equal(t, []string{"/bin/chmod", "+a", confDirACL, layout.confDir(), filepath.Join(layout.confDir(), "conf.d")}, (*calls)[0].args)
	assert.Contains(t, (*calls)[1].args, datadogYAML)
	for _, command := range *commands {
		assert.NotEqual(t, "chmod", command[0], "no access control list may be applied as root")
	}
	info, err := os.Stat(datadogYAML)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o660), info.Mode().Perm())
	info, err = os.Stat(layout.confDir())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o770), info.Mode().Perm())
}

// TestSecureConfigReachesNothingOutsideTheTree pins that the ownership and mode pass changes only
// the configuration's own entries: a file linked in from elsewhere, a link to an outside file and
// a directory replaced by a link are all left as they are, along with what they point to.
func TestSecureConfigReachesNothingOutsideTheTree(t *testing.T) {
	layout := stubDmgLayout(t)
	calls := recordAgentCommands(t)
	outside := t.TempDir()
	hardLinked := filepath.Join(outside, "hard.yaml")
	linkedTo := filepath.Join(outside, "linked.yaml")
	outsideDir := filepath.Join(outside, "dir")
	inDir := filepath.Join(outsideDir, "inside.yaml")
	require.NoError(t, os.Mkdir(outsideDir, 0o700))
	for _, path := range []string{hardLinked, linkedTo, inDir} {
		require.NoError(t, os.WriteFile(path, nil, 0o600))
	}
	require.NoError(t, os.Link(hardLinked, filepath.Join(layout.confDir(), "hard.yaml")))
	require.NoError(t, os.Symlink(linkedTo, filepath.Join(layout.confDir(), "linked.yaml")))
	require.NoError(t, os.Symlink(outsideDir, filepath.Join(layout.confDir(), "conf.d")))

	outsidePaths := []string{hardLinked, linkedTo, inDir, outsideDir}
	before := map[string]os.FileMode{}
	for _, path := range outsidePaths {
		info, err := os.Stat(path)
		require.NoError(t, err)
		before[path] = info.Mode()
	}

	require.NoError(t, secureConfig(t.Context(), layout.confDir()))

	for _, path := range outsidePaths {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, before[path], info.Mode(), "%s was changed", path)
	}
	for _, call := range *calls {
		for _, arg := range call.args {
			assert.NotContains(t, []string{filepath.Join(layout.confDir(), "hard.yaml"), filepath.Join(layout.confDir(), "linked.yaml"), filepath.Join(layout.confDir(), "conf.d")}, arg,
				"no access control list may be applied to an entry that was not secured")
		}
	}
}

// TestSecureTreeSkipsSocketsAndPipes covers the run directory on a host with a logged-in user: the
// Agent's GUI keeps a socket there (run/ipc/gui-<uid>.sock), which cannot be opened, and opening it
// used to fail the whole .dmg set-up. Sockets and pipes are skipped; the rest is still secured.
func TestSecureTreeSkipsSocketsAndPipes(t *testing.T) {
	// A short path: a unix socket's path is limited to about a hundred bytes.
	root, err := os.MkdirTemp("/tmp", "securetree")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(root) })
	ipc := filepath.Join(root, "ipc")
	require.NoError(t, os.Mkdir(ipc, 0o755))
	listener, err := net.Listen("unix", filepath.Join(ipc, "gui-501.sock"))
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })
	require.NoError(t, syscall.Mkfifo(filepath.Join(ipc, "pipe"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agent.pid"), nil, 0o600))

	dirs, files, err := secureTree(root, os.Getuid(), os.Getgid(), nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{root, ipc}, dirs)
	assert.ElementsMatch(t, []string{filepath.Join(root, "agent.pid")}, files)
}

// openConfRoot opens dir as the os.Root the configuration steps work through, for the rest of the
// test.
func openConfRoot(t *testing.T, dir string) *os.Root {
	t.Helper()

	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	t.Cleanup(func() { root.Close() })
	return root
}

// TestSetUpConfigDoesNotWriteThroughLinks pins that the configuration set-up never writes through
// an entry of the configuration: a link and a file linked from elsewhere are replaced by files of
// their own, and what they pointed to is left as it was.
func TestSetUpConfigDoesNotWriteThroughLinks(t *testing.T) {
	layout := stubDmgLayout(t)
	conf := layout.confDir()
	outside := t.TempDir()
	linkedTo := filepath.Join(outside, "native-host.yaml")
	hardLinked := filepath.Join(outside, "install_info")
	for _, path := range []string{linkedTo, hardLinked} {
		require.NoError(t, os.WriteFile(path, []byte("untouched\n"), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(conf, "ai_usage_native_host.yaml.example"), []byte("# trace_agent_url: x\n"), 0o644))
	require.NoError(t, os.Symlink(linkedTo, filepath.Join(conf, "ai_usage_native_host.yaml")))
	require.NoError(t, os.Link(hardLinked, filepath.Join(conf, "install_info")))

	require.NoError(t, setUpConfig(t.Context(), layout, installEnv{}))

	for _, path := range []string{linkedTo, hardLinked} {
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "untouched\n", string(content), "%s was written through", path)
	}
	for _, name := range []string{"ai_usage_native_host.yaml", "install_info"} {
		info, err := os.Lstat(filepath.Join(conf, name))
		require.NoError(t, err)
		assert.True(t, info.Mode().IsRegular(), "%s is not a file of its own", name)
	}
}

// TestSetUpConfigLeavesALinkedDatadogYAMLAlone pins that a datadog.yaml that is a link is neither
// read nor written through: the install settings are not applied to it.
func TestSetUpConfigLeavesALinkedDatadogYAMLAlone(t *testing.T) {
	layout := stubDmgLayout(t)
	conf := layout.confDir()
	target := filepath.Join(t.TempDir(), "datadog.yaml")
	require.NoError(t, os.WriteFile(target, []byte("api_key: kept\n"), 0o600))
	require.NoError(t, os.Symlink(target, filepath.Join(conf, "datadog.yaml")))

	require.NoError(t, setUpConfig(t.Context(), layout, installEnv{"DD_API_KEY": "replaced"}))

	content, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "api_key: kept\n", string(content))
}

// TestSetUpConfigRestoresTheSavedConfigurationInsideTheTree pins that restoring the saved
// configuration writes nothing outside it, even where a directory of the configuration has been
// replaced by a link, and that saved links are restored as links rather than followed.
func TestSetUpConfigRestoresTheSavedConfigurationInsideTheTree(t *testing.T) {
	layout := stubDmgLayout(t)
	conf := layout.confDir()
	saved := layout.savedConfDir()
	outside := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(secret, []byte("secret\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(saved, "conf.d", "check.d"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(saved, "datadog.yaml"), []byte("api_key: saved\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(saved, "conf.d", "check.d", "conf.yaml"), []byte("instances: []\n"), 0o600))
	require.NoError(t, os.Symlink(secret, filepath.Join(saved, "system-probe.yaml")))
	require.NoError(t, os.Symlink(outside, filepath.Join(conf, "conf.d")))

	require.NoError(t, setUpConfig(t.Context(), layout, installEnv{}))

	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	assert.Empty(t, entries, "the restore wrote through the conf.d link")
	destination, err := os.Readlink(filepath.Join(conf, "system-probe.yaml"))
	require.NoError(t, err, "a saved link must be restored as a link")
	assert.Equal(t, secret, destination)
	content, err := os.ReadFile(filepath.Join(conf, "datadog.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "api_key: saved")
}
