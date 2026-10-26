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
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
)

var datadogAgentPackage = hooks{
	postInstall: postInstallDatadogAgent,
}

// agentJob is the core Agent's launchd job.
const agentJob = "com.datadoghq.agent"

// agentJobs are the Agent's launchd jobs, in the order they are loaded.
var agentJobs = []string{
	agentJob,
	"com.datadoghq.sysprobe",
	"com.datadoghq.data-plane",
}

// procmgrJob is the process manager daemon.
const procmgrJob = "com.datadoghq.procmgr"

// stableJobs are the launchd jobs that run normally, in the order they are loaded.
var stableJobs = append([]string{procmgrJob}, agentJobs...)

// launchdClient and launchdJobDir are indirected so the hook tests can run without launchd.
var (
	launchdClient = func() *launchd.Client { return launchd.NewClient(launchd.System) }
	launchdJobDir = launchd.System.Dir()
)

// stableJob returns the job definition record for a label in the system domain.
func stableJob(label string) launchd.Job {
	return launchd.Job{
		Label:     label,
		PlistPath: filepath.Join(launchdJobDir, label+".plist"),
		Domain:    launchd.System,
	}
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
	for _, label := range stableJobs {
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
	job := stableJob(label)
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
// `installer postinst datadog-agent dmg`: loading the stable job set.
//
// The script creates the account and the directories, with their ownership, modes and access
// control lists, and writes the install info itself, so none of that happens here.
func postInstallDMG(ctx HookContext) error {
	return installStableJobs(ctx)
}
