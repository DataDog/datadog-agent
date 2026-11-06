// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package amd

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// RASCounters are the cumulative RAS error counters of a device, from the
// plain-text files of its ras/ sysfs directory
// (https://docs.kernel.org/gpu/amdgpu/ras.html). A counter is nil when the
// device does not report it.
type RASCounters struct {
	Uncorrectable  *uint64 // sum of "ue" over the <block>_err_count files
	Correctable    *uint64 // sum of "ce" over the <block>_err_count files
	Deferred       *uint64 // sum of "de" (UMC only) over the <block>_err_count files
	Fatal          *uint64 // "Fatal Error" count in event_state
	PoisonCreated  *uint64 // "Poison Creation" count in event_state
	PoisonConsumed *uint64 // "Poison Consumption" count in event_state
}

// readRAS reads the RAS counters under devicePath/ras. Devices without RAS
// support have no such directory and return empty counters.
func (r *reader) readRAS(devicePath string) RASCounters {
	var c RASCounters
	dir := filepath.Join(devicePath, "ras")
	incomplete := false
	for _, entry := range r.readDir(dir) {
		if !strings.HasSuffix(entry.Name(), "_err_count") {
			continue
		}
		content, ok := r.read(filepath.Join(dir, entry.Name()))
		if !ok {
			incomplete = true
			continue
		}
		counts, err := parseKeyCounts(content, ": ")
		if err != nil {
			// The driver writes "Query currently inaccessible" while the block
			// cannot be queried: not an error, just no value this time.
			incomplete = true
			continue
		}
		addCount(&c.Uncorrectable, counts, "ue")
		addCount(&c.Correctable, counts, "ce")
		addCount(&c.Deferred, counts, "de")
	}
	if incomplete {
		// A sum missing a block would look like a drop, then an increase once
		// the block is back: report no sum rather than a partial one.
		c.Uncorrectable, c.Correctable, c.Deferred = nil, nil, nil
	}

	if content, ok := r.read(filepath.Join(dir, "event_state")); ok {
		counts, err := parseEventState(content)
		if err != nil {
			r.errs = append(r.errs, fmt.Errorf("parse %s: %w", filepath.Join(dir, "event_state"), err))
		} else {
			addCount(&c.Fatal, counts, "Fatal Error")
			addCount(&c.PoisonCreated, counts, "Poison Creation")
			addCount(&c.PoisonConsumed, counts, "Poison Consumption")
		}
	}
	return c
}

func addCount(dst **uint64, counts map[string]uint64, key string) {
	v, ok := counts[key]
	if !ok {
		return
	}
	if *dst == nil {
		*dst = new(uint64)
	}
	**dst += v
}

// parseKeyCounts parses "key<sep>value" lines such as "ue: 0".
func parseKeyCounts(content, sep string) (map[string]uint64, error) {
	counts := make(map[string]uint64)
	for line := range strings.Lines(content) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, found := strings.Cut(line, sep)
		if !found {
			return nil, fmt.Errorf("unexpected line %q", line)
		}
		n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("unexpected line %q: %w", line, err)
		}
		counts[strings.TrimSpace(key)] = n
	}
	return counts, nil
}

// parseEventState parses the event_state file, whose lines other than the
// sequence number look like "Fatal Error: count:0, last_seqno:0".
func parseEventState(content string) (map[string]uint64, error) {
	counts := make(map[string]uint64)
	for line := range strings.Lines(content) {
		name, rest, found := strings.Cut(strings.TrimSpace(line), ": count:")
		if !found {
			continue
		}
		value, _, _ := strings.Cut(rest, ",")
		n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("unexpected line %q: %w", line, err)
		}
		counts[name] = n
	}
	return counts, nil
}
