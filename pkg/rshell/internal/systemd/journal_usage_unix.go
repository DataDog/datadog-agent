// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin

package systemd

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// JournalDiskUsage reports allocated blocks, matching journalctl's disk-usage
// semantics more closely than summing logical file lengths.
func (c *Client) JournalDiskUsage(ctx context.Context) (builtins.JournalUsage, error) {
	_, files, err := c.journalAllocationFiles()
	if err != nil {
		return builtins.JournalUsage{}, err
	}
	usage := builtins.JournalUsage{Files: len(files)}
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return builtins.JournalUsage{}, err
		}
		info, err := c.lstatTargetPath(path)
		if err != nil {
			return builtins.JournalUsage{}, fmt.Errorf("inspect journal file %q: %w", path, err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return builtins.JournalUsage{}, fmt.Errorf("journal file %q changed during usage scan", path)
		}
		stat, err := journalStat(info)
		if err != nil {
			return builtins.JournalUsage{}, fmt.Errorf("journal file %q: %w", path, err)
		}
		if usage.Bytes > math.MaxUint64-stat.allocated {
			return builtins.JournalUsage{}, errors.New("journal allocation total overflow")
		}
		usage.Bytes += stat.allocated
	}
	return usage, nil
}

// journalAllocationFiles returns the machine ID and the paths of every file
// that counts toward reported journal disk usage, matching the file set
// collectVacuumCandidates sums allocated bytes over: readable ".journal"
// files plus recognized ".journal~" corruption archives.
func (c *Client) journalAllocationFiles() (string, []string, error) {
	return c.journalFilesFiltered(true)
}
