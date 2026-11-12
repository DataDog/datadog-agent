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
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/symlink"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// watchConfigExperimentCommand is the package-command name the detached watcher process runs,
// dispatched through runDatadogAgentPackageCommand (see packages_darwin.go's packageCommands).
const watchConfigExperimentCommand = "watchConfigExperiment"

// handOverInstallerCommand is the package-command name of the detached process that hands the
// installer daemon over from its stable job to its experiment one when a configuration experiment
// starts. The experiment daemon then launches the watcher that supervises the experiment.
const handOverInstallerCommand = "handOverInstaller"

// handBackInstallerCommand is the package-command name of the detached process that hands the
// installer daemon back to its stable job after a configuration experiment is stopped or promoted.
const handBackInstallerCommand = "handBackInstaller"

// experimentDaemonEnv is set, to "true", only in the experiment daemon's job definition (see
// com.datadoghq.installer.plist.tmpl). The installer commands the daemon runs, and the hooks they
// run, inherit it. The resume hook reads it.
const experimentDaemonEnv = "DD_INSTALLER_EXPERIMENT_DAEMON"

// runByExperimentDaemon reports whether this process descends from the experiment daemon, as
// opposed to the stable one or an installer command run by hand.
func runByExperimentDaemon() bool {
	return os.Getenv(experimentDaemonEnv) == "true"
}

var datadogAgentPackage = hooks{
	preRemove:   preRemoveDatadogAgent,
	postInstall: postInstallDatadogAgent,

	preStartExperiment:   preStartExperimentDatadogAgent,
	prePromoteExperiment: prePromoteExperimentDatadogAgent,

	postStartConfigExperiment:   postStartConfigExperimentDatadogAgent,
	preStopConfigExperiment:     preStopConfigExperimentDatadogAgent,
	postPromoteConfigExperiment: postPromoteConfigExperimentDatadogAgent,
	resumeConfigExperiment:      resumeConfigExperimentDatadogAgent,
}

// datadogAgentLaunchdJobs are the launchd jobs of the Agent package, in the shape of
// datadogAgentService on Linux.
type datadogAgentLaunchdJobs struct {
	// LaunchdMainJobStable is the core Agent's job.
	LaunchdMainJobStable string
	// LaunchdJobsStable are the jobs that run normally, in the order they are loaded.
	LaunchdJobsStable []string
	// LaunchdJobsExp are the jobs a configuration experiment swaps for their -exp variant, by
	// label without the suffix, in the order they are loaded.
	LaunchdJobsExp []string
}

// agentJob is the core Agent's launchd job.
const agentJob = "com.datadoghq.agent"

// installerJob is the root installer daemon, which drives experiments. It is loaded last, once the
// Agent it reports on is up.
//
// A configuration experiment swaps it too, so the daemon runs under the experiment configuration
// like the Agent, but never from a hook: a hook runs inside the daemon's own job, and launchd kills
// everything left in a job's process group when it stops the job, so a hook that swapped the daemon
// would stop itself halfway through. A detached process does it instead: the hand-over process
// moves the daemon to the experiment (handOverInstaller), and a hand-back process returns it once
// the experiment is stopped or promoted (handBackInstaller).
//
// The experiment daemon, not the stable one, launches the watcher that supervises the experiment:
// the supervisor is then the code the experiment runs under, which is the only one that can know
// every job the experiment swaps (see resumeConfigExperimentDatadogAgent).
const installerJob = "com.datadoghq.installer"

// procmgrJob is the process manager. It is loaded first, so it is up before the processes it will
// supervise. It has no experiment variant: a configuration experiment leaves it running on its
// stable job, reading the stable configuration. When the experiment ends it is loaded again if
// anything stopped it, and restarted on a promote so it reads the promoted configuration
// (ensureProcmgr).
const procmgrJob = "com.datadoghq.procmgr"

// experimentJobOrder is the order the jobs a configuration experiment swaps are loaded in. A job
// the embedded definitions add beyond these is loaded after them, in name order, and the installer
// daemon is always last, so it is the last job to come back to the stable set.
var experimentJobOrder = []string{agentJob, "com.datadoghq.sysprobe", "com.datadoghq.data-plane"}

// configExperimentJobs are every job a configuration experiment swaps, in the order they are
// loaded. They are the jobs the embedded definitions have an experiment variant for, not a list
// kept beside them: the installer that runs an experiment supervises and swaps every job its own
// definitions describe, so a service a newer version adds is covered without touching this file.
var configExperimentJobs = orderedExperimentJobs()

// agentJobs are the jobs the configuration experiment hooks swap themselves: every swapped job but
// the installer daemon, in the order they are loaded.
var agentJobs = withoutJob(configExperimentJobs, installerJob)

// stableOnlyJobs are the jobs with no experiment variant, which an experiment leaves on their
// stable definition: the process manager today. They are loaded before the others.
var stableOnlyJobs = orderedStableOnlyJobs()

// orderedExperimentJobs returns the labels the embedded definitions have an experiment variant
// for, in loading order (see experimentJobOrder).
func orderedExperimentJobs() []string {
	labels, err := embedded.LaunchdJobs(embedded.LaunchdExperiment)
	if err != nil {
		// The definitions are compiled in, so this cannot happen. Fall back on the jobs known today.
		log.Errorf("could not list the experiment job definitions: %v", err)
		return append(append([]string{}, experimentJobOrder...), installerJob)
	}
	rank := func(label string) int {
		if label == installerJob {
			return len(experimentJobOrder) + 1
		}
		if i := slices.Index(experimentJobOrder, label); i >= 0 {
			return i
		}
		return len(experimentJobOrder)
	}
	slices.SortFunc(labels, func(a, b string) int {
		if c := rank(a) - rank(b); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	return labels
}

// orderedStableOnlyJobs returns the labels the embedded definitions describe only in their stable
// variant, in name order, with the process manager first.
func orderedStableOnlyJobs() []string {
	stable, err := embedded.LaunchdJobs(embedded.LaunchdStable)
	if err != nil {
		log.Errorf("could not list the job definitions: %v", err)
		return []string{procmgrJob}
	}
	var labels []string
	for _, label := range stable {
		if !slices.Contains(configExperimentJobs, label) {
			labels = append(labels, label)
		}
	}
	slices.SortFunc(labels, func(a, b string) int {
		if (a == procmgrJob) != (b == procmgrJob) {
			if a == procmgrJob {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	return labels
}

// withoutJob returns labels without label, keeping the order.
func withoutJob(labels []string, label string) []string {
	return slices.DeleteFunc(slices.Clone(labels), func(l string) bool { return l == label })
}

// agentLaunchdJobs are the jobs the Agent package installs.
var agentLaunchdJobs = datadogAgentLaunchdJobs{
	LaunchdMainJobStable: agentJob,
	LaunchdJobsStable:    append(slices.Clone(stableOnlyJobs), configExperimentJobs...),
	LaunchdJobsExp:       configExperimentJobs,
}

// stableJobs are the launchd jobs that run normally, in the order they are loaded.
var stableJobs = agentLaunchdJobs.LaunchdJobsStable

// experimentJobs are the jobs an experiment runs under, by full label.
var experimentJobs = experimentLabels()

func experimentLabels() []string {
	labels := make([]string, 0, len(configExperimentJobs))
	for _, label := range configExperimentJobs {
		labels = append(labels, label+string(launchd.Experiment))
	}
	return labels
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

// agentJobSet is the set the hooks swap, in the system domain. It excludes the installer daemon.
func agentJobSet() launchd.JobSet {
	return launchd.JobSet{Labels: agentJobs, Dir: launchdJobDir, Client: launchdClient()}
}

// installerJobSet is the installer daemon alone, in the system domain.
func installerJobSet() launchd.JobSet {
	return launchd.JobSet{Labels: []string{installerJob}, Dir: launchdJobDir, Client: installerLaunchdClient()}
}

// configExperimentJobSet is every job a configuration experiment swaps, in the system domain. Only
// a process outside the installer daemon's job may swap it: see installerJob.
func configExperimentJobSet() launchd.JobSet {
	return launchd.JobSet{Labels: configExperimentJobs, Dir: launchdJobDir, Client: installerLaunchdClient()}
}

// installerBootoutSettleTimeout bounds how long a bootout of the installer daemon waits for it to
// leave the domain. On SIGTERM the daemon first finishes the request it is handling, and launchd
// only kills it once the ExitTimeOut in its definition (10 seconds,
// com.datadoghq.installer.plist.tmpl) runs out.
const installerBootoutSettleTimeout = 30 * time.Second

// installerLaunchdClient is launchdClient, waiting long enough for the installer daemon to leave.
func installerLaunchdClient() *launchd.Client {
	client := launchdClient()
	client.BootoutSettleTimeout = installerBootoutSettleTimeout
	return client
}

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
	collapseConfigExperiment(ctx)
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
		unloadExperimentJobs(ctx)
		if err := installStableJobs(ctx); err != nil {
			errs = append(errs, err)
		}
	} else {
		errs = append(errs, fmt.Errorf("not loading the Agent's jobs: %s does not exist", agentUser))
	}
	startDMGGUI(ctx, defaultDmgLayout, env)
	return errors.Join(errs...)
}

// collapseConfigExperiment discards whatever is left of a configuration experiment before the
// install, so running the post-install twice is harmless: it stops a surviving watcher, which would
// read the experiment going away as a crash and revert on top of the install, and clears the
// deadline, so the next daemon does not resume it. setUpDMGInstall then discards the experiment
// configuration (restExperimentConfig), and unloadExperimentJobs its jobs.
//
// The .dmg's preinstall script already does this on an upgrade; this repeats it for a host it did
// not run on or did not finish on. Every step is allowed to fail.
func collapseConfigExperiment(ctx HookContext) {
	if err := stopSurvivingWatchers(ctx); err != nil {
		log.Warnf("could not stop a surviving experiment watcher: %v", err)
	}
	if err := experimentDeadline().Clear(); err != nil {
		log.Warnf("could not clear the experiment deadline: %v", err)
	}
}

// unloadExperimentJobs unloads and removes the experiment job set before the stable one is loaded,
// so a host never ends up running both -- two Agents, or two installer daemons. Every step is
// allowed to fail: an experiment job left behind is not loaded at boot.
func unloadExperimentJobs(ctx HookContext) {
	jobs := configExperimentJobSet()
	if err := jobs.Stop(ctx, launchd.Experiment); err != nil {
		log.Warnf("could not unload the experiment jobs: %v", err)
	}
	if err := jobs.Remove(launchd.Experiment); err != nil {
		log.Warnf("could not remove the experiment job definitions: %v", err)
	}
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
	// The experiment set first: a leftover -exp job would otherwise keep running against a
	// configuration that is about to be removed. Its definitions are not loaded at boot, so a job
	// that can't be unloaded only leaves an unused file behind.
	experiment := configExperimentJobSet()
	if err := experiment.Stop(ctx, launchd.Experiment); err != nil {
		log.Warnf("failed to unload the experiment jobs: %v", err)
		errs = append(errs, err)
	}
	if err := experiment.Remove(launchd.Experiment); err != nil {
		log.Warnf("failed to remove the experiment job definitions: %v", err)
		errs = append(errs, err)
	}
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

// errVersionExperimentNotSupported is returned for a version experiment: nothing on macOS swaps the
// running binaries yet, so with no hook to refuse it the installer would switch the experiment
// pointer, restart nothing, and report the task as done.
var errVersionExperimentNotSupported = errors.New("version experiments are not supported on macOS")

// preStartExperimentDatadogAgent refuses a version experiment before the installer sets it, so the
// package repository is left as it was and the task is reported as failed.
func preStartExperimentDatadogAgent(_ HookContext) error {
	return errVersionExperimentNotSupported
}

// prePromoteExperimentDatadogAgent refuses to promote a version experiment, for the same reason
// preStartExperimentDatadogAgent refuses to start one.
func prePromoteExperimentDatadogAgent(_ HookContext) error {
	return errVersionExperimentNotSupported
}

// postStartConfigExperimentDatadogAgent hands the Agent over to the experiment job set, then
// launches the detached process that hands the installer daemon over too, once this hook's
// installer command has finished (handOverInstaller). The experiment daemon, once it is up,
// launches the watcher that supervises the whole set (resumeConfigExperimentDatadogAgent). The
// installer has already published the experiment configuration directory by the time it runs.
//
// A hand-over that fails to launch is treated the same as a failed start: with no daemon to launch
// it, an experiment would have no watcher, so it is reverted rather than left running
// unsupervised.
//
// Every failure hands the Agent back to the stable job set, so the experiment configuration
// directory the installer published is discarded with it: the installer returns the error without
// removing it, and left in place it would have the failed experiment reported as running.
func postStartConfigExperimentDatadogAgent(ctx HookContext) (err error) {
	defer func() {
		if err == nil {
			return
		}
		if discardErr := discardExperimentConfig(ctx); discardErr != nil {
			err = fmt.Errorf("%w, and %w", err, discardErr)
		}
	}()
	if err = (configExperiment{jobs: agentJobSet()}).Start(ctx); err != nil {
		return err
	}
	if handOverErr := launchInstallerHandOver(ctx); handOverErr != nil {
		log.Errorf("could not launch the installer daemon hand-over, reverting: %v", handOverErr)
		if revertErr := (configExperiment{jobs: agentJobSet()}).Stop(context.WithoutCancel(ctx)); revertErr != nil {
			return fmt.Errorf("hand-over failed to launch (%w) and the experiment could not be reverted: %w", handOverErr, revertErr)
		}
		return fmt.Errorf("hand-over failed to launch, experiment reverted: %w", handOverErr)
	}
	return nil
}

// launchConfigExperimentWatcher starts the detached watcher process. Only the experiment daemon
// launches it.
//
// Indirected so tests can stub it without spawning a real process.
var launchConfigExperimentWatcher = func(ctx context.Context) error {
	return startDetachedPackageCommand(ctx, watchConfigExperimentCommand)
}

// launchInstallerHandOver starts the detached process that hands the installer daemon over from
// its stable job to its experiment one. Indirected for the same reason as
// launchConfigExperimentWatcher.
var launchInstallerHandOver = func(ctx context.Context) error {
	return startDetachedPackageCommand(ctx, handOverInstallerCommand)
}

// launchInstallerHandBack starts the detached process that hands the installer daemon back to its
// stable job. Indirected for the same reason as launchConfigExperimentWatcher.
var launchInstallerHandBack = func(ctx context.Context) error {
	return startDetachedPackageCommand(ctx, handBackInstallerCommand)
}

// startDetachedPackageCommand re-execs the installer binary with the hidden `package-command`
// subcommand, as the Windows experiment watchdog does, detached from this process's context and in
// a session of its own, so it outlives both the hook that launched it and the daemon job that hook
// runs in.
func startDetachedPackageCommand(ctx context.Context, command string) error {
	installerBin, err := exec.GetExecutable()
	if err != nil {
		return fmt.Errorf("could not get the installer executable path: %w", err)
	}
	installerBin, err = filepath.EvalSymlinks(installerBin)
	if err != nil {
		return fmt.Errorf("could not resolve the installer executable path: %w", err)
	}
	installer := exec.NewInstallerExec(env.FromEnv(), installerBin)
	return installer.StartPackageCommandDetached(ctx, agentPackage, command)
}

// runDatadogAgentPackageCommand dispatches commands run by a detached process launched via
// startDetachedPackageCommand. Registered into packageCommands in packages_darwin.go.
func runDatadogAgentPackageCommand(ctx context.Context, command string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, command)
	defer func() { span.Finish(err) }()

	switch command {
	case watchConfigExperimentCommand:
		return watchExperiment(ctx)
	case handOverInstallerCommand:
		return runInstallerHandOver(ctx)
	case handBackInstallerCommand:
		return handBackInstaller(ctx)
	default:
		return fmt.Errorf("unknown package command: %s", command)
	}
}

// preStopConfigExperimentDatadogAgent hands the Agent back to the stable job set, before the
// installer discards the experiment configuration directory, then has the installer daemon handed
// back too.
//
// The context is detached from cancellation: the experiment is being torn down, and stopping
// halfway would leave the host running neither set.
func preStopConfigExperimentDatadogAgent(ctx HookContext) error {
	ctx.Context = context.WithoutCancel(ctx.Context)
	stopErr := configExperiment{jobs: agentJobSet()}.Stop(ctx)
	return errors.Join(stopErr, handBackInstallerAfterHook(ctx))
}

// postPromoteConfigExperimentDatadogAgent hands the Agent back to the stable job set, which now
// reads the promoted configuration, then has the installer daemon handed back too.
func postPromoteConfigExperimentDatadogAgent(ctx HookContext) error {
	ctx.Context = context.WithoutCancel(ctx.Context)
	promoteErr := configExperiment{jobs: agentJobSet()}.Promote(ctx)
	return errors.Join(promoteErr, handBackInstallerAfterHook(ctx))
}

// handBackInstallerAfterHook launches the detached hand-back process when the experiment daemon is
// loaded, whoever runs the hook: the experiment daemon itself, or an operator running the installer
// by hand. It is launched even when the Agent could not be handed back: the stable daemon that
// comes back runs the resume hook, which reverts an experiment left half torn down.
//
// With no experiment daemon loaded, as when the stable daemon runs the hook, there is nothing to
// hand back: the installer daemon is already on its stable job.
func handBackInstallerAfterHook(ctx context.Context) error {
	loaded, err := launchdClient().Loaded(ctx, installerJob+string(launchd.Experiment))
	if err != nil {
		return fmt.Errorf("could not tell whether the experiment daemon is loaded: %w", err)
	}
	if !loaded {
		return nil
	}
	if err := launchInstallerHandBack(ctx); err != nil {
		return fmt.Errorf("could not launch the installer daemon hand-back: %w", err)
	}
	return nil
}
