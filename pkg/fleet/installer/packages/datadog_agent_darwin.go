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
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/installinfo"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// watchConfigExperimentCommand is the package-command name the detached watcher process runs,
// dispatched through runDatadogAgentPackageCommand (see packages_darwin.go's packageCommands).
const watchConfigExperimentCommand = "watchConfigExperiment"

var datadogAgentPackage = hooks{
	preInstall:  preInstallDatadogAgent,
	postInstall: postInstallDatadogAgent,
	preRemove:   preRemoveDatadogAgent,

	postStartConfigExperiment:   postStartConfigExperimentDatadogAgent,
	preStopConfigExperiment:     preStopConfigExperimentDatadogAgent,
	postPromoteConfigExperiment: postPromoteConfigExperimentDatadogAgent,
	resumeConfigExperiment:      resumeConfigExperimentDatadogAgent,
}

const (
	// convenienceLinkDir is where the user-facing commands live. /usr/local/bin rather than
	// /usr/bin: /usr/bin is on the read-only system volume and cannot be written to.
	convenienceLinkDir = "/usr/local/bin"
)

// agentLayout is where the hooks find what they manage on disk. The .dmg's preinstall and
// postinstall scripts create the account and the install root's directories, with their
// ownership, modes and access control lists, so the hooks never do.
//
// It is parameterised on its roots so the tests can run against temporary directories as an
// unprivileged user. Production always uses defaultAgentLayout.
type agentLayout struct {
	// installRoot is the single root everything the Agent owns lives under: the binaries
	// alongside etc, etc-exp, run and logs. Created once, preserved across every upgrade.
	installRoot string
	// linkDir is where the convenience commands are linked from.
	linkDir string
	// packagesRoot is the root of the OCI package repositories the shared installer code keeps.
	// It sits outside the install root, and macOS stores nothing of its own there.
	packagesRoot string
}

var defaultAgentLayout = agentLayout{
	installRoot:  filepath.Dir(paths.AgentConfigDir),
	linkDir:      convenienceLinkDir,
	packagesRoot: paths.PackagesPath,
}

// convenienceLinks are the user-facing commands the .dmg links. They name the install root, which
// is the same address for the life of the machine, so they never need updating.
func (l agentLayout) convenienceLinks() map[string]string {
	return map[string]string{
		filepath.Join(l.linkDir, "datadog-agent"):     filepath.Join(l.installRoot, "bin", "agent", "agent"),
		filepath.Join(l.linkDir, "datadog-installer"): filepath.Join(l.installRoot, "embedded", "bin", "installer"),
	}
}

// agentJobs are the launchd jobs that are defined in both variants, in the order they are loaded.
// These are the jobs a configuration experiment swaps.
var agentJobs = []string{
	"com.datadoghq.agent",
	"com.datadoghq.sysprobe",
	"com.datadoghq.data-plane",
}

// installerJob is the daemon that drives experiments. It has no experiment variant: the process
// that starts and stops an experiment cannot be part of the set it swaps, or it would stop itself
// halfway through.
const installerJob = "com.datadoghq.installer"

// stableJobs are the launchd jobs that run normally, in the order they are loaded.
var stableJobs = append(append([]string{}, agentJobs...), installerJob)

// experimentJobs are the jobs an experiment runs under, by full label.
var experimentJobs = experimentLabels()

func experimentLabels() []string {
	labels := make([]string, 0, len(agentJobs))
	for _, label := range agentJobs {
		labels = append(labels, label+string(launchd.Experiment))
	}
	return labels
}

// launchdClient and launchdJobDir are indirected so the hook tests can run without launchd.
var (
	launchdClient = func() *launchd.Client { return launchd.NewClient(launchd.System) }
	launchdJobDir = launchd.System.Dir()
)

// agentJobSet is the swappable set, in the system domain. It excludes the installer daemon.
func agentJobSet() launchd.JobSet {
	return launchd.JobSet{Labels: agentJobs, Dir: launchdJobDir, Client: launchdClient()}
}

// stableJob returns the job definition record for a label in the system domain.
func stableJob(label string) launchd.Job {
	return launchd.Job{
		Label:     label,
		PlistPath: filepath.Join(launchdJobDir, label+".plist"),
		Domain:    launchd.System,
	}
}

// registerPackageRepository registers the Agent in the OCI package repository the shared installer
// code keeps for every package.
//
// macOS installs the Agent from a .dmg rather than from an OCI package, so nothing else on this
// platform ever creates that repository -- but the shared code reads it unconditionally.
// InstallConfigExperiment cleans the package repository before it writes the configuration
// experiment, so on a host without one the read fails with ENOENT and no configuration experiment
// can start at all.
//
// The version directory the links resolve to is a placeholder. macOS keeps no versioned package
// pool: the files the entry stands for live in the install root, and are replaced there by the
// next .dmg rather than installed alongside. What the shared code needs is a repository whose
// stable and experiment links resolve, which is what this creates.
//
// The version it is registered under is not a placeholder, though: it is what Fleet Automation
// reads as the host's stable version and health-checks the running Agent against, so it has to
// name the Agent actually installed -- see registeredVersion -- and has to move forward when a
// .dmg replaces that Agent.
func registerPackageRepository(ctx HookContext, layout agentLayout) (err error) {
	span, ctx := ctx.StartSpan("register_package_repository")
	defer func() {
		span.Finish(err)
	}()

	if err = os.MkdirAll(layout.packagesRoot, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", layout.packagesRoot, err)
	}
	repositories := repository.NewRepositories(layout.packagesRoot, AsyncPreRemoveHooks)

	state, err := repositories.GetState(ctx.Package)
	if err != nil {
		return fmt.Errorf("failed to read the %s package state: %w", ctx.Package, err)
	}
	if state.HasStable() {
		isPlaceholder, err := stableIsPlaceholder(repositories, ctx.Package)
		if err != nil {
			return err
		}
		// A real package is left alone. Repository.Create removes whatever it finds before
		// writing, so re-registering would discard it: once the Agent can be installed from an OCI
		// package, doInstall has registered that package and pointed stable at it before the
		// wrapped .pkg runs -- and the .pkg's postinstall script is the same one a .dmg install
		// runs, so it reaches this code with the real package already in place. Its version is the
		// one the Agent underneath reports anyway, so there is nothing to refresh.
		if !isPlaceholder {
			return nil
		}
		// A placeholder this function registered on an earlier .dmg install. The .dmg replaces the
		// Agent in the install root without touching this entry, so after an upgrade it still
		// names the version installed first while the binaries it stands for are newer. The entry
		// is what Fleet Automation health-checks the host against, so it has to move forward with
		// the Agent: fall through and register it again under the current version.
		if state.Stable == registeredVersion() {
			return nil
		}
	}

	// Create wants a source directory to move in as the version, and takes ownership of it. An
	// empty one stands in for the package pool this platform does not have.
	placeholder, err := repositories.MkdirTemp()
	if err != nil {
		return fmt.Errorf("failed to create the placeholder package directory: %w", err)
	}
	// Only reached if Create failed before moving it.
	defer os.RemoveAll(placeholder)

	if err = repositories.Create(ctx, ctx.Package, registeredVersion(), placeholder); err != nil {
		return fmt.Errorf("failed to register %s as a package: %w", ctx.Package, err)
	}
	return nil
}

// registeredVersion is the version the placeholder package entry is registered under.
//
// It has to be the same value the installer daemon reports as the package's running version, which
// is version.AgentPackageVersion (see runningVersions in pkg/fleet/daemon/daemon.go): Fleet
// Automation decides whether a host is healthy by comparing the two, and fails every deployment to
// it before pushing any configuration when they differ. version.AgentVersion is not
// interchangeable -- it is the human-facing form and uses '+' where AgentPackageVersion uses '.'.
func registeredVersion() string {
	if version.AgentPackageVersion != "" {
		return version.AgentPackageVersion
	}
	// Only builds that go through tasks/omnibus.py stamp AgentPackageVersion. Fall back so a
	// locally built installer registers a resolvable entry rather than failing on an empty version.
	return version.AgentVersion
}

// stableIsPlaceholder reports whether the package's stable link resolves to a placeholder entry --
// the empty directory registerPackageRepository moves in -- rather than to a real package.
//
// It is what tells an entry this function owns, and may replace to move the version forward, apart
// from one an OCI install registered, which holds the package's files and must not be touched.
func stableIsPlaceholder(repositories *repository.Repositories, pkg string) (bool, error) {
	stablePath := repositories.Get(pkg).StablePath()
	entries, err := os.ReadDir(stablePath)
	if err != nil {
		return false, fmt.Errorf("failed to read the %s stable package directory %s: %w", pkg, stablePath, err)
	}
	return len(entries) == 0, nil
}

// installStableJobs writes and loads the stable launchd job set, including the installer daemon.
//
// Both install paths run this, from the same embedded definitions, so a host installed from the
// .dmg and a host installed by Fleet end up with byte-identical job definitions.
func installStableJobs(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("install_stable_jobs")
	defer func() {
		span.Finish(err)
	}()

	client := launchdClient()
	for _, label := range stableJobs {
		if err := loadStableJob(ctx, client, label); err != nil {
			return err
		}
	}
	return nil
}

// loadStableJob writes one stable job definition from the embedded copy and loads it.
func loadStableJob(ctx context.Context, client *launchd.Client, label string) error {
	definition, err := embedded.GetLaunchdJob(label, embedded.LaunchdStable)
	if err != nil {
		return fmt.Errorf("failed to read job definition %s: %w", label, err)
	}
	job := stableJob(label)
	if err := job.Write(definition); err != nil {
		return fmt.Errorf("failed to write job definition %s: %w", label, err)
	}
	// Bootout first so a rewritten definition is the one launchd is running: launchd
	// caches the definition it loaded, and bootstrapping over a loaded job is a no-op.
	if err := client.Bootout(ctx, label); err != nil {
		log.Warnf("failed to unload %s before reloading it: %v", label, err)
	}
	// A persistent disabled override prevents bootstrap, not just process startup.
	if err := client.Enable(ctx, label); err != nil {
		return fmt.Errorf("failed to enable %s: %w", label, err)
	}
	if err := client.Bootstrap(ctx, job); err != nil {
		return fmt.Errorf("failed to load %s: %w", label, err)
	}
	if err := client.Kickstart(ctx, label, false); err != nil {
		return fmt.Errorf("failed to start %s: %w", label, err)
	}
	return nil
}

// removeJobs unloads the given jobs and removes their definitions. Every step is allowed to fail:
// it runs on the uninstall path, where a job that is already gone is the desired outcome.
func removeJobs(ctx HookContext, labels []string) {
	client := launchdClient()
	for _, label := range labels {
		removeJob(ctx, client, label)
	}
}

// removeJob unloads one job and removes its definition.
func removeJob(ctx context.Context, client *launchd.Client, label string) {
	if err := client.Bootout(ctx, label); err != nil {
		log.Warnf("failed to unload %s: %v", label, err)
	}
	if err := stableJob(label).Remove(); err != nil {
		log.Warnf("failed to remove job definition %s: %v", label, err)
	}
}

// uninstallFilesystem removes the symlinks the install created. The install root itself is left
// alone: it holds configuration, and an uninstall is not a licence to discard it.
func uninstallFilesystem(ctx HookContext, layout agentLayout) {
	for link := range layout.convenienceLinks() {
		if err := file.EnsureSymlinkAbsent(ctx, link); err != nil {
			log.Warnf("failed to remove convenience link %s: %v", link, err)
		}
	}
}

// preInstallDatadogAgent stops the stable job set so the binaries it is running can be replaced.
// All the steps are allowed to fail: there may be nothing installed yet.
func preInstallDatadogAgent(ctx HookContext) error {
	client := launchdClient()
	for _, label := range stableJobs {
		if err := client.Bootout(ctx, label); err != nil {
			log.Warnf("failed to unload %s: %v", label, err)
		}
	}
	return nil
}

// postInstallDatadogAgent registers the Agent as a package and loads the stable job set.
//
// The Agent reaches macOS only as a .dmg, run directly or wrapped in an OCI package, so those are
// the only package types it handles.
func postInstallDatadogAgent(ctx HookContext) error {
	switch ctx.PackageType {
	case PackageTypeOCI:
		// The OCI layer wraps a .pkg installer payload rather than raw binaries, mirroring how
		// datadog_agent_windows.go wraps an MSI: doInstall's Create() call has already moved the
		// extracted layer into the package repository and flipped stable/experiment to it before
		// this hook runs, so registerPackageRepository has nothing left to do. What remains is
		// running the wrapped .pkg, whose own postinstall script sets up the filesystem and runs
		// the dmg branch below -- the same script a real .dmg install runs directly.
		return installWrappedPackage(ctx)
	case PackageTypeDMG:
		return postInstallDMG(ctx, defaultAgentLayout)
	default:
		return fmt.Errorf("unsupported package type for %s on macOS: %s", ctx.Package, ctx.PackageType)
	}
}

// postInstallDMG is the part of the post-install the .dmg's postinstall script delegates to
// `installer postinst datadog-agent dmg`: registering the Agent as a package and loading the stable
// job set.
//
// The script creates the account and the directories, with their ownership, modes and access
// control lists, and writes the install info itself, so none of that happens here.
//
// Both steps run even if the other fails, and in this order so the daemon finds the package
// registered when it starts: a failed registration only disables configuration experiments, and
// must not also leave the Agent stopped.
func postInstallDMG(ctx HookContext, layout agentLayout) error {
	var errs []error
	if err := registerPackageRepository(ctx, layout); err != nil {
		errs = append(errs, err)
	}
	if err := installStableJobs(ctx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// installWrappedPackage runs the .pkg installer payload an OCI-delivered Agent package wraps.
//
// macOS has no OCI/MSI-equivalent silent-install mechanism of its own: the only way to run a .pkg
// non-interactively is Apple's installer(8). ctx.PackagePath is the version directory Create()
// already moved the extracted OCI layer into (repository.Repository.StablePath() or
// ExperimentPath(), via hooksCLI.getPath()), so the payload to run is whatever single .pkg that
// layer contains.
func installWrappedPackage(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("install_wrapped_package")
	defer func() {
		span.Finish(err)
	}()

	matches, err := filepath.Glob(filepath.Join(ctx.PackagePath, "*.pkg"))
	if err != nil {
		return fmt.Errorf("failed to look for a .pkg in %s: %w", ctx.PackagePath, err)
	}
	if len(matches) == 0 {
		return fmt.Errorf("no .pkg found in %s", ctx.PackagePath)
	}
	if len(matches) > 1 {
		return fmt.Errorf("multiple .pkg found in %s: %v", ctx.PackagePath, matches)
	}

	cmd := telemetry.CommandContext(ctx, "installer", "-pkg", matches[0], "-target", "/")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("failed to run installer on %s: %w", matches[0], err)
	}
	return nil
}

// preRemoveDatadogAgent stops and removes both job sets.
// All the steps are allowed to fail.
func preRemoveDatadogAgent(ctx HookContext) error {
	// The experiment set first: a leftover -exp job would otherwise keep running against a
	// configuration that is about to be removed.
	removeJobs(ctx, experimentJobs)
	removeJobs(ctx, stableJobs)
	if !ctx.Upgrade {
		uninstallFilesystem(ctx, defaultAgentLayout)
		installinfo.RemoveInstallInfo()
	}
	return nil
}

// postStartConfigExperimentDatadogAgent hands the Agent over to the experiment job set, then
// launches the detached watcher process that supervises it. The installer has already published
// the experiment configuration directory by the time it runs.
//
// A watcher that fails to launch is treated the same as a failed start: an experiment running
// with no process watching its deadline or its exit is exactly the gap this feature exists to
// close, so it is reverted rather than left running unsupervised.
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
	if watcherErr := launchConfigExperimentWatcher(ctx); watcherErr != nil {
		log.Errorf("could not launch the configuration experiment watcher, reverting: %v", watcherErr)
		if revertErr := (configExperiment{jobs: agentJobSet()}).Stop(context.WithoutCancel(ctx)); revertErr != nil {
			return fmt.Errorf("watcher failed to launch (%w) and the experiment could not be reverted: %w", watcherErr, revertErr)
		}
		return fmt.Errorf("watcher failed to launch, experiment reverted: %w", watcherErr)
	}
	return nil
}

// launchConfigExperimentWatcher starts the detached watcher process, mirroring the Windows
// experiment watchdog's launchPackageCommandInBackground: it re-execs the installer binary with
// the hidden `package-command` subcommand, detached from this process's context so it outlives
// the hook that launched it.
//
// Indirected so tests can stub it without spawning a real process, the same way launchdClient is
// indirected for launchd.
var launchConfigExperimentWatcher = func(ctx context.Context) error {
	installerBin, err := exec.GetExecutable()
	if err != nil {
		return fmt.Errorf("could not get the installer executable path: %w", err)
	}
	installerBin, err = filepath.EvalSymlinks(installerBin)
	if err != nil {
		return fmt.Errorf("could not resolve the installer executable path: %w", err)
	}
	installer := exec.NewInstallerExec(env.FromEnv(), installerBin)
	return installer.StartPackageCommandDetached(ctx, agentPackage, watchConfigExperimentCommand)
}

// runDatadogAgentPackageCommand dispatches commands run by a detached process launched via
// launchConfigExperimentWatcher. Registered into packageCommands in packages_darwin.go.
func runDatadogAgentPackageCommand(ctx context.Context, command string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, command)
	defer func() { span.Finish(err) }()

	switch command {
	case watchConfigExperimentCommand:
		return watchExperiment(ctx)
	default:
		return fmt.Errorf("unknown package command: %s", command)
	}
}

// preStopConfigExperimentDatadogAgent hands the Agent back to the stable job set, before the
// installer discards the experiment configuration directory.
//
// The context is detached from cancellation: the experiment is being torn down, and stopping
// halfway would leave the host running neither set.
func preStopConfigExperimentDatadogAgent(ctx HookContext) error {
	ctx.Context = context.WithoutCancel(ctx.Context)
	return configExperiment{jobs: agentJobSet()}.Stop(ctx)
}

// postPromoteConfigExperimentDatadogAgent hands the Agent back to the stable job set, which now
// reads the promoted configuration.
func postPromoteConfigExperimentDatadogAgent(ctx HookContext) error {
	ctx.Context = context.WithoutCancel(ctx.Context)
	return configExperiment{jobs: agentJobSet()}.Promote(ctx)
}
