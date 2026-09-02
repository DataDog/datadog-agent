// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/installinfo"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var datadogAgentPackage = hooks{
	preInstall:  preInstallDatadogAgent,
	postInstall: postInstallDatadogAgent,
	preRemove:   preRemoveDatadogAgent,

	postStartConfigExperiment:   postStartConfigExperimentDatadogAgent,
	preStopConfigExperiment:     preStopConfigExperimentDatadogAgent,
	postPromoteConfigExperiment: postPromoteConfigExperimentDatadogAgent,
}

const (
	// agentUser and agentGroup are the account the Agent's launchd jobs run as. macOS ships
	// the daemon group on every system, so only the account is created.
	agentUser  = "_dd-agent"
	agentGroup = "daemon"

	// convenienceLinkDir is where the user-facing commands live. /usr/local/bin rather than
	// /usr/bin: /usr/bin is on the read-only system volume and cannot be written to.
	convenienceLinkDir = "/usr/local/bin"
)

// agentLayout is the on-disk layout the hooks create.
//
// It is parameterised on its root and its owner so the tests can assert the shape against a
// temporary root as an unprivileged user. Production always uses defaultAgentLayout.
type agentLayout struct {
	// installRoot is the single root everything the Agent owns lives under: the binaries
	// alongside etc, etc-exp, run and logs. Created once, preserved across every upgrade.
	installRoot string
	// linkDir is where the convenience commands are linked from.
	linkDir string

	owner string
	group string
}

var defaultAgentLayout = agentLayout{
	installRoot: filepath.Dir(paths.AgentConfigDir),
	linkDir:     convenienceLinkDir,
	owner:       agentUser,
	group:       agentGroup,
}

func (l agentLayout) etcDir() string    { return filepath.Join(l.installRoot, "etc") }
func (l agentLayout) etcExpDir() string { return filepath.Join(l.installRoot, "etc-exp") }
func (l agentLayout) runDir() string    { return filepath.Join(l.installRoot, "run") }
func (l agentLayout) logDir() string    { return filepath.Join(l.installRoot, "logs") }

// directories are the state directories the Agent needs to function.
//
// The install root itself is created without a recursive ownership pass: a recursive pass over it
// would traverse etc-exp, which rests as a symlink to etc, and so would write through to the
// stable configuration. etc-exp is the configuration layer's alone.
func (l agentLayout) directories() file.Directories {
	return file.Directories{
		{Path: l.installRoot, Mode: 0755, Owner: l.owner, Group: l.group},
		{Path: l.etcDir(), Mode: 0755, Owner: l.owner, Group: l.group},
		{Path: filepath.Join(l.etcDir(), "managed"), Mode: 0755, Owner: l.owner, Group: l.group},
		{Path: l.runDir(), Mode: 0755, Owner: l.owner, Group: l.group},
		{Path: filepath.Join(l.runDir(), "ipc"), Mode: 0755, Owner: l.owner, Group: l.group},
		{Path: l.logDir(), Mode: 0750, Owner: l.owner, Group: l.group},
	}
}

// configPermissions are the ownerships enforced on the configuration directory.
//
// Every entry is rooted at etc, never at the install root, so no recursive pass can reach etc-exp.
func (l agentLayout) configPermissions() file.Permissions {
	return file.Permissions{
		{Path: ".", Owner: l.owner, Group: l.group, Recursive: true},
		{Path: "managed", Owner: l.owner, Group: l.group, Recursive: true},
	}
}

// convenienceLinks are the user-facing commands. They name the install root, which is the same
// address for the life of the machine, so they never need updating.
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

// launchdClient and ensureAgentUser are indirected so the hook tests can assert the filesystem
// layout without launchd and without touching the machine's directory service.
var (
	launchdClient   = func() *launchd.Client { return launchd.NewClient(launchd.System) }
	ensureAgentUser = user.EnsureAgentUserAndGroup
	launchdJobDir   = launchd.System.Dir()
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

// installFilesystem creates the state directories and the convenience links.
//
// Everything in it is idempotent: it is the hook both install paths run, and it runs again on
// every upgrade. It never touches etc-exp, which the configuration layer owns alone.
func installFilesystem(ctx HookContext, layout agentLayout) (err error) {
	span, ctx := ctx.StartSpan("setup_filesystem")
	defer func() {
		span.Finish(err)
	}()

	// 1. Ensure the service account exists. The group already does.
	if err = ensureAgentUser(ctx, layout.installRoot); err != nil {
		return fmt.Errorf("failed to create %s user: %w", layout.owner, err)
	}

	// 2. Create the state directories.
	if err = layout.directories().Ensure(ctx); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// 3. Enforce configuration ownership. Rooted at etc, so etc-exp is never traversed.
	if err = layout.configPermissions().Ensure(ctx, layout.etcDir()); err != nil {
		return fmt.Errorf("failed to set config ownerships: %w", err)
	}

	// 4. Link the user-facing commands.
	for link, target := range layout.convenienceLinks() {
		if err = os.MkdirAll(filepath.Dir(link), 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", filepath.Dir(link), err)
		}
		if err = file.EnsureSymlink(ctx, target, link); err != nil {
			return fmt.Errorf("failed to create convenience link %s: %w", link, err)
		}
	}
	return nil
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
	if err := client.Bootstrap(ctx, job); err != nil {
		return fmt.Errorf("failed to load %s: %w", label, err)
	}
	if err := client.Enable(ctx, label); err != nil {
		return fmt.Errorf("failed to enable %s: %w", label, err)
	}
	if err := client.Kickstart(ctx, label, false); err != nil {
		return fmt.Errorf("failed to start %s: %w", label, err)
	}
	return nil
}

// InstallDaemonJob writes and loads the installer daemon's launchd job.
//
// It is the .dmg's entry point: the package scripts own the Agent's own jobs, but the daemon's
// definition is the one both install paths must agree on, so it comes from the embedded copy
// rather than from a plist the shell writes. The job exits cleanly when remote_updates is off,
// and launchd does not relaunch a clean exit, so loading it unconditionally is safe.
func InstallDaemonJob(ctx context.Context) error {
	return loadStableJob(ctx, launchdClient(), installerJob)
}

// RemoveDaemonJob unloads the installer daemon and removes its definition. Every step is allowed
// to fail: it runs on the uninstall and upgrade paths, where a job that is already gone is the
// desired outcome.
func RemoveDaemonJob(ctx context.Context) {
	removeJob(ctx, launchdClient(), installerJob)
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

// postInstallDatadogAgent creates the state directories and loads the stable job set.
func postInstallDatadogAgent(ctx HookContext) error {
	if err := installFilesystem(ctx, defaultAgentLayout); err != nil {
		return err
	}
	if err := installinfo.WriteInstallInfo(ctx, string(ctx.PackageType)); err != nil {
		return fmt.Errorf("failed to write install info: %w", err)
	}
	return installStableJobs(ctx)
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

// postStartConfigExperimentDatadogAgent hands the Agent over to the experiment job set. The
// installer has already published the experiment configuration directory by the time it runs.
func postStartConfigExperimentDatadogAgent(ctx HookContext) error {
	return configExperiment{jobs: agentJobSet()}.Start(ctx)
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
