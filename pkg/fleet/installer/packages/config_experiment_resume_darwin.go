// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// resumeConfigExperimentDatadogAgent is run once when the daemon starts, to recover a
// configuration experiment left running unsupervised because the detached watcher process
// (config_experiment_watcher_darwin.go) that was tracking it died along with everything else in
// an abrupt shutdown -- a crash, a kill, a reboot -- before it could revert or hand off.
//
// The deadline file is the only durable signal of this, since the watcher is a bare process with
// no launchd job and nothing else re-spawns it: a present, unexpired deadline means an experiment
// was mid-flight with nothing currently watching it, so the experiment job set and its watcher
// are re-established here. A present, expired deadline means the unsupervised window was already
// exceeded before the daemon got a chance to resume it, so the experiment is reverted instead.
// No deadline at all, but an experiment configuration directory still deployed, is reverted too
// (see revertOrphanedExperiment).
func resumeConfigExperimentDatadogAgent(ctx HookContext) error {
	deadline := launchd.Deadline{Path: configExperimentDeadlinePath}
	present, err := deadline.Present()
	if err != nil {
		return fmt.Errorf("resume: could not check experiment deadline presence: %w", err)
	}
	if !present {
		return revertOrphanedExperiment(ctx)
	}

	expired, err := deadline.Expired(configExperimentDeadlineWindow)
	if err != nil {
		return fmt.Errorf("resume: could not check experiment deadline: %w", err)
	}
	if expired {
		log.Warnf("resume: configuration experiment deadline already expired before the daemon could resume it, reverting")
		_, err := revertExperimentIfStillPending(ctx, deadline, "experiment deadline expired before the daemon could resume it")
		return err
	}

	log.Infof("resume: configuration experiment still within its deadline, resuming supervision")
	if err := (configExperiment{jobs: agentJobSet()}).Resume(ctx); err != nil {
		log.Errorf("resume: could not resume the configuration experiment job set, reverting: %v", err)
		if _, revertErr := revertExperimentIfStillPending(ctx, deadline, "experiment could not be resumed"); revertErr != nil {
			return fmt.Errorf("resume failed (%w) and the experiment could not be reverted: %w", err, revertErr)
		}
		return fmt.Errorf("resume failed, experiment reverted: %w", err)
	}
	if err := launchConfigExperimentWatcher(ctx); err != nil {
		log.Errorf("resume: could not relaunch the configuration experiment watcher, reverting: %v", err)
		if revertErr := (configExperiment{jobs: agentJobSet()}).Stop(context.WithoutCancel(ctx)); revertErr != nil {
			return fmt.Errorf("watcher failed to relaunch (%w) and the experiment could not be reverted: %w", err, revertErr)
		}
		return fmt.Errorf("watcher failed to relaunch, experiment reverted: %w", err)
	}
	return nil
}

// revertOrphanedExperiment reverts an experiment configuration directory that is deployed with no
// deadline beside it. Start writes the deadline before touching any job, and restoreStable clears
// it before anything else, so this is a host whose previous daemon died either after the installer
// published the directory but before the experiment job set ever started, or partway through a
// deliberate stop. Either way nothing will ever resume the experiment, yet the directory alone
// would have the daemon report it as the running configuration indefinitely.
func revertOrphanedExperiment(ctx HookContext) error {
	state, err := configExperimentDirs.GetState()
	if err != nil {
		return fmt.Errorf("resume: could not read the configuration state: %w", err)
	}
	if state.ExperimentDeploymentID == "" {
		return nil
	}
	log.Warnf("resume: configuration experiment %s is deployed with no deadline, reverting", state.ExperimentDeploymentID)
	if err := revertExperiment(ctx); err != nil {
		return fmt.Errorf("resume: %w", err)
	}
	return nil
}
