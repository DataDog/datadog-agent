// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// The .dmg's postinstall script hands the install to `installer postinst datadog-agent dmg`, the
// way deb and rpm do. Everything below runs from that hook, after the payload is in place. Only what
// must happen before the payload replaces the old version -- stopping it and saving its
// configuration -- stays in the preinstall script.

const (
	agentUser     = "_dd-agent"
	agentGroup    = "admin"
	guiLabel      = "com.datadoghq.gui"
	monitorLabel  = "com.datadoghq.ai-usage-agent.desktop-monitor"
	oldMonitor    = "com.datadoghq.ai-prompt-logger.desktop-monitor"
	nativeHost    = "com.datadoghq.ai_usage_agent.native_host"
	oldNativeHost = "com.datadoghq.ai_prompt_logger.native_host"
	// defaultChromeExtensionID is the AI usage Chrome extension the native host talks to unless the
	// install says otherwise.
	defaultChromeExtensionID = "gkmbhgbippkmmmidcikijiblbagbjgjj"

	confDirACL  = "user:_dd-agent allow read,write,readattr,writeattr,readextattr,writeextattr,readsecurity,list,search,add_file,add_subdirectory,file_inherit,directory_inherit"
	confFileACL = "user:_dd-agent allow read,write,readattr,writeattr,readextattr,writeextattr,readsecurity"
	ipcDirACL   = "user:_dd-agent allow read,write,readattr,readextattr,readsecurity,list,search,file_inherit,directory_inherit"
)

// dmgLayout is where the .dmg install lives. It is parameterised on its roots so the tests can run
// against temporary directories. Production always uses defaultDmgLayout.
type dmgLayout struct {
	installDir      string
	applicationsDir string
	logLink         string
	stagingDir      string
	linkDir         string
	launchAgentsDir string
	usersDir        string
	consoleDevice   string
	packagesDir     string
	packagesRunDir  string
}

var defaultDmgLayout = dmgLayout{
	installDir:      filepath.Dir(paths.AgentConfigDir),
	applicationsDir: "/Applications",
	logLink:         "/var/log/datadog",
	stagingDir:      "/private/var/root/datadog-install",
	linkDir:         "/usr/local/bin",
	launchAgentsDir: "/Library/LaunchAgents",
	usersDir:        "/Users",
	consoleDevice:   "/dev/console",
	packagesDir:     paths.PackagesPath,
	packagesRunDir:  filepath.Join(paths.PackagesPath, "run"),
}

func (l dmgLayout) confDir() string      { return filepath.Join(l.installDir, "etc") }
func (l dmgLayout) runDir() string       { return filepath.Join(l.installDir, "run") }
func (l dmgLayout) logDir() string       { return filepath.Join(l.installDir, "logs") }
func (l dmgLayout) savedConfDir() string { return filepath.Join(l.stagingDir, "saved-conf") }
func (l dmgLayout) appName() string      { return "Datadog Agent.app" }

// sleep is indirected so the tests don't wait for the GUI to quit.
var sleep = time.Sleep

// setUpDMGInstall sets up everything around the binaries the payload laid down: the Agent's
// account, its directories and their permissions, the configuration, the application, the links
// and the Chrome native messaging hosts. The LaunchDaemons and the GUI LaunchAgents are loaded
// afterwards, by postInstallDMG and startDMGGUI.
//
// Steps the Agent cannot run without return an error; the rest only log.
func setUpDMGInstall(ctx context.Context, layout dmgLayout) error {
	if err := ensureAgentUser(ctx); err != nil {
		return err
	}
	ensureLogLink(layout)
	removeOldInstallFiles(layout)
	stopGUI(ctx, layout)
	if err := prepareLogDir(layout); err != nil {
		return err
	}
	installApp(layout)
	if err := prepareRunDir(ctx, layout); err != nil {
		return err
	}
	env := readInstallEnv(layout)
	if err := setUpConfig(ctx, layout, env); err != nil {
		return err
	}
	installNativeMessagingHosts(layout, env)
	installLinks(layout)
	return nil
}

// ensureAgentUser creates the Agent's account if it is missing, in the 450-499 range, and enforces
// its least-privilege properties on every install, whoever created it: no shell, no home, hidden,
// the daemon primary group, no password and no authentication authority, and no admin, wheel or
// staff membership.
func ensureAgentUser(ctx context.Context) error {
	if _, err := user.Lookup(agentUser); err != nil {
		uid, err := freeDaemonUID(ctx)
		if err != nil {
			return err
		}
		for _, args := range [][]string{
			{".", "-create", "/Users/" + agentUser},
			{".", "-create", "/Users/" + agentUser, "RealName", "Datadog Agent"},
			{".", "-create", "/Users/" + agentUser, "UniqueID", strconv.Itoa(uid)},
		} {
			if _, err := hostCommand(ctx, "dscl", args...); err != nil {
				return fmt.Errorf("could not create %s: %w", agentUser, err)
			}
		}
	}
	for _, args := range [][]string{
		{".", "-create", "/Users/" + agentUser, "UserShell", "/usr/bin/false"},
		{".", "-create", "/Users/" + agentUser, "NFSHomeDirectory", "/var/empty"},
		{".", "-create", "/Users/" + agentUser, "IsHidden", "1"},
		{".", "-create", "/Users/" + agentUser, "PrimaryGroupID", "1"},
		{".", "-create", "/Users/" + agentUser, "Password", "*"},
	} {
		if _, err := hostCommand(ctx, "dscl", args...); err != nil {
			return fmt.Errorf("could not set up %s: %w", agentUser, err)
		}
	}
	_, _ = hostCommand(ctx, "dscl", ".", "-delete", "/Users/"+agentUser, "AuthenticationAuthority")
	for _, group := range []string{"admin", "wheel", "staff"} {
		if _, err := hostCommand(ctx, "dseditgroup", "-o", "checkmember", "-m", agentUser, group); err == nil {
			log.Infof("removing %s from group %s", agentUser, group)
			if _, err := hostCommand(ctx, "dseditgroup", "-o", "edit", "-d", agentUser, group); err != nil {
				return fmt.Errorf("could not remove %s from %s: %w", agentUser, group, err)
			}
		}
	}
	if u, err := user.Lookup(agentUser); err == nil {
		if uid, _ := strconv.Atoi(u.Uid); uid >= 500 {
			log.Warnf("%s has UID %d, in the interactive range; it is kept so file ownership stays valid", agentUser, uid)
		}
	}
	if out, _ := hostCommand(ctx, "sysadminctl", "-secureTokenStatus", agentUser); strings.Contains(string(out), "ENABLED") {
		log.Warnf("%s has a Secure Token; daemon accounts should not hold one", agentUser)
	}
	return nil
}

// freeDaemonUID returns the first UID from 450 that no account uses, below the interactive range.
func freeDaemonUID(ctx context.Context) (int, error) {
	out, err := hostCommand(ctx, "dscl", ".", "-list", "/Users", "UniqueID")
	if err != nil {
		return 0, fmt.Errorf("could not list user IDs: %w", err)
	}
	used := map[int]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			if uid, err := strconv.Atoi(fields[1]); err == nil {
				used[uid] = true
			}
		}
	}
	for uid := 450; uid < 500; uid++ {
		if !used[uid] {
			return uid, nil
		}
	}
	return 0, fmt.Errorf("no free UID in 450-499 for %s", agentUser)
}

// ensureLogLink points /var/log/datadog at the install root's log directory. macOS removes /var/log
// on OS upgrades, so the logs live in the install root. A real directory left by an old version is
// migrated.
func ensureLogLink(layout dmgLayout) {
	info, err := os.Lstat(layout.logLink)
	switch {
	case err == nil && info.Mode()&fs.ModeSymlink != 0:
		return
	case err == nil && info.IsDir():
		log.Infof("migrating %s to %s", layout.logLink, layout.logDir())
		if err := copyTree(layout.logLink, layout.logDir(), true); err != nil {
			log.Warnf("could not migrate %s: %v", layout.logLink, err)
		}
		if err := os.RemoveAll(layout.logLink); err != nil {
			log.Warnf("could not remove %s: %v", layout.logLink, err)
			return
		}
	default:
		if err := os.MkdirAll(filepath.Dir(layout.logLink), 0o755); err != nil {
			log.Warnf("could not create %s: %v", filepath.Dir(layout.logLink), err)
			return
		}
	}
	if err := os.Symlink(layout.logDir(), layout.logLink); err != nil {
		log.Warnf("could not link %s: %v", layout.logLink, err)
	}
}

// removeOldInstallFiles removes what earlier versions left that nothing uses any longer.
func removeOldInstallFiles(layout dmgLayout) {
	stale := []string{
		filepath.Join(layout.installDir, "agent"),
		filepath.Join(layout.installDir, "checks.d"),
		filepath.Join(layout.installDir, "embedded", "bin", "ai-prompt-logger-native-host"),
		// Builds that ran a configuration experiment watcher from the Agent's run directory.
		filepath.Join(layout.runDir(), "experiment-deadline"),
		// Builds that kept the installer daemon's state and logs under the packages root. The
		// daemon now keeps them in the Agent's run and log directories, and anything left under the
		// packages root would be read as a package.
		filepath.Join(layout.packagesDir, "logs"),
		filepath.Join(layout.packagesRunDir, "installer.sock"),
		filepath.Join(layout.packagesRunDir, "installer_tasks.db"),
		filepath.Join(layout.packagesRunDir, "remote-config-installer.db"),
	}
	for _, path := range stale {
		if err := os.RemoveAll(path); err != nil {
			log.Warnf("could not remove %s: %v", path, err)
		}
	}
}

// stopGUI stops the GUI and the desktop monitor for every logged-in user, so the application can
// be replaced, and waits up to 10 seconds for the GUI to quit.
func stopGUI(ctx context.Context, layout dmgLayout) {
	for _, uid := range loggedInUIDs(ctx) {
		for _, label := range []string{guiLabel, monitorLabel, oldMonitor} {
			_, _ = hostCommand(ctx, "launchctl", "bootout", "gui/"+uid+"/"+label)
		}
	}
	guiProcess := layout.appName() + "/Contents/MacOS/gui"
	for range 10 {
		if _, err := hostCommand(ctx, "pgrep", "-f", guiProcess); err != nil {
			return
		}
		sleep(time.Second)
	}
	log.Warnf("the GUI is still running after 10s; continuing")
}

// prepareLogDir gives the log directory to the Agent's account; the admin group can read and write
// the logs.
func prepareLogDir(layout dmgLayout) error {
	if err := os.MkdirAll(layout.logDir(), 0o750); err != nil {
		return fmt.Errorf("could not create %s: %w", layout.logDir(), err)
	}
	if err := chownTree(layout.logDir(), agentUser, agentGroup); err != nil {
		return err
	}
	return os.Chmod(layout.logDir(), 0o770)
}

// installApp moves the application the payload laid down in the install root to /Applications.
func installApp(layout dmgLayout) {
	src := filepath.Join(layout.installDir, layout.appName())
	if err := os.Rename(src, filepath.Join(layout.applicationsDir, layout.appName())); err != nil {
		log.Infof("application already installed: %v", err)
	}
}

// prepareRunDir gives the run directory to the Agent's account and sets up its ipc directory, where
// the per-user GUI processes create their sockets: anyone can create one but not remove another's,
// and the sockets inherit an ACL that lets the Agent connect to them.
func prepareRunDir(ctx context.Context, layout dmgLayout) error {
	ipc := filepath.Join(layout.runDir(), "ipc")
	if err := os.MkdirAll(ipc, 0o755); err != nil {
		return fmt.Errorf("could not create %s: %w", ipc, err)
	}
	if err := chownTree(layout.runDir(), agentUser, agentGroup); err != nil {
		return err
	}
	if err := os.Chmod(layout.runDir(), 0o775); err != nil {
		return err
	}
	stale, _ := filepath.Glob(filepath.Join(ipc, "gui-*.sock"))
	for _, sock := range stale {
		_ = os.Remove(sock)
	}
	if err := chownTree(ipc, "root", "wheel"); err != nil {
		return err
	}
	if err := os.Chmod(ipc, 0o777|fs.ModeSticky); err != nil {
		return err
	}
	if _, err := hostCommand(ctx, "chmod", "+a", ipcDirACL, ipc); err != nil {
		log.Warnf("could not set the ACL on %s: %v", ipc, err)
	}
	return nil
}

// installEnv holds what install_mac_os.sh passed to the install, from its staging env file.
type installEnv map[string]string

// readInstallEnv reads the env file install_mac_os.sh writes in the staging directory. The first
// line for a key wins, as with grep | cut.
func readInstallEnv(layout dmgLayout) installEnv {
	env := installEnv{}
	f, err := os.Open(filepath.Join(layout.stagingDir, "env"))
	if err != nil {
		return env
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		if _, seen := env[key]; !seen {
			env[key] = value
		}
	}
	return env
}

// setUpConfig restores the configuration saved by the preinstall script, or creates the default one
// on a first install, applies what the install script was given, writes the install info, and gives
// the configuration to the Agent's account.
func setUpConfig(ctx context.Context, layout dmgLayout, env installEnv) error {
	conf := layout.confDir()
	if err := os.MkdirAll(filepath.Join(conf, "checks.d"), 0o755); err != nil {
		return err
	}
	saved := layout.savedConfDir()
	if info, err := os.Stat(saved); err == nil && info.IsDir() {
		for _, name := range []string{"datadog.yaml", "system-probe.yaml"} {
			_ = copyFile(filepath.Join(saved, name), filepath.Join(conf, name))
		}
		_ = copyTree(filepath.Join(saved, "conf.d"), filepath.Join(conf, "conf.d"), true)
		_ = copyTree(filepath.Join(saved, "checks.d"), filepath.Join(conf, "checks.d"), false)
	}
	datadogYAML := filepath.Join(conf, "datadog.yaml")
	if _, err := os.Stat(datadogYAML); errors.Is(err, fs.ErrNotExist) {
		example, err := os.ReadFile(filepath.Join(conf, "datadog.yaml.example"))
		if err != nil {
			return fmt.Errorf("could not read the example configuration: %w", err)
		}
		if err := os.WriteFile(datadogYAML, replaceLines(example, `^api_key:$`, "api_key: APIKEY"), 0o660); err != nil {
			return err
		}
	}
	nativeHostYAML := filepath.Join(conf, "ai_usage_native_host.yaml")
	if example, err := os.ReadFile(nativeHostYAML + ".example"); err == nil {
		content := example
		if port := apmReceiverPort(datadogYAML); port != "" && port != "8126" {
			content = replaceLines(content, `^[ #]*trace_agent_url:.*`, `trace_agent_url: "http://127.0.0.1:`+port+`"`)
		}
		if err := os.WriteFile(nativeHostYAML, content, 0o660); err != nil {
			return err
		}
	}
	if err := applyInstallEnv(conf, env); err != nil {
		return err
	}
	if err := writeInstallInfo(conf, env); err != nil {
		return err
	}
	return secureConfig(ctx, conf)
}

// apmReceiverPort returns apm_config.receiver_port from datadog.yaml when it is a number.
func apmReceiverPort(datadogYAML string) string {
	content, err := os.ReadFile(datadogYAML)
	if err != nil {
		return ""
	}
	inAPM := false
	topLevel := regexp.MustCompile(`^[^\s][^:]*:`)
	apmSection := regexp.MustCompile(`^\s*apm_config:\s*($|#)`)
	receiverPort := regexp.MustCompile(`^\s*receiver_port:\s*(.*)$`)
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if topLevel.MatchString(line) {
			inAPM = false
		}
		if apmSection.MatchString(line) {
			inAPM = true
			continue
		}
		if m := receiverPort.FindStringSubmatch(line); inAPM && m != nil {
			value, _, _ := strings.Cut(m[1], "#")
			value = strings.Trim(strings.TrimSpace(value), `"'`)
			if _, err := strconv.Atoi(value); err == nil {
				return value
			}
		}
	}
	return ""
}

// applyInstallEnv writes what install_mac_os.sh was given into the configuration.
func applyInstallEnv(conf string, env installEnv) error {
	nativeHostYAML := filepath.Join(conf, "ai_usage_native_host.yaml")
	if id := installChromeExtensionID(env); id != "" {
		if content, err := os.ReadFile(nativeHostYAML); err == nil {
			if regexp.MustCompile(`(?m)^\s*#?\s*chrome_extension_id:`).Match(content) {
				content = replaceLines(content, `^# *chrome_extension_id:.*`, "chrome_extension_id: "+id)
				content = replaceLines(content, `^chrome_extension_id:.*`, "chrome_extension_id: "+id)
			} else {
				content = append(content, []byte("\nchrome_extension_id: "+id+"\n")...)
			}
			if err := os.WriteFile(nativeHostYAML, content, 0o660); err != nil {
				return err
			}
		}
	}
	datadogYAML := filepath.Join(conf, "datadog.yaml")
	content, err := os.ReadFile(datadogYAML)
	if err != nil {
		return err
	}
	if key := env["DD_API_KEY"]; key != "" {
		content = replaceLines(content, `^api_key:.*`, "api_key: "+key)
	}
	if site := env["DD_SITE"]; site != "" {
		content = replaceLines(content, `^#* *site:.*`, "site: "+site)
	}
	if mode := env["DD_INFRASTRUCTURE_MODE"]; mode != "" {
		content = setOrAppend(content, `^#* *infrastructure_mode:.*`, "infrastructure_mode: "+mode)
	}
	if logs := env["DD_LOGS_ENABLED"]; logs == "true" || logs == "false" {
		content = setOrAppend(content, `^(# *)?logs_enabled:.*`, "logs_enabled: "+logs)
	}
	return os.WriteFile(datadogYAML, content, 0o660)
}

// writeInstallInfo records how the Agent was installed.
func writeInstallInfo(conf string, env installEnv) error {
	method, version := "macos_dmg", "macos_dmg"
	if m := env["DD_INSTALL_METHOD"]; m != "" {
		method, version = m, m+"-"+env["DD_INSTALL_SCRIPT_VERSION"]
	}
	content := fmt.Sprintf("---\ninstall_method:\n  tool: %s\n  tool_version: %s\n  installer_version: %s\n\n", method, method, version)
	return os.WriteFile(filepath.Join(conf, "install_info"), []byte(content), 0o660)
}

// secureConfig gives the configuration to the Agent's account, the admin group able to edit it, and
// sets ACLs so the Agent keeps access to files an editor replaces: inheritable ones on directories,
// explicit ones on files, which macOS editors carry over on an atomic save. The AI usage native host
// runs as the logged-in user, so everyone may traverse the directory and read its one config file.
func secureConfig(ctx context.Context, conf string) error {
	if err := chownTree(conf, agentUser, agentGroup); err != nil {
		return err
	}
	if err := os.Chmod(conf, 0o770); err != nil {
		return err
	}
	var dirs, files []string
	err := filepath.WalkDir(conf, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
		case d.IsDir():
			dirs = append(dirs, path)
		default:
			files = append(files, path)
			return os.Chmod(path, 0o660)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("could not set the configuration's permissions: %w", err)
	}
	for _, batch := range [][]string{append([]string{"+a", confDirACL}, dirs...), append([]string{"+a", confFileACL}, files...)} {
		if len(batch) == 2 {
			continue
		}
		if _, err := hostCommand(ctx, "chmod", batch...); err != nil {
			log.Warnf("could not set the configuration's ACLs: %v", err)
		}
	}
	nativeHostYAML := filepath.Join(conf, "ai_usage_native_host.yaml")
	if _, err := os.Stat(nativeHostYAML); err == nil {
		_, _ = hostCommand(ctx, "chmod", "+a", "everyone allow search,readattr,readextattr,readsecurity", conf)
		_, _ = hostCommand(ctx, "chmod", "+a", "everyone allow read,readattr,readextattr,readsecurity", nativeHostYAML)
	}
	return nil
}

// chromeExtensionIDPattern is the shape of a Chrome extension ID: 32 letters from a to p.
var chromeExtensionIDPattern = regexp.MustCompile(`^[a-p]{32}$`)

// nativeHostManifestDir is where Chrome looks for a user's native messaging hosts, relative to
// the user's home.
var nativeHostManifestDir = filepath.Join("Library", "Application Support", "Google", "Chrome", "NativeMessagingHosts")

// nativeHostManifest is the Chrome native messaging host manifest.
type nativeHostManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

// installNativeMessagingHosts registers the AI usage native host with Chrome for every user, by
// writing the manifest into each user's home.
func installNativeMessagingHosts(layout dmgLayout, env installEnv) {
	extensionID := installChromeExtensionID(env)
	if extensionID == "" {
		extensionID = configuredChromeExtensionID(filepath.Join(layout.confDir(), "ai_usage_native_host.yaml"))
	}
	if extensionID == "" {
		extensionID = defaultChromeExtensionID
	}
	if !chromeExtensionIDPattern.MatchString(extensionID) {
		log.Warnf("%q is not a Chrome extension ID; using %s", extensionID, defaultChromeExtensionID)
		extensionID = defaultChromeExtensionID
	}
	launcher := filepath.Join(layout.installDir, "embedded", "bin", "run_ai_usage_native_host.sh")
	if _, err := os.Stat(launcher); err != nil {
		log.Warnf("AI usage native host launcher missing at %s; skipping the Chrome manifests", launcher)
		return
	}
	manifest, err := json.MarshalIndent(nativeHostManifest{
		Name:           nativeHost,
		Description:    "Datadog AI usage native messaging host",
		Path:           launcher,
		Type:           "stdio",
		AllowedOrigins: []string{"chrome-extension://" + extensionID + "/"},
	}, "", "  ")
	if err != nil {
		log.Warnf("could not build the Chrome manifest: %v", err)
		return
	}
	manifest = append(manifest, '\n')
	homes, _ := os.ReadDir(layout.usersDir)
	for _, home := range homes {
		name := home.Name()
		if !home.IsDir() || name == "Shared" {
			continue
		}
		uid, gid, err := lookupUserIDs(name)
		if err != nil {
			continue
		}
		if err := writeNativeHostManifest(filepath.Join(layout.usersDir, name), uid, gid, manifest); err != nil {
			log.Warnf("could not install the Chrome manifest for %s: %v", name, err)
		}
	}
}

// writeNativeHostManifest writes the manifest into the Chrome NativeMessagingHosts directory under
// home, owned by uid:gid. The home belongs to the user, so every operation goes through an os.Root
// that keeps it inside the home, and the manifest is written to a new file that is renamed into
// place: a symlink or hard link the user left at the destination is replaced, never written
// through. The directories it creates are handed to the user, so Chrome can still write to them.
func writeNativeHostManifest(home string, uid, gid int, manifest []byte) error {
	if info, err := os.Lstat(home); err != nil {
		return err
	} else if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", home)
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return err
	}
	defer root.Close()

	dir := ""
	for _, part := range strings.Split(nativeHostManifestDir, string(filepath.Separator)) {
		dir = filepath.Join(dir, part)
		if err := root.Mkdir(dir, 0o755); err == nil {
			if err := root.Lchown(dir, uid, gid); err != nil {
				return err
			}
		} else if !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	if err := root.Remove(filepath.Join(dir, oldNativeHost+".json")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Warnf("could not remove the old Chrome manifest in %s: %v", home, err)
	}

	tmp := filepath.Join(dir, "."+nativeHost+".json."+strconv.Itoa(os.Getpid()))
	_ = root.Remove(tmp)
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(manifest)
	err = errors.Join(err, f.Chown(uid, gid), f.Chmod(0o644), f.Close())
	if err == nil {
		err = root.Rename(tmp, filepath.Join(dir, nativeHost+".json"))
	}
	if err != nil {
		_ = root.Remove(tmp)
	}
	return err
}

// installChromeExtensionID returns the Chrome extension ID install_mac_os.sh was given, or "" when
// it was given none, or something that is not an extension ID, which is logged and ignored so the
// configuration keeps the ID it had.
func installChromeExtensionID(env installEnv) string {
	id := strings.TrimSpace(strings.ReplaceAll(env["DD_AI_USAGE_CHROME_EXTENSION_ID"], "\r", ""))
	if id != "" && !chromeExtensionIDPattern.MatchString(id) {
		log.Warnf("DD_AI_USAGE_CHROME_EXTENSION_ID %q is not a Chrome extension ID; ignoring it", id)
		return ""
	}
	return id
}

// configuredChromeExtensionID returns chrome_extension_id from the native host config.
func configuredChromeExtensionID(nativeHostYAML string) string {
	content, err := os.ReadFile(nativeHostYAML)
	if err != nil {
		return ""
	}
	m := regexp.MustCompile(`(?m)^chrome_extension_id:\s*(.*)$`).FindSubmatch(content)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(string(m[1])), `"'`))
}

// installLinks links the user-facing commands. They name the install root, which never moves.
func installLinks(layout dmgLayout) {
	if err := os.MkdirAll(layout.linkDir, 0o755); err != nil {
		log.Warnf("could not create %s: %v", layout.linkDir, err)
		return
	}
	for link, target := range map[string]string{
		"datadog-agent":     filepath.Join(layout.installDir, "bin", "agent", "agent"),
		"datadog-installer": filepath.Join(layout.installDir, "embedded", "bin", "installer"),
	} {
		path := filepath.Join(layout.linkDir, link)
		_ = os.Remove(path)
		if err := os.Symlink(target, path); err != nil {
			log.Warnf("could not link %s: %v", path, err)
		}
	}
}

// startDMGGUI installs the GUI and the AI usage desktop monitor as system-wide LaunchAgents, loads
// them for the user at the console if there is a GUI session, and removes the staging directory.
// It runs once the LaunchDaemons are loaded.
//
// The GUI session is detected by asking launchd for the console user's gui domain. Asking for the
// user's manager name instead answers Background when the installer runs outside the user's Aqua
// context, so the helpers would not be restarted after an upgrade.
func startDMGGUI(ctx context.Context, layout dmgLayout, env installEnv) {
	consoleUID := ""
	if info, err := os.Stat(layout.consoleDevice); err == nil {
		if owner := fileOwner(info); owner != "" {
			if u, err := user.Lookup(owner); err == nil {
				if _, err := hostCommand(ctx, "launchctl", "print", "gui/"+u.Uid); err == nil {
					consoleUID = u.Uid
				}
			}
		}
	}
	install := func(label string, example string, edit func([]byte) []byte) {
		content, err := os.ReadFile(example)
		if err != nil {
			log.Warnf("LaunchAgent template missing at %s", example)
			return
		}
		if edit != nil {
			content = edit(content)
		}
		plist := filepath.Join(layout.launchAgentsDir, label+".plist")
		if err := os.WriteFile(plist, content, 0o644); err != nil {
			log.Warnf("could not write %s: %v", plist, err)
			return
		}
		_ = os.Chmod(plist, 0o644)
		_ = os.Lchown(plist, 0, 0)
		if consoleUID == "" {
			log.Infof("no GUI session; %s will start at the next login", label)
			return
		}
		_, _ = hostCommand(ctx, "launchctl", "enable", "gui/"+consoleUID+"/"+label)
		if _, err := hostCommand(ctx, "launchctl", "bootstrap", "gui/"+consoleUID, plist); err != nil {
			log.Warnf("could not load %s: %v", label, err)
		}
	}
	install(guiLabel, filepath.Join(layout.confDir(), guiLabel+".plist.example"), func(content []byte) []byte {
		if env["DD_GUI_APP_MENU_ENABLED"] == "true" {
			return regexp.MustCompile(`(?m)^.*<string>--headless</string>.*\n`).ReplaceAll(content, nil)
		}
		return content
	})
	_ = os.Remove(filepath.Join(layout.launchAgentsDir, oldMonitor+".plist"))
	_ = os.Remove(filepath.Join(layout.confDir(), oldMonitor+".plist.example"))
	install(monitorLabel, filepath.Join(layout.confDir(), monitorLabel+".plist.example"), nil)
	if err := os.RemoveAll(layout.stagingDir); err != nil {
		log.Warnf("could not remove %s: %v", layout.stagingDir, err)
	}
}

// fileOwner returns the name of the user who owns a file, or "" if it can't be resolved.
func fileOwner(info fs.FileInfo) string {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	u, err := user.LookupId(strconv.FormatUint(uint64(st.Uid), 10))
	if err != nil {
		return ""
	}
	return u.Username
}

// replaceLines replaces every line matching pattern, as sed's s/pattern/repl/ does on each line.
func replaceLines(content []byte, pattern string, replacement string) []byte {
	return regexp.MustCompile(`(?m)`+pattern).ReplaceAllLiteral(content, []byte(replacement))
}

// setOrAppend replaces the lines matching pattern, or appends line when there is none.
func setOrAppend(content []byte, pattern string, line string) []byte {
	if regexp.MustCompile(`(?m)` + pattern).Match(content) {
		return replaceLines(content, pattern, line)
	}
	return append(content, []byte("\n"+line+"\n")...)
}

// chownTree gives path, and everything under it, to owner and group without following links.
func chownTree(root string, owner string, group string) error {
	uid, gid, err := lookupOwner(owner, group)
	if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(path, uid, gid)
	})
}

// lookupOwner is indirected so the tests can chown to the current user.
var lookupOwner = func(owner string, group string) (int, int, error) {
	u, err := user.Lookup(owner)
	if err != nil {
		return 0, 0, fmt.Errorf("could not look up %s: %w", owner, err)
	}
	g, err := user.LookupGroup(group)
	if err != nil {
		return 0, 0, fmt.Errorf("could not look up %s: %w", group, err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(g.Gid)
	return uid, gid, nil
}

// lookupUserIDs returns a user's uid and primary gid. It is indirected so the tests can pick which
// home directories belong to an account.
var lookupUserIDs = func(name string) (int, int, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, 0, err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, err
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}

// copyTree copies the contents of src into dst, as cp -R src/* dst would, keeping links as links.
// Existing files are replaced only when overwrite is set, as cp -n would skip them.
func copyTree(src string, dst string, overwrite bool) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case !overwrite:
			if _, err := os.Lstat(target); err == nil {
				return nil
			}
		}
		if d.Type()&fs.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_ = os.Remove(target)
			return os.Symlink(link, target)
		}
		return copyFile(path, target)
	})
}

// copyFile copies a regular file, keeping its mode.
func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	_ = os.Remove(dst)
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
