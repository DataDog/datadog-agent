// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package launchd

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
)

// Variant selects one of the two job sets a label is defined for.
type Variant = embedded.LaunchdVariant

const (
	// Stable is the job set that runs normally: supervised by launchd, reaching the binaries
	// through the façade, reading the stable configuration directory.
	Stable = embedded.LaunchdStable
	// Experiment is the job set an experiment runs under: unsupervised, reaching directly into
	// the versioned pool, reading the experiment configuration directory.
	Experiment = embedded.LaunchdExperiment
)

// JobSet is a group of launchd jobs that are loaded and unloaded together, in either variant.
//
// The two variants are two separate job sets, not one job restarted with different arguments.
// Everything that distinguishes an experiment — which configuration directory it reads, which
// copy of the binary it runs, and above all that launchd does not respawn it — is baked into the
// job definition, so an experiment that exits is terminal rather than one iteration of a respawn
// loop, and the stable definition stays untouched on disk the whole time an experiment runs.
type JobSet struct {
	// Labels are the job labels without their variant suffix, in the order they are loaded.
	Labels []string
	// Dir is the directory job definitions are written to. Empty means the client domain's
	// default directory.
	Dir string
	// Client runs launchctl for this set.
	Client *Client
}

// Job returns the job record for one label in the given variant.
func (s JobSet) Job(label string, variant Variant) Job {
	job := Job{Label: label + string(variant), Domain: s.Client.Domain}
	if s.Dir != "" {
		job.PlistPath = s.Dir + "/" + job.Label + ".plist"
	}
	return job
}

// Write writes every job definition in the set, in the given variant, to disk. It does not load
// them.
func (s JobSet) Write(variant Variant) error {
	for _, label := range s.Labels {
		definition, err := embedded.GetLaunchdJob(label, variant)
		if err != nil {
			return fmt.Errorf("could not read the job definition for %s: %w", label+string(variant), err)
		}
		if err := s.Job(label, variant).Write(definition); err != nil {
			return err
		}
	}
	return nil
}

// Remove removes every job definition in the set, in the given variant, from disk. It succeeds
// when a definition is already absent.
func (s JobSet) Remove(variant Variant) error {
	for _, label := range s.Labels {
		if err := s.Job(label, variant).Remove(); err != nil {
			return err
		}
	}
	return nil
}

// Start loads and starts every job in the set, in the given variant, in order.
//
// Enable is a separate step from Bootstrap because launchd's disabled override survives both a
// bootout and a rewritten definition: a job disabled by a previous uninstall would otherwise load
// and never run.
func (s JobSet) Start(ctx context.Context, variant Variant) error {
	for _, label := range s.Labels {
		job := s.Job(label, variant)
		// Bootout first so the definition on disk is the one launchd runs: launchd caches the
		// definition it loaded, and bootstrapping over a loaded job is a no-op.
		if err := s.Client.Bootout(ctx, job.Label); err != nil {
			return err
		}
		if err := s.Client.Bootstrap(ctx, job); err != nil {
			return err
		}
		if err := s.Client.Enable(ctx, job.Label); err != nil {
			return err
		}
		if err := s.Client.Kickstart(ctx, job.Label, false); err != nil {
			return err
		}
	}
	return nil
}

// Stop unloads every job in the set, in the given variant, in reverse order.
//
// The jobs are unloaded in reverse so a job is never left running against a dependency that has
// already gone away.
func (s JobSet) Stop(ctx context.Context, variant Variant) error {
	for i := len(s.Labels) - 1; i >= 0; i-- {
		if err := s.Client.Bootout(ctx, s.Labels[i]+string(variant)); err != nil {
			return err
		}
	}
	return nil
}
