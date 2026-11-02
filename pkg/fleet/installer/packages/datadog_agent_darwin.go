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
	"slices"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
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
// The root installer daemon is loaded last, once the Agent it reports on is up. The .dmg's
// postinstall script leaves it out of its fallback: the Agent runs without it.
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
	definition, err := embedded.GetLaunchdJob(label)
	if err != nil {
		return fmt.Errorf("failed to read job definition %s: %w", label, err)
	}
	job := launchd.Job{Label: label, Dir: launchdJobDir}
	// Unload before rewriting so the new definition is the one launchd runs: launchd caches the
	// definition it loaded, and bootstrapping over a loaded job is a no-op. If the job can't be
	// unloaded, leave its file untouched and fail, so the postinstall fallback sees a definition
	// that differs from the shipped one and reloads it instead of skipping it.
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
	if err := client.Bootstrap(ctx, job); err != nil {
		return fmt.Errorf("failed to load %s: %w", label, err)
	}
	if err := client.Kickstart(ctx, label, false); err != nil {
		return fmt.Errorf("failed to start %s: %w", label, err)
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

// postInstallDMG is the part of the post-install the .dmg's postinstall script delegates to
// `installer postinst datadog-agent dmg`: registering the Agent as a package and loading the stable
// job set.
//
// The script creates the account and the directories, with their ownership, modes and access
// control lists, and writes the install info itself, so none of that happens here.
//
// Every step runs even if an earlier one fails, and in this order so the installer daemon finds its
// directories and the package registered when it starts: a failed registration only leaves Fleet
// without the Agent's version, and must not also leave the Agent stopped.
func postInstallDMG(ctx HookContext) error {
	var errs []error
	// The installer daemon keeps its state in its own root-owned run directory, never in the
	// Agent's. Create it, or repair it, before the daemon is loaded below.
	if err := ensureInstallerDataDir(); err != nil {
		errs = append(errs, err)
	}
	if err := registerPackageRepository(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := installStableJobs(ctx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// registerPackageRepository registers the Agent in the package repository the shared installer
// code keeps for every package.
//
// macOS installs the Agent from a .dmg rather than from an OCI package, so nothing else on this
// platform ever creates that repository -- but the installer daemon reads it to report the
// packages installed on the host, and with no entry it reports none.
//
// The version directory the links resolve to is a placeholder. macOS keeps no versioned package
// pool: the files the entry stands for live in the install root, and are replaced there by the
// next .dmg rather than installed alongside. What the shared code needs is a repository whose
// stable link resolves, which is what this creates.
//
// The version it is registered under is not a placeholder, though: it is what Fleet Automation
// reads as the host's stable version and health-checks the running Agent against, so it has to
// name the Agent actually installed -- see registeredVersion -- and has to move forward when a
// .dmg replaces that Agent.
func registerPackageRepository(ctx HookContext) (err error) {
	span, ctx := ctx.StartSpan("register_package_repository")
	defer func() {
		span.Finish(err)
	}()

	if err = os.MkdirAll(packagesRoot, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", packagesRoot, err)
	}
	repositories := repository.NewRepositories(packagesRoot, AsyncPreRemoveHooks)

	state, err := repositories.GetState(ctx.Package)
	if err != nil {
		return fmt.Errorf("failed to read the %s package state: %w", ctx.Package, err)
	}
	if state.HasStable() {
		isPlaceholder, err := stableIsPlaceholder(repositories, ctx.Package)
		if err != nil {
			return err
		}
		// A real package is left alone: Repository.Create removes whatever it finds before
		// writing, so re-registering would discard it.
		if !isPlaceholder {
			return nil
		}
		// A placeholder registered on an earlier .dmg install. The .dmg replaces the Agent in the
		// install root without touching this entry, so after an upgrade it still names the version
		// installed first. Register it again under the current version.
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

// stableIsPlaceholder reports whether the package's stable link resolves to a placeholder entry --
// the empty directory registerPackageRepository moves in -- rather than to a real package.
func stableIsPlaceholder(repositories *repository.Repositories, pkg string) (bool, error) {
	stablePath := repositories.Get(pkg).StablePath()
	entries, err := os.ReadDir(stablePath)
	if err != nil {
		return false, fmt.Errorf("failed to read the %s stable package directory %s: %w", pkg, stablePath, err)
	}
	return len(entries) == 0, nil
}

// preRemoveDatadogAgent unloads the stable job set and removes its definitions, in the reverse of
// the order they are loaded in. The .dmg's preinstall script runs it through the installed
// version's `installer prerm --upgrade datadog-agent dmg`, before the new payload replaces it, so
// the version being replaced takes down exactly the jobs it installed -- including any the next
// version no longer knows about, whose definitions launchd would otherwise load again at boot.
//
// The definitions are removed on an upgrade too: both install paths write every definition from
// scratch, so nothing is lost. Neither the package type nor ctx.Upgrade changes anything: tearing
// down the jobs is the same whichever way the Agent was installed and whatever replaces it.
//
// Every step is allowed to fail: a job that is already gone is the desired outcome, and an error
// here would only stop the next version from installing.
func preRemoveDatadogAgent(ctx HookContext) error {
	client := launchdClient()
	for _, label := range slices.Backward(agentLaunchdJobs.LaunchdJobsStable) {
		if err := client.Bootout(ctx, label); err != nil {
			// As in loadStableJob, a job that can't be unloaded keeps its definition, so launchd is
			// never left running a job whose file is gone, and the postinstall script reloads it.
			log.Warnf("failed to unload %s: %v", label, err)
			continue
		}
		if err := os.Remove(launchd.Job{Label: label, Dir: launchdJobDir}.Path()); err != nil && !os.IsNotExist(err) {
			log.Warnf("failed to remove job definition %s: %v", label, err)
		}
	}
	return nil
}
