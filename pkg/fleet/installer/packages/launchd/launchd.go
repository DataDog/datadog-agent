// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

// Package launchd wraps the launchctl(1) subset the installer needs to manage launchd jobs.
//
// Every operation is idempotent, because the hooks that call them run on both install paths and
// may run again on a host that is already in the desired state: bootstrapping a loaded job,
// booting out an absent one and removing a definition that is not there all succeed.
package launchd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// Domain is a launchd domain.
type Domain string

const (
	// System is the system domain: jobs run as root at boot, with no login session.
	System Domain = "system"
	// GUI is a per-user domain, for jobs that need a login session. Per-user jobs are out of
	// scope for Fleet, which never manages one; the domain exists so a job definition can name
	// where it belongs.
	GUI Domain = "gui"
)

// Dir returns the directory job definitions for the domain live in.
func (d Domain) Dir() string {
	if d == GUI {
		return "/Library/LaunchAgents"
	}
	return "/Library/LaunchDaemons"
}

// Job is a launchd job definition on disk.
type Job struct {
	// Label is the job's launchd label, e.g. com.datadoghq.agent.
	Label string
	// PlistPath is where the definition lives. Empty means the default path for the domain.
	PlistPath string
	// Domain is the domain the job belongs to.
	Domain Domain
}

// Path returns the path the job definition is written to.
func (j Job) Path() string {
	if j.PlistPath != "" {
		return j.PlistPath
	}
	return filepath.Join(j.Domain.Dir(), j.Label+".plist")
}

// Write writes the job definition to disk.
//
// The write goes to a temporary file in the same directory and is renamed into place, so launchd
// never reads a partially written property list.
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
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("could not close job definition: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("could not move job definition into place: %w", err)
	}
	return nil
}

// Remove removes the job definition from disk. It succeeds when the definition is already absent.
func (j Job) Remove() error {
	if err := os.Remove(j.Path()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not remove job definition: %w", err)
	}
	return nil
}

// JobStatus is what launchctl print reports about a job.
type JobStatus struct {
	// Label is the job's launchd label.
	Label string
	// PID is the running process's PID, or 0 when the job is loaded but not running.
	PID int
	// LastExitStatus is the exit code launchd last recorded for the job, or 0 when it has
	// never exited.
	LastExitStatus int
	// Loaded reports whether the job is present in the domain.
	Loaded bool
}

// Runner executes a command and returns its combined output.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Client runs launchctl against one domain.
type Client struct {
	// Domain is the domain every operation targets.
	Domain Domain
	// UID selects the session for the GUI domain. It is ignored for the system domain.
	UID int

	// Runner executes launchctl. Nil runs the real binary; tests substitute a recorder.
	Runner Runner
}

// NewClient returns a Client targeting the given domain.
func NewClient(domain Domain) *Client {
	return &Client{Domain: domain}
}

// Bootstrap loads a job definition into the domain. It succeeds when the job is already loaded.
func (c *Client) Bootstrap(ctx context.Context, job Job) error {
	out, err := c.launchctl(ctx, "bootstrap", c.domainTarget(), job.Path())
	if err == nil || isAlreadyLoaded(out) {
		return nil
	}
	return fmt.Errorf("could not bootstrap %s: %w (%s)", job.Label, err, strings.TrimSpace(string(out)))
}

// Bootout unloads a job from the domain. It succeeds when the job is not loaded.
func (c *Client) Bootout(ctx context.Context, label string) error {
	out, err := c.launchctl(ctx, "bootout", c.serviceTarget(label))
	if err == nil || isNotLoaded(out) {
		return nil
	}
	return fmt.Errorf("could not bootout %s: %w (%s)", label, err, strings.TrimSpace(string(out)))
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

// Print reports what launchd knows about a job. A job that is not loaded is reported with
// Loaded false and no error.
func (c *Client) Print(ctx context.Context, label string) (JobStatus, error) {
	out, err := c.launchctl(ctx, "print", c.serviceTarget(label))
	if err != nil {
		if isNotLoaded(out) {
			return JobStatus{Label: label}, nil
		}
		return JobStatus{Label: label}, fmt.Errorf("could not print %s: %w (%s)", label, err, strings.TrimSpace(string(out)))
	}
	status := parsePrint(label, string(out))
	return status, nil
}

// Loaded reports whether the job is present in the domain.
func (c *Client) Loaded(ctx context.Context, label string) (bool, error) {
	status, err := c.Print(ctx, label)
	if err != nil {
		return false, err
	}
	return status.Loaded, nil
}

func (c *Client) domainTarget() string {
	if c.Domain == GUI {
		return fmt.Sprintf("gui/%d", c.UID)
	}
	return string(System)
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

const launchctlPath = "/bin/launchctl"

var (
	pidRe       = regexp.MustCompile(`(?m)^\s*pid\s*=\s*(\d+)\s*$`)
	exitRe      = regexp.MustCompile(`(?m)^\s*last exit (?:code|status)\s*=\s*(-?\d+)\s*$`)
	notLoadedRe = regexp.MustCompile(`(?i)could not find service|no such (?:process|file or directory)|service not loaded`)
	loadedRe    = regexp.MustCompile(`(?i)service already loaded|already bootstrapped|operation already in progress|file exists`)
)

// parsePrint extracts the fields the installer needs from launchctl print output. launchd's
// output is a nested, unstable, undocumented dump, so only the individual lines that matter are
// matched; anything absent is left at its zero value rather than treated as an error.
func parsePrint(label string, out string) JobStatus {
	status := JobStatus{Label: label, Loaded: true}
	if m := pidRe.FindStringSubmatch(out); m != nil {
		if pid, err := strconv.Atoi(m[1]); err == nil {
			status.PID = pid
		}
	}
	if m := exitRe.FindStringSubmatch(out); m != nil {
		if code, err := strconv.Atoi(m[1]); err == nil {
			status.LastExitStatus = code
		}
	}
	return status
}

func isNotLoaded(out []byte) bool {
	return notLoadedRe.Match(out)
}

func isAlreadyLoaded(out []byte) bool {
	return loadedRe.Match(out)
}
