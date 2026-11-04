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
	"io"
	"io/fs"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

type vacuumDirectory struct {
	path string
	root *os.Root
}

type vacuumCandidate struct {
	directory *vacuumDirectory
	name      string
	modTime   time.Time
	size      int64
	stat      journalFileStat
}

// VacuumJournal deletes only strictly recognized archived journals from the
// configured target while honoring the requested size and time thresholds.
func (c *Client) VacuumJournal(ctx context.Context, request builtins.JournalVacuumRequest) (builtins.JournalVacuumResult, error) {
	if request.Now.IsZero() {
		return builtins.JournalVacuumResult{}, errors.New("journal vacuum reference time is required")
	}
	if request.MaxBytes == 0 && request.Before.IsZero() {
		return builtins.JournalVacuumResult{}, errors.New("journal vacuum requires a size or time limit")
	}
	if request.MaxBytes > 0 && request.Before.IsZero() {
		return builtins.JournalVacuumResult{}, errors.New("journal size vacuum requires a time cutoff")
	}
	if !request.Before.IsZero() && request.Before.After(request.Now) {
		return builtins.JournalVacuumResult{}, errors.New("journal vacuum time cutoff cannot be in the future")
	}

	directories, err := c.openVacuumDirectories()
	if err != nil {
		return builtins.JournalVacuumResult{}, err
	}
	defer closeVacuumDirectories(directories)

	candidates, allocatedBytes, err := collectVacuumCandidates(directories)
	if err != nil {
		return builtins.JournalVacuumResult{}, err
	}
	// The size target applies to the total allocated journal bytes (active plus
	// archived), matching what JournalDiskUsage reports and what systemd's own
	// --vacuum-size caps. Only archived candidates at or before the time cutoff
	// are ever deleted, so a target below the active journals' own footprint is
	// simply not reachable.
	remainingBytes := allocatedBytes
	result := builtins.JournalVacuumResult{RemainingBytes: remainingBytes}

	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return result, vacuumPartialError(result, err)
		}
		oldEnough := !candidate.modTime.After(request.Before)
		overSize := request.MaxBytes > 0 && remainingBytes > request.MaxBytes
		if !oldEnough || (request.MaxBytes > 0 && !overSize) {
			break
		}
		if err := revalidateVacuumCandidate(candidate); err != nil {
			return result, vacuumPartialError(result, err)
		}
		if !request.DryRun {
			if err := candidate.directory.root.Remove(candidate.name); err != nil {
				return result, vacuumPartialError(result, fmt.Errorf("remove archived journal: %w", err))
			}
		}
		result.Files++
		result.Bytes += candidate.stat.allocated
		if remainingBytes < candidate.stat.allocated {
			remainingBytes = 0
		} else {
			remainingBytes -= candidate.stat.allocated
		}
		result.RemainingBytes = remainingBytes
	}
	return result, nil
}

func (c *Client) openVacuumDirectories() ([]*vacuumDirectory, error) {
	if c.target.MachineIDPath == "" {
		return nil, errors.New("systemd target machine ID path is not configured")
	}
	if len(c.target.JournalDirs) == 0 {
		return nil, errors.New("systemd target journal directories are not configured")
	}
	machineID, err := c.readMachineID()
	if err != nil {
		return nil, err
	}

	directories := make([]*vacuumDirectory, 0, len(c.target.JournalDirs))
	for _, basePath := range c.target.JournalDirs {
		baseRoot, err := c.openTargetDirectory(basePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			closeVacuumDirectories(directories)
			return nil, fmt.Errorf("open journal root %q: %w", basePath, err)
		}
		before, err := baseRoot.Lstat(machineID)
		if err != nil {
			baseRoot.Close()
			if os.IsNotExist(err) {
				continue
			}
			closeVacuumDirectories(directories)
			return nil, fmt.Errorf("inspect journal machine directory: %w", err)
		}
		if !before.IsDir() || before.Mode()&fs.ModeSymlink != 0 {
			baseRoot.Close()
			closeVacuumDirectories(directories)
			return nil, errors.New("journal machine directory is not a real directory")
		}
		beforeStat, err := journalStat(before)
		if err != nil {
			baseRoot.Close()
			closeVacuumDirectories(directories)
			return nil, fmt.Errorf("inspect journal machine directory identity: %w", err)
		}
		machineRoot, err := baseRoot.OpenRoot(machineID)
		baseRoot.Close()
		if err != nil {
			closeVacuumDirectories(directories)
			return nil, fmt.Errorf("open journal machine directory: %w", err)
		}
		after, err := machineRoot.Stat(".")
		if err != nil {
			machineRoot.Close()
			closeVacuumDirectories(directories)
			return nil, fmt.Errorf("verify journal machine directory: %w", err)
		}
		afterStat, err := journalStat(after)
		if err != nil || beforeStat.dev != afterStat.dev || beforeStat.ino != afterStat.ino {
			machineRoot.Close()
			closeVacuumDirectories(directories)
			return nil, errors.New("journal machine directory changed while opening")
		}
		directories = append(directories, &vacuumDirectory{path: basePath, root: machineRoot})
	}
	return directories, nil
}

// collectVacuumCandidates returns the deletable archived journal files and the
// total allocated bytes of every journal file in the pinned directories, active
// files included. The byte total is deliberately a superset of the candidate
// set so that subtracting a deleted candidate can never underflow it.
func collectVacuumCandidates(directories []*vacuumDirectory) ([]vacuumCandidate, uint64, error) {
	candidates := make([]vacuumCandidate, 0)
	var allocatedBytes uint64
	for _, directory := range directories {
		handle, err := directory.root.Open(".")
		if err != nil {
			return nil, 0, fmt.Errorf("open pinned journal directory: %w", err)
		}
		entries, readErr := handle.ReadDir(maxJournalFiles + 1)
		closeErr := handle.Close()
		if readErr != nil && readErr != io.EOF {
			return nil, 0, fmt.Errorf("read pinned journal directory: %w", readErr)
		}
		if closeErr != nil {
			return nil, 0, fmt.Errorf("close pinned journal directory: %w", closeErr)
		}
		if len(entries) > maxJournalFiles {
			return nil, 0, fmt.Errorf("journal directory has too many entries (maximum %d)", maxJournalFiles)
		}
		for _, entry := range entries {
			if entry.Type()&fs.ModeSymlink != 0 {
				continue
			}
			archived := isArchivedJournalName(entry.Name())
			if !archived && !strings.HasSuffix(entry.Name(), ".journal") {
				continue
			}
			info, err := directory.root.Lstat(entry.Name())
			if err != nil {
				return nil, 0, fmt.Errorf("inspect archived journal: %w", err)
			}
			if !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 {
				continue
			}
			stat, err := journalStat(info)
			if err != nil {
				return nil, 0, fmt.Errorf("inspect archived journal allocation: %w", err)
			}
			if allocatedBytes > math.MaxUint64-stat.allocated {
				return nil, 0, errors.New("journal allocation total overflow")
			}
			allocatedBytes += stat.allocated
			if !archived || stat.nlink != 1 {
				continue
			}
			candidates = append(candidates, vacuumCandidate{
				directory: directory,
				name:      entry.Name(),
				modTime:   info.ModTime(),
				size:      info.Size(),
				stat:      stat,
			})
			if len(candidates) > maxJournalFiles {
				return nil, 0, fmt.Errorf("systemd target has too many archived journal files (maximum %d)", maxJournalFiles)
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime.Equal(candidates[j].modTime) {
			if candidates[i].directory.path == candidates[j].directory.path {
				return candidates[i].name < candidates[j].name
			}
			return candidates[i].directory.path < candidates[j].directory.path
		}
		return candidates[i].modTime.Before(candidates[j].modTime)
	})
	return candidates, allocatedBytes, nil
}

func revalidateVacuumCandidate(candidate vacuumCandidate) error {
	info, err := candidate.directory.root.Lstat(candidate.name)
	if err != nil {
		return fmt.Errorf("revalidate archived journal: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 || info.Size() != candidate.size || !info.ModTime().Equal(candidate.modTime) {
		return errors.New("archived journal changed before deletion")
	}
	stat, err := journalStat(info)
	if err != nil {
		return fmt.Errorf("revalidate archived journal allocation: %w", err)
	}
	if stat.dev != candidate.stat.dev || stat.ino != candidate.stat.ino || stat.nlink != 1 || stat.blocks != candidate.stat.blocks {
		return errors.New("archived journal identity changed before deletion")
	}
	return nil
}

func closeVacuumDirectories(directories []*vacuumDirectory) {
	for _, directory := range directories {
		directory.root.Close()
	}
}

func vacuumPartialError(result builtins.JournalVacuumResult, err error) error {
	if result.Files == 0 {
		return err
	}
	return fmt.Errorf("journal vacuum stopped after deleting %d files (%d bytes): %w", result.Files, result.Bytes, err)
}

func (*Client) openTargetDirectory(path string) (*os.Root, error) {
	return os.OpenRoot(path)
}
