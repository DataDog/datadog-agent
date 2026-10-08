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

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// dirSwap exchanges a live directory for one prepared beside it.
//
// A promote is a single renamex_np(RENAME_SWAP) in one parent on one filesystem: the live and
// incoming directories trade names atomically, so the live path names the old directory or the
// new one at every instant, including across a crash or a reboot -- there is no window in which it
// is missing. The directory left at the incoming path is then the previous live one, and is
// discarded. Failing to discard it is not fatal: the experiment path is put back to resting right
// after a promote, which clears whatever directory is still there.
type dirSwap struct {
	// live is the directory being replaced, e.g. /opt/datadog-agent/etc.
	live string
	// incoming is the directory that takes its place. It must be a sibling of live.
	incoming string
}

// renameSwap is indirected so a test can force the exchange to fail.
var renameSwap = func(from, to string) error {
	return unix.RenamexNp(from, to, unix.RENAME_SWAP)
}

// Commit performs the swap. On success incoming no longer exists: it *is* live.
func (s dirSwap) Commit(_ context.Context) error {
	parent := filepath.Dir(s.live)
	if filepath.Dir(s.incoming) != parent {
		return fmt.Errorf("%s and %s are not in the same directory, so they cannot be swapped by rename", s.live, s.incoming)
	}
	if _, err := os.Lstat(s.incoming); err != nil {
		return fmt.Errorf("could not inspect %s: %w", s.incoming, err)
	}

	if err := renameSwap(s.incoming, s.live); err != nil {
		return fmt.Errorf("could not swap %s into place at %s: %w", s.incoming, s.live, err)
	}
	// incoming now holds the previous live directory.
	if err := os.RemoveAll(s.incoming); err != nil {
		log.Warnf("could not discard the previous %s, left at %s: %v", s.live, s.incoming, err)
	}
	return nil
}
