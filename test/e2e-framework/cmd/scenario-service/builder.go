// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Builder produces a scenariorun binary for a given commit.
type Builder interface {
	Build(commit string) (binPath string, err error)
}

// gitBuilder checks out a commit into a worktree and builds ./cmd/scenariorun,
// caching the resulting binary by commit. Repo root and cache dir are configurable.
type gitBuilder struct {
	repoRoot string
	cacheDir string
	mu       sync.Mutex
	cache    map[string]string
}

func newGitBuilder(repoRoot, cacheDir string) *gitBuilder {
	return &gitBuilder{repoRoot: repoRoot, cacheDir: cacheDir, cache: map[string]string{}}
}

func (b *gitBuilder) Build(commit string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if p, ok := b.cache[commit]; ok {
		return p, nil
	}
	wt := filepath.Join(b.cacheDir, "wt-"+commit)
	if err := runCmd(b.repoRoot, "git", "worktree", "add", "--detach", wt, commit); err != nil {
		return "", fmt.Errorf("git worktree add %s: %w", commit, err)
	}
	bin := filepath.Join(b.cacheDir, "scenariorun-"+commit)
	mod := filepath.Join(wt, "test", "e2e-framework")
	if err := runCmd(mod, "go", "build", "-o", bin, "./cmd/scenariorun"); err != nil {
		return "", fmt.Errorf("build scenariorun @ %s: %w", commit, err)
	}
	b.cache[commit] = bin
	return bin, nil
}

func runCmd(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Driver drives a per-commit scenariorun binary via its stable CLI protocol.
type Driver struct{ Builder Builder }

func (d Driver) exec(commit string, args ...string) ([]byte, error) {
	bin, err := d.Builder.Build(commit)
	if err != nil {
		return nil, err
	}
	return exec.Command(bin, args...).Output() //nolint:gosec
}

// Describe returns the per-commit binary's describe --json output.
func (d Driver) Describe(commit string) ([]byte, error) {
	return d.exec(commit, "describe", "--json")
}

// Run provisions a scenario at a commit with the given config + stack name.
func (d Driver) Run(commit, scenarioName, stack string, cfg map[string]string) error {
	args := []string{"create", scenarioName, "--stack", stack}
	for k, v := range cfg {
		args = append(args, "--"+k, v)
	}
	_, err := d.exec(commit, args...)
	return err
}

// Action runs a named action on a running stack.
func (d Driver) Action(commit, scenarioName, action, stack string, cfg map[string]string) error {
	args := []string{"action", scenarioName, action, "--stack", stack}
	for k, v := range cfg {
		args = append(args, "--"+k, v)
	}
	_, err := d.exec(commit, args...)
	return err
}

// Destroy tears down a running stack.
func (d Driver) Destroy(commit, scenarioName, stack string) error {
	_, err := d.exec(commit, "destroy", scenarioName, "--stack", stack)
	return err
}
