// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

// Package launchd wraps the launchctl(1) subset the installer needs to manage launchd jobs.
//
// Every operation is idempotent, because the hooks that call them run on both install paths and
// may run again on a host that is already in the desired state: bootstrapping a loaded job
// and booting out an absent one both succeed.
package launchd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Domain is a launchd domain.
type Domain string

// System is the system domain: jobs run at boot, with no login session.
const System Domain = "system"

// Dir returns the directory job definitions for the domain live in.
func (Domain) Dir() string {
	return "/Library/LaunchDaemons"
}

// Job is a launchd job definition on disk, at <Dir>/<Label>.plist.
type Job struct {
	// Label is the job's launchd label, e.g. com.datadoghq.agent.
	Label string
	// Dir is the directory the definition lives in. Empty means the system domain's directory.
	Dir string
}

// Path returns the path the job definition is written to.
func (j Job) Path() string {
	dir := j.Dir
	if dir == "" {
		dir = System.Dir()
	}
	return filepath.Join(dir, j.Label+".plist")
}

// Write writes the job definition to disk.
//
// The write goes to a temporary file in the same directory, is flushed to disk and is renamed into
// place, so neither launchd nor a power loss can leave a partially written or empty property list.
func (j Job) Write(content []byte) error {
	path := j.Path()
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err != nil {
		return fmt.Errorf("could not create temporary job definition: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("could not write job definition: %w", err)
	}
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return fmt.Errorf("could not set job definition mode: %w", err)
	}
	// A LaunchDaemon definition is expected to be owned by root:wheel. Ownership is asserted
	// explicitly here rather than left to whichever identity happened to invoke Write -- matching
	// the behavior the .pkg postinst previously enforced with an explicit chown. Failure is a
	// warning, not a hard error: Write also runs unprivileged in tests, where changing ownership
	// to root is never permitted.
	if err := tmp.Chown(0, 0); err != nil {
		log.Warnf("could not set root:wheel ownership on job definition %s: %v", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("could not flush job definition: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("could not close job definition: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("could not move job definition into place: %w", err)
	}
	return nil
}

// Runner executes a command and returns its combined output.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Client runs launchctl against one domain.
type Client struct {
	// Domain is the domain every operation targets.
	Domain Domain

	// Runner executes launchctl. Nil runs the real binary; tests substitute a recorder.
	Runner Runner

	// BootoutSettleTimeout bounds how long Bootout waits for a label to leave the domain after
	// launchctl bootout returns. Zero uses defaultBootoutSettleTimeout; tests override it to keep
	// the settle loop from adding wall-clock time to the suite.
	BootoutSettleTimeout time.Duration
	// BootoutSettlePollInterval is how often Bootout re-checks the label while waiting. Zero uses
	// defaultBootoutSettlePollInterval.
	BootoutSettlePollInterval time.Duration
}

// NewClient returns a Client targeting the given domain.
func NewClient(domain Domain) *Client {
	return &Client{Domain: domain}
}

const (
	// defaultBootoutSettleTimeout must be longer than the ExitTimeOut of the jobs the installer
	// manages (10 seconds): launchd waits that long for a job to stop before it kills it, so a
	// shorter wait would report a slow-stopping job as stuck. It matches the 30 seconds the .dmg
	// postinstall script's fallback waits for the same thing.
	defaultBootoutSettleTimeout      = 30 * time.Second
	defaultBootoutSettlePollInterval = 100 * time.Millisecond
)

func (c *Client) bootoutSettleTimeout() time.Duration {
	if c.BootoutSettleTimeout > 0 {
		return c.BootoutSettleTimeout
	}
	return defaultBootoutSettleTimeout
}

func (c *Client) bootoutSettlePollInterval() time.Duration {
	if c.BootoutSettlePollInterval > 0 {
		return c.BootoutSettlePollInterval
	}
	return defaultBootoutSettlePollInterval
}

// Bootstrap loads a job definition into the domain. It succeeds when the job is already loaded.
func (c *Client) Bootstrap(ctx context.Context, job Job) error {
	out, err := c.launchctl(ctx, "bootstrap", c.domainTarget(), job.Path())
	if err == nil {
		return nil
	}
	// launchctl's error output is free text that varies between macOS versions, so whether the
	// job ended up loaded is asked of the domain instead: a job it reports as loaded is a
	// successful, idempotent bootstrap.
	if loaded, loadedErr := c.Loaded(ctx, job.Label); loadedErr == nil && loaded {
		return nil
	}
	return fmt.Errorf("could not bootstrap %s: %w (%s)", job.Label, err, strings.TrimSpace(string(out)))
}

// Bootout unloads a job from the domain. It succeeds when the job is not loaded.
//
// launchd's bootout is asynchronous: launchctl can return before the label has actually left the
// domain, which used to let a following Bootstrap race it and fail with a bare
// "Input/output error". Bootout now polls Loaded until the label is gone, bounded by
// BootoutSettleTimeout, so a nil return is a real guarantee that the domain no longer has it.
func (c *Client) Bootout(ctx context.Context, label string) error {
	out, err := c.launchctl(ctx, "bootout", c.serviceTarget(label))
	if err != nil {
		// As in Bootstrap, the domain rather than the error text says whether the job is gone: a
		// job that is not loaded is a successful, idempotent bootout, and any other failure is
		// reported at once rather than after waiting for a job that is not going to leave.
		if loaded, loadedErr := c.Loaded(ctx, label); loadedErr == nil && !loaded {
			return nil
		}
		return fmt.Errorf("could not bootout %s: %w (%s)", label, err, strings.TrimSpace(string(out)))
	}
	return c.waitUntilUnloaded(ctx, label)
}

func (c *Client) waitUntilUnloaded(ctx context.Context, label string) error {
	timeout := c.bootoutSettleTimeout()
	settleCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	timedOut := fmt.Errorf("%s did not leave the domain within %s", label, timeout)
	for {
		loaded, err := c.Loaded(settleCtx, label)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fmt.Errorf("could not confirm %s left the domain: %w", label, ctxErr)
			}
			// The deadline killed a launchctl print that was still running: report the timeout,
			// not the signal that ended the command.
			if settleCtx.Err() != nil {
				return timedOut
			}
			return fmt.Errorf("could not confirm %s left the domain: %w", label, err)
		}
		if !loaded {
			return nil
		}
		select {
		case <-settleCtx.Done():
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fmt.Errorf("could not confirm %s left the domain: %w", label, ctxErr)
			}
			return timedOut
		case <-time.After(c.bootoutSettlePollInterval()):
		}
	}
}

// Kickstart starts a loaded job. With kill set, a running instance is terminated and restarted.
func (c *Client) Kickstart(ctx context.Context, label string, kill bool) error {
	args := []string{"kickstart"}
	if kill {
		args = append(args, "-k")
	}
	args = append(args, c.serviceTarget(label))
	out, err := c.launchctl(ctx, args...)
	if err != nil {
		return fmt.Errorf("could not kickstart %s: %w (%s)", label, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Enable clears a job's disabled override, so launchd starts it at boot.
//
// The override survives bootout and a rewritten definition, which is why enabling is a separate
// step from bootstrapping: a job disabled by an operator or by a previous uninstall would
// otherwise load and never run.
func (c *Client) Enable(ctx context.Context, label string) error {
	out, err := c.launchctl(ctx, "enable", c.serviceTarget(label))
	if err != nil {
		return fmt.Errorf("could not enable %s: %w (%s)", label, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Loaded reports whether the job is present in the domain.
func (c *Client) Loaded(ctx context.Context, label string) (bool, error) {
	out, err := c.launchctl(ctx, "print", c.serviceTarget(label))
	if err == nil {
		return true, nil
	}
	if code, ok := exitCode(err); ok && code == exitServiceNotFound {
		return false, nil
	}
	return false, fmt.Errorf("could not print %s: %w (%s)", label, err, strings.TrimSpace(string(out)))
}

func (c *Client) domainTarget() string {
	return string(c.Domain)
}

func (c *Client) serviceTarget(label string) string {
	return c.domainTarget() + "/" + label
}

func (c *Client) launchctl(ctx context.Context, args ...string) ([]byte, error) {
	if c.Runner != nil {
		return c.Runner(ctx, launchctlPath, args...)
	}
	return telemetry.CommandContext(ctx, launchctlPath, args...).CombinedOutput()
}

const (
	launchctlPath = "/bin/launchctl"
	// exitServiceNotFound is the status launchctl exits with when the service it is asked about
	// is not in the domain.
	exitServiceNotFound = 113
)

// exitCode returns the status a command exited with, when err carries one.
func exitCode(err error) (int, bool) {
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), true
	}
	return 0, false
}
