// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// restingLink is the experiment configuration path together with the stable path it points at
// when no experiment is deployed.
//
// The link is the only representation of "no configuration experiment is deployed": resting as a
// symlink to the stable directory means nothing is deployed, standing as a real directory means
// something is. There is no flag kept alongside it that could disagree, so a crash at any point
// leaves the host in one of those two states rather than in a third one.
//
// It is the sole owner of the experiment path. Nothing else may create, replace or delete it —
// in particular no recursive ownership or permission pass may traverse it, because following the
// link would apply the pass to the stable directory a second time under the wrong root.
type restingLink struct {
	// path is the experiment configuration path, e.g. /opt/datadog-agent/etc-exp.
	path string
	// target is the stable configuration path the link rests on, e.g. /opt/datadog-agent/etc.
	target string
}

// IsResting reports whether no configuration experiment is deployed.
//
// An absent path counts as resting: there is nothing deployed, and Rest will create the link.
func (l restingLink) IsResting() (bool, error) {
	info, err := os.Lstat(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("could not inspect %s: %w", l.path, err)
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return true, nil
	case info.IsDir():
		return false, nil
	default:
		return false, fmt.Errorf("%s is neither a symlink nor a directory (mode %s)", l.path, info.Mode())
	}
}

// Rest discards whatever occupies the experiment path and puts the link back.
//
// It is idempotent, and it is the only way a deployed experiment is removed: a host that never
// started one and a host whose experiment was just discarded are indistinguishable afterwards.
func (l restingLink) Rest() error {
	if err := os.RemoveAll(l.path); err != nil {
		return fmt.Errorf("could not clear %s: %w", l.path, err)
	}
	if err := os.Symlink(l.target, l.path); err != nil {
		return fmt.Errorf("could not rest %s on %s: %w", l.path, l.target, err)
	}
	return nil
}

// Materialize publishes an already prepared directory at the experiment path.
//
// incoming must be a sibling of the experiment path so the publish is a rename: everything that
// can fail — the copy, the patches, the deployment ID — has already happened in the scratch
// directory, and this single step either takes effect whole or not at all. On failure the link is
// put back, so a failed experiment is indistinguishable from one that never started.
func (l restingLink) Materialize(incoming string) error {
	resting, err := l.IsResting()
	if err != nil {
		return err
	}
	if !resting {
		return fmt.Errorf("%s already holds a configuration experiment", l.path)
	}
	if filepath.Dir(incoming) != filepath.Dir(l.path) {
		return fmt.Errorf("%s is not a sibling of %s, so it cannot be published by rename", incoming, l.path)
	}
	if err := os.Remove(l.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not lift the resting link at %s: %w", l.path, err)
	}
	if err := os.Rename(incoming, l.path); err != nil {
		if restErr := l.Rest(); restErr != nil {
			return fmt.Errorf("could not publish %s: %w, and could not rest the link again: %w", incoming, err, restErr)
		}
		return fmt.Errorf("could not publish %s at %s: %w", incoming, l.path, err)
	}
	return nil
}
