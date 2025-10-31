// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procscan

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"regexp"
	"slices"
	"strconv"
)

// listPidsChunks lists the PIDs of the processes in the given procfs root by pages.
//
// It will never report an empty page, but it may report a page smaller than
// the page size even if it is not the last page.
func listPidsChunks(procfsRoot string, pageSize int) iter.Seq2[[]uint32, error] {
	return func(yield func([]uint32, error) bool) {
		f, err := os.Open(procfsRoot)
		if err != nil {
			yield(nil, fmt.Errorf("open procfs: %w", err))
			return
		}
		defer f.Close()
		var sawEOF bool
		for !sawEOF {
			entries, err := f.Readdirnames(pageSize)
			if errors.Is(err, io.EOF) {
				sawEOF, err = true, nil
			}
			if err != nil {
				yield(nil, fmt.Errorf("read procfs dirnames: %w", err))
				return
			}
			entries = slices.DeleteFunc(entries, isNotAPid)
			pids := make([]uint32, 0, len(entries))
			for _, entry := range entries {
				pid, parseErr := strconv.ParseUint(entry, 10, 32)
				if parseErr != nil {
					continue
				}
				pids = append(pids, uint32(pid))
			}
			if len(pids) == 0 {
				continue
			}
			if !yield(pids, nil) {
				return
			}
		}
	}
}

// listPids lists the PIDs of the processes in the given procfs root.
//
// Internall, it paginates the procfs directory by pages of the given size.
func listPids(procfsRoot string, pageSize int) iter.Seq2[uint32, error] {
	return func(yield func(uint32, error) bool) {
		for pids, err := range listPidsChunks(procfsRoot, pageSize) {
			if err != nil {
				yield(0, err)
				return
			}
			for _, pid := range pids {
				if !yield(pid, nil) {
					return
				}
			}
		}
	}
}

var maybePidRegex = regexp.MustCompile(`^[1-9]\d{0,9}$`)

func isNotAPid(entry string) bool {
	return !maybePidRegex.MatchString(entry)
}
