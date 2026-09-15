// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// dirSwap exchanges a live directory for one prepared beside it.
//
// A promote is two renames in one parent on one filesystem: the live directory is moved aside,
// the incoming one is moved onto its name, and the one set aside is discarded. Rename is atomic
// within a filesystem, so a reader either sees the old directory or the new one, never a merge of
// the two and never a missing path for longer than the second rename takes. If the second rename
// fails the first is undone, which is why the directory is moved aside rather than deleted.
type dirSwap struct {
	// live is the directory being replaced, e.g. /opt/datadog-agent/etc.
	live string
	// incoming is the directory that takes its place. It must be a sibling of live.
	incoming string
}

// rename is indirected so a test can force the second rename to fail and assert the rollback.
var rename = os.Rename

// Commit performs the swap. On success incoming no longer exists: it *is* live.
func (s dirSwap) Commit(_ context.Context) (err error) {
	parent := filepath.Dir(s.live)
	if filepath.Dir(s.incoming) != parent {
		return fmt.Errorf("%s and %s are not in the same directory, so they cannot be swapped by rename", s.live, s.incoming)
	}
	if _, err := os.Lstat(s.incoming); err != nil {
		return fmt.Errorf("could not inspect %s: %w", s.incoming, err)
	}

	asideDir, err := os.MkdirTemp(parent, ".datadog-config-aside")
	if err != nil {
		return fmt.Errorf("could not create the directory to move %s aside: %w", s.live, err)
	}
	defer os.RemoveAll(asideDir)
	aside := filepath.Join(asideDir, filepath.Base(s.live))

	if err := rename(s.live, aside); err != nil {
		return fmt.Errorf("could not move %s aside: %w", s.live, err)
	}
	defer func() {
		if err != nil {
			if rollbackErr := os.Rename(aside, s.live); rollbackErr != nil {
				err = fmt.Errorf("%w, and %s could not be restored: %w", err, s.live, rollbackErr)
			}
		}
	}()
	if err := rename(s.incoming, s.live); err != nil {
		return fmt.Errorf("could not move %s into place at %s: %w", s.incoming, s.live, err)
	}
	return nil
}
