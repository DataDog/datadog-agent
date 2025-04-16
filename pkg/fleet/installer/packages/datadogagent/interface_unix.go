// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package datadogagent

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/packagemanager"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PreInstall performs pre-installation steps for the agent
func PreInstall(ctx context.Context) error {
	span, ctx := telemetry.StartSpanFromContext(ctx, "pre_install_agent")
	defer func() {
		span.Finish(nil)
	}()

	if err := stopAndRemoveAgentUnits(ctx, false, agentUnit); err != nil {
		log.Warnf("failed to stop and remove agent units: %s", err)
	}

	return packagemanager.RemovePackage(ctx, agentPackage)
}

// PostInstall performs post-installation steps for the agent
func PostInstall(ctx context.Context, installPath string, caller string, _ ...string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "post_install_agent")
	defer func() {
		span.Finish(err)
	}()

	if err := setupFilesystem(ctx, installPath, caller); err != nil {
		return err
	}
	if err := restoreCustomIntegrations(ctx, installPath); err != nil {
		log.Warnf("failed to restore custom integrations: %s", err)
	}
	if caller == installerCaller {
		if err := setupAndStartAgentUnits(ctx, stableUnits, agentUnit); err != nil {
			return err
		}
	}
	return nil
}

// PreRemove performs pre-removal steps for the agent
// All the steps are allowed to fail
func PreRemove(ctx context.Context, installPath string, caller string, upgrade bool) error {
	span, ctx := telemetry.StartSpanFromContext(ctx, "pre_remove_agent")
	defer func() {
		span.Finish(nil)
	}()
	if caller == installerCaller {
		if err := stopAndRemoveAgentUnits(ctx, true, agentUnit); err != nil {
			log.Warnf("failed to stop and remove experiment agent units: %s", err)
		}
	}

	if err := stopAndRemoveAgentUnits(ctx, false, agentUnit); err != nil {
		log.Warnf("failed to stop and remove agent units: %s", err)
	}

	if upgrade {
		if err := saveCustomIntegrations(ctx, installPath); err != nil {
			log.Warnf("failed to save custom integrations: %s", err)
		}
	}

	if err := removeCustomIntegrations(ctx, installPath); err != nil {
		log.Warnf("failed to remove custom integrations: %s\n", err.Error())
	}

	// Delete all the .pyc files. This MUST be done after using pip or any python, because executing python might generate .pyc files
	removeCompiledPythonFiles(installPath)

	if !upgrade {
		// Remove files not tracked by the package manager
		removeFilesystem(ctx, installPath)
	}

	return nil
}

// PreStartExperiment performs pre-start steps for the experiment.
// It must be executed by the stable unit before starting the experiment & before PostStartExperiment.
func PreStartExperiment(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "pre_start_experiment")
	defer func() {
		span.Finish(err)
	}()

	if err = saveCustomIntegrations(ctx, StablePath); err != nil {
		log.Warnf("failed to save custom integrations: %s", err)
	}

	return nil
}

// PostStartExperiment performs post-start steps for the experiment.
// It must be executed by the experiment unit before starting the experiment & after PreStartExperiment.
func PostStartExperiment(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "post_start_experiment")
	defer func() {
		span.Finish(err)
	}()

	if err := setupFilesystem(ctx, ExperimentPath, installerCaller); err != nil {
		return err
	}

	if err := restoreCustomIntegrations(ctx, ExperimentPath); err != nil {
		log.Warnf("failed to restore custom integrations: %s", err)
	}

	return setupAndStartAgentUnits(ctx, expUnits, agentExpUnit)
}

// PreStopExperiment performs pre-stop steps for the experiment.
// It must be executed by the experiment unit before stopping the experiment & before PostStopExperiment.
func PreStopExperiment(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "pre_stop_experiment")
	defer func() {
		span.Finish(err)
	}()

	ctx = context.WithoutCancel(ctx)
	return stopAndRemoveAgentUnits(ctx, true, agentExpUnit) // This restarts stable units
}

// PostStopExperiment performs post-stop steps for the experiment.
// It must be executed by the stable unit before stopping the experiment & after PreStopExperiment.
func PostStopExperiment(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "post_stop_experiment")
	defer func() {
		span.Finish(err)
	}()

	// Nothing to do.

	return nil
}

// PrePromoteExperiment performs pre-promote steps for the experiment.
// It must be executed by the stable unit before promoting the experiment & before PostPromoteExperiment.
func PrePromoteExperiment(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "pre_promote_experiment")
	defer func() {
		span.Finish(err)
	}()
	return stopAndRemoveAgentUnits(ctx, false, agentUnit)
}

// PostPromoteExperiment performs post-promote steps for the experiment.
// It must be executed by the experiment unit (now the new stable) before promoting the experiment & after PrePromoteExperiment.
func PostPromoteExperiment(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "post_promote_experiment")
	defer func() {
		span.Finish(err)
	}()

	if err := setupFilesystem(ctx, StablePath, installerCaller); err != nil {
		return err
	}

	ctx = context.WithoutCancel(ctx)
	if err := setupAndStartAgentUnits(ctx, stableUnits, agentUnit); err != nil {
		return err
	}
	return stopAndRemoveAgentUnits(ctx, true, agentExpUnit)
}
