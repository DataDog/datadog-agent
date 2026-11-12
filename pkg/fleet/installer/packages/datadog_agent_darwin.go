// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/symlink"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var datadogAgentPackage = hooks{
	preRemove:   preRemoveDatadogAgent,
	postInstall: postInstallDatadogAgent,
}

// datadogAgentLaunchdJobs are the launchd jobs of the Agent package, in the shape of
// datadogAgentService on Linux.
type datadogAgentLaunchdJobs struct {
	// LaunchdMainJobStable is the core Agent's job.
	LaunchdMainJobStable string
	// LaunchdJobsStable are the jobs that run normally, in the order they are loaded.
	LaunchdJobsStable []string
}

// agentLaunchdJobs are the jobs the Agent package installs.
//
// The process manager is loaded first, so it is up before the processes it will supervise. It
// is a stable job only: unlike on Linux, it will get no experiment variant, since a configuration
// experiment swaps the Agent's own jobs, not the daemon that supervises them.
//
// The root installer daemon is loaded last, once the Agent it reports on is up.
var agentLaunchdJobs = datadogAgentLaunchdJobs{
	LaunchdMainJobStable: "com.datadoghq.agent",
	LaunchdJobsStable: []string{
		"com.datadoghq.procmgr",
		"com.datadoghq.agent",
		"com.datadoghq.sysprobe",
		"com.datadoghq.data-plane",
		"com.datadoghq.installer",
	},
}

// ensureInstallerDataDir creates the installer's root-owned state directories. Indirected so the
// hook tests can run unprivileged.
var ensureInstallerDataDir = paths.EnsureInstallerDataDir

// packagesRoot is the root of the package repositories the shared installer code keeps.
// Indirected so the hook tests can run against a temporary directory.
var packagesRoot = paths.PackagesPath

// agentUserExists reports whether the Agent's account exists. Indirected so the hook tests do not
// depend on the account of the machine they run on.
var agentUserExists = func() bool {
	_, err := user.Lookup(agentUser)
	return err == nil
}

// launchdClient and launchdJobDir are indirected so the hook tests can run without launchd.
var (
	launchdClient = func() *launchd.Client { return launchd.NewClient(launchd.System) }
	launchdJobDir = launchd.System.Dir()
)

// installStableJobs writes and loads the stable launchd job set from the embedded definitions.
//
// Every job is loaded even if an earlier one fails: the jobs do not depend on one another to load,
// and one that fails must not also leave the others stopped.
func installStableJobs(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("install_stable_jobs")
	defer func() {
		span.Finish(err)
	}()

	client := launchdClient()
	var errs []error
	for _, label := range agentLaunchdJobs.LaunchdJobsStable {
		if err := loadStableJob(ctx, client, label); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// loadStableJob writes one stable job definition from the embedded copy and loads it.
func loadStableJob(ctx context.Context, client *launchd.Client, label string) error {
	definition, err := embedded.GetLaunchdJob(label, embedded.LaunchdStable)
	if err != nil {
		return fmt.Errorf("failed to read job definition %s: %w", label, err)
	}
	job := launchd.Job{Label: label, Dir: launchdJobDir}
	// Unload before rewriting so the new definition is the one launchd runs: launchd caches the
	// definition it loaded, and bootstrapping over a loaded job is a no-op. If the job can't be
	// unloaded, leave its file untouched and fail: the file then still matches what launchd runs,
	// and the failed install says the new definition is not in use.
	if err := client.Bootout(ctx, label); err != nil {
		return fmt.Errorf("failed to unload %s before reloading it: %w", label, err)
	}
	if err := job.Write(definition); err != nil {
		return fmt.Errorf("failed to write job definition %s: %w", label, err)
	}
	// A persistent disabled override prevents bootstrap, not just process startup.
	if err := client.Enable(ctx, label); err != nil {
		return fmt.Errorf("failed to enable %s: %w", label, err)
	}
	// Every job sets RunAtLoad, so bootstrapping it starts it. It is not kickstarted on top: a job
	// launchd cannot spawn, for instance one whose account is missing, would block that call.
	if err := client.Bootstrap(ctx, job); err != nil {
		return fmt.Errorf("failed to load %s: %w", label, err)
	}
	return nil
}

// postInstallDatadogAgent loads the stable job set.
//
// The Agent reaches macOS as a .dmg, so that is the only package type it handles.
func postInstallDatadogAgent(ctx HookContext) error {
	switch ctx.PackageType {
	case PackageTypeDMG:
		return postInstallDMG(ctx)
	default:
		return fmt.Errorf("unsupported package type for %s on macOS: %s", ctx.Package, ctx.PackageType)
	}
}

// postInstallDMG is the post-install the .dmg's postinstall script delegates to
// `installer postinst datadog-agent dmg`: setting up the account, directories, configuration,
// application and links (setUpDMGInstall), registering the Agent as a package, loading the stable
// job set, and installing the GUI LaunchAgents (startDMGGUI). If it fails, the script fails the
// install.
//
// Every step runs even if an earlier one fails, and in this order so the installer daemon finds its
// directories and the package registered when it starts: a failed registration only leaves Fleet
// without the Agent's version, and must not also leave the Agent stopped.
func postInstallDMG(ctx HookContext) error {
	var errs []error
	env := readInstallEnv(defaultDmgLayout)
	if err := setUpDMGInstall(ctx, defaultDmgLayout); err != nil {
		errs = append(errs, err)
	}
	// The packages root holds the package entry the installer daemon reports the Agent from, and
	// only root may write to it. Create it, or repair it, before the entry is registered below.
	if err := ensureInstallerDataDir(); err != nil {
		errs = append(errs, err)
	}
	if err := linkPackageEntry(ctx, defaultDmgLayout.installDir); err != nil {
		errs = append(errs, err)
	}
	// The Agent's jobs run as its account, and launchd cannot start them without it. If the account
	// could not be created, they are left unloaded and the hook fails, so the postinstall script
	// fails the install instead of waiting on jobs that will never run.
	if agentUserExists() {
		if err := installStableJobs(ctx); err != nil {
			errs = append(errs, err)
		}
	} else {
		errs = append(errs, fmt.Errorf("not loading the Agent's jobs: %s does not exist", agentUser))
	}
	startDMGGUI(ctx, defaultDmgLayout, env)
	return errors.Join(errs...)
}

// linkPackageEntry gives the Agent its entry in the package repository the shared installer code
// keeps for every package, /opt/datadog-packages/datadog-agent. The installer daemon reads that
// entry to report the packages installed on the host, and configuration experiments clean it before
// they start.
//
// The repository is a versioned package pool, with stable and experiment pointing at the versions
// in use. A .dmg install has a single version, installed in place in the install root, so the
// entry links there, the same links the deb and rpm packages ship on Linux
// (packages/agent/linux/BUILD.bazel):
//
//	/opt/datadog-packages/run/datadog-agent/<version> -> /opt/datadog-agent
//	/opt/datadog-packages/datadog-agent/stable -> /opt/datadog-packages/run/datadog-agent/<version>
//	/opt/datadog-packages/datadog-agent/experiment -> /opt/datadog-packages/datadog-agent/stable
//
// The version link lives outside the repository, where its garbage collection never looks, so a
// link to the install root is never taken for a package to delete. It is named after the Agent
// actually installed -- see registeredVersion -- because Fleet Automation reads it as the host's
// stable version and health-checks the running Agent against it, and it moves forward on every
// .dmg upgrade, which removes the previous version's link and the placeholder directories earlier
// builds registered. A real package already registered at stable, a directory with content, is
// left alone.
func linkPackageEntry(ctx HookContext, installDir string) (err error) {
	span, _ := ctx.StartSpan("link_package_entry")
	defer func() {
		span.Finish(err)
	}()

	root := filepath.Join(packagesRoot, ctx.Package)
	versionsDir := filepath.Join(packagesRoot, "run", ctx.Package)
	for _, dir := range []string{root, versionsDir} {
		if err = os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	isReal, err := stableIsRealPackage(root)
	if err != nil {
		return err
	}
	if isReal {
		return nil
	}

	pkgVersion := registeredVersion()
	versionLink := filepath.Join(versionsDir, pkgVersion)
	stableLink := filepath.Join(root, "stable")
	for _, link := range [][2]string{
		{versionLink, installDir},
		{stableLink, versionLink},
		{filepath.Join(root, "experiment"), stableLink},
	} {
		if err = symlink.Set(link[0], link[1]); err != nil {
			return fmt.Errorf("failed to link %s to %s: %w", link[0], link[1], err)
		}
	}
	removeOtherEntries(root, "stable", "experiment")
	removeOtherEntries(versionsDir, pkgVersion)
	return nil
}

// registeredVersion is the version the package entry is registered under.
//
// It has to be the same value the installer daemon reports as the package's running version, which
// is version.AgentPackageVersion (see runningVersions in pkg/fleet/daemon/daemon.go): Fleet
// Automation decides whether a host is healthy by comparing the two. version.AgentVersion is not
// interchangeable -- it is the human-facing form and uses '+' where AgentPackageVersion uses '.'.
func registeredVersion() string {
	if version.AgentPackageVersion != "" {
		return version.AgentPackageVersion
	}
	// Only builds that go through tasks/omnibus.py stamp AgentPackageVersion. Fall back so a
	// locally built installer registers a resolvable entry rather than failing on an empty version.
	return version.AgentVersion
}

// stableIsRealPackage reports whether the package's stable link resolves to a real package: a
// directory in the repository with content, as an OCI install registers. A version link to the
// install root, an empty placeholder directory from earlier builds, or no stable link at all is not.
func stableIsRealPackage(root string) (bool, error) {
	target, err := os.Readlink(filepath.Join(root, "stable"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read the stable link in %s: %w", root, err)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read the stable package %s: %w", target, err)
	}
	if !info.IsDir() {
		return false, nil
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return false, fmt.Errorf("failed to read the stable package %s: %w", target, err)
	}
	return len(entries) > 0, nil
}

// removeOtherEntries removes every entry of dir but the ones named in keep: links are removed
// themselves, never what they point to, and empty directories (the placeholders earlier builds
// registered) are removed. Anything else is left and logged.
func removeOtherEntries(dir string, keep ...string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Warnf("could not list %s: %v", dir, err)
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if slices.Contains(keep, name) {
			continue
		}
		path := filepath.Join(dir, name)
		switch {
		case entry.Type()&os.ModeSymlink != 0:
			if err := os.Remove(path); err != nil {
				log.Warnf("could not remove %s: %v", path, err)
			}
		case entry.IsDir():
			if err := os.Remove(path); err != nil {
				log.Warnf("leaving %s in place: %v", path, err)
			}
		default:
			log.Warnf("leaving unexpected file %s in place", path)
		}
	}
}

// preRemoveDatadogAgent unloads the stable job set and removes its definitions, in the reverse of
// the order they are loaded in. The .dmg's preinstall script runs it through the installed
// version's `installer prerm --upgrade datadog-agent dmg`, before the new payload replaces it, so
// the version being replaced takes down exactly the jobs it installed -- including any the next
// version no longer knows about, whose definitions launchd would otherwise load again at boot.
//
// The definitions are removed on an upgrade too: both install paths write every definition from
// scratch, so nothing is lost.
//
// When the Agent is uninstalled rather than upgraded, as cmd/agent/macos/uninstall_mac_os.sh does
// through `installer prerm datadog-agent dmg`, the rest of the install goes too (uninstallAgent).
//
// Every step runs even if an earlier one fails. On an upgrade nothing is reported: a job that is
// already gone is the desired outcome, the next version's postinstall reloads the jobs, and an
// error would only stop it from installing. On an uninstall, the jobs that could not be unloaded
// are reported once the rest is removed, so the uninstall script removes them itself instead of
// leaving them registered against binaries that are gone.
func preRemoveDatadogAgent(ctx HookContext) error {
	client := launchdClient()
	var errs []error
	for _, label := range slices.Backward(agentLaunchdJobs.LaunchdJobsStable) {
		if err := client.Bootout(ctx, label); err != nil {
			// As in loadStableJob, a job that can't be unloaded keeps its definition, so launchd is
			// never left running a job whose file is gone, and the postinstall script reloads it.
			log.Warnf("failed to unload %s: %v", label, err)
			errs = append(errs, fmt.Errorf("failed to unload %s: %w", label, err))
			continue
		}
		if err := os.Remove(launchd.Job{Label: label, Dir: launchdJobDir}.Path()); err != nil && !os.IsNotExist(err) {
			log.Warnf("failed to remove job definition %s: %v", label, err)
			errs = append(errs, fmt.Errorf("failed to remove job definition %s: %w", label, err))
		}
	}
	if ctx.Upgrade {
		return nil
	}
	uninstallAgent(ctx, defaultUninstallLayout)
	return errors.Join(errs...)
}

// uninstallLayout is everything an uninstall removes besides the LaunchDaemons. It is the same
// list cmd/agent/macos/uninstall_mac_os.sh removes for versions without the installer.
//
// It is parameterised on its paths so the tests can run against temporary directories.
type uninstallLayout struct {
	// guiLabels are the per-user LaunchAgents, booted out for every logged-in user.
	guiLabels []string
	// processPatterns are the pkill -f patterns of the GUI processes left behind.
	processPatterns []string
	// paths are removed in order, recursively. The install root, which holds the installer that is
	// running this, goes last.
	paths []string
}

var defaultUninstallLayout = uninstallLayout{
	guiLabels: []string{
		"com.datadoghq.gui",
		"com.datadoghq.ai-usage-agent.desktop-monitor",
		"com.datadoghq.ai-prompt-logger.desktop-monitor",
	},
	processPatterns: []string{
		"Datadog Agent.app",
		"ai-usage-agent-native-host.*--desktop-monitor",
		"ai-prompt-logger-native-host.*--desktop-monitor",
	},
	paths: []string{
		"/Library/LaunchAgents/com.datadoghq.gui.plist",
		"/Library/LaunchAgents/com.datadoghq.ai-usage-agent.desktop-monitor.plist",
		"/Library/LaunchAgents/com.datadoghq.ai-prompt-logger.desktop-monitor.plist",
		"/Applications/Datadog Agent.app",
		"/usr/local/bin/datadog-agent",
		"/usr/local/bin/datadog-installer",
		"/var/log/datadog",
		"/private/var/root/datadog-install",
		paths.PackagesPath,
		filepath.Dir(paths.AgentConfigDir),
	},
}

// hostCommand runs the system commands the install and uninstall need: launchctl, dscl, chmod,
// pkill, who and the like. Indirected so the hook tests can run without them.
var hostCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return telemetry.CommandContext(ctx, name, args...).Output()
}

// uninstallAgent stops the GUI for every logged-in user and removes what the install left besides
// the LaunchDaemons: the LaunchAgents, the application, the links and the install root. Every step
// is allowed to fail, so one missing piece doesn't stop the rest from going.
func uninstallAgent(ctx context.Context, layout uninstallLayout) {
	for _, uid := range loggedInUIDs(ctx) {
		for _, label := range layout.guiLabels {
			_, _ = hostCommand(ctx, "launchctl", "bootout", "gui/"+uid+"/"+label)
		}
	}
	for _, pattern := range layout.processPatterns {
		_, _ = hostCommand(ctx, "pkill", "-f", pattern)
	}
	// os.RemoveAll removes a link itself rather than what it points to.
	for _, path := range layout.paths {
		if err := os.RemoveAll(path); err != nil {
			log.Warnf("failed to remove %s: %v", path, err)
		}
	}
}

// loggedInUIDs returns the uids of the users with a login session, whose GUI domains hold the
// Agent's LaunchAgents.
func loggedInUIDs(ctx context.Context) []string {
	out, err := hostCommand(ctx, "who")
	if err != nil {
		log.Warnf("could not list logged-in users: %v", err)
		return nil
	}
	var uids []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		u, err := user.Lookup(fields[0])
		if err != nil || slices.Contains(uids, u.Uid) {
			continue
		}
		uids = append(uids, u.Uid)
	}
	return uids
}
