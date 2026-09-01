// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package daemon

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/experiment"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// agentPackage is the only package that has a version experiment on macOS.
const agentPackage = "datadog-agent"

// newExperimentSupervisor builds the macOS supervisor.
//
// The revert it is given is the daemon's ordinary stop-experiment path, not a shortcut: an
// automatic revert and an operator-requested one have to leave the host in the same state, and the
// only way to guarantee that is for them to be the same code. That path runs the pre/post stop
// hooks, which is also what clears the deadline — the supervisor clears it too, and the two are
// idempotent.
func newExperimentSupervisor(d *daemonImpl) experimentSupervisor {
	labels, err := embedded.LaunchdJobs(embedded.LaunchdExperiment)
	if err != nil {
		// Without the label list there is nothing to watch. The deadline still works, so an
		// experiment is still bounded; only the fast path -- noticing an exit -- is lost.
		log.Errorf("Daemon: could not read the experiment job labels, an experiment will only be reverted at its deadline: %v", err)
	}
	expLabels := make([]string, 0, len(labels))
	for _, label := range labels {
		expLabels = append(expLabels, label+string(embedded.LaunchdExperiment))
	}

	// A watcher that cannot be created is not fatal for the same reason: the deadline is the
	// backstop and it lives on disk, not in this process.
	var watcher experiment.ProcWatcher
	if w, err := experiment.NewKqueueWatcher(); err != nil {
		log.Errorf("Daemon: could not watch the experiment jobs, an experiment will only be reverted at its deadline: %v", err)
	} else {
		watcher = w
	}

	revert := func(ctx context.Context, version string, reason string) error {
		log.Warnf("Daemon: reverting the %s experiment: %s", version, reason)
		// StopExperiment, not stopExperiment: the supervisor is ticked without the daemon
		// mutex held, precisely so that the revert can take it the ordinary way.
		if err := d.StopExperiment(ctx, agentPackage); err != nil {
			return fmt.Errorf("could not revert the %s experiment: %w", version, err)
		}
		return nil
	}
	return experiment.NewSupervisor(experiment.NewDeadline(), watcher, expLabels, revert)
}
