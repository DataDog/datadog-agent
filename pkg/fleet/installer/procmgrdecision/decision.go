// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package procmgrdecision decides whether the Agent runs its processes under
// dd-procmgrd (the process manager) or under the OS service manager.
//
// The decision comes from, in order: an explicit value for the operation, the
// decision record kept by the installer, a one-time legacy seed read from an
// older knob on disk, and the default (process manager enabled). See Resolve.
//
// The package never logs. Non-fatal remarks are returned as strings so that the
// caller can log them.
package procmgrdecision

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

// Default is the decision when nothing else applies: procmgr enabled.
const Default = true

// Source says where a decision came from.
type Source string

const (
	// SourceExplicit is a value supplied for this operation.
	SourceExplicit Source = "explicit"
	// SourceRecord is the decision record written by the installer.
	SourceRecord Source = "record"
	// SourceSeed is a legacy knob on disk.
	SourceSeed Source = "seed"
	// SourceDefault is the default decision.
	SourceDefault Source = "default"
)

// Decision is the result of Resolve.
type Decision struct {
	Enabled bool
	Source  Source
	// Detail is the path given to the seed reader when Source is SourceSeed, empty otherwise.
	Detail string
	// Warning is a non-fatal remark from the seed reader, empty otherwise. The caller logs it.
	Warning string
}

var (
	// ErrUntrusted: the record directory or file exists but fails the ownership or permission check.
	ErrUntrusted = errors.New("process manager record is not trusted")
	// ErrInvalid: the record file exists and is trusted but its content is not a valid record.
	ErrInvalid = errors.New("process manager record is invalid")
)

// SeedError is returned when a legacy knob exists but cannot be read or understood.
type SeedError struct {
	Detail string // the path of the knob
	Err    error
}

func (e *SeedError) Error() string {
	return fmt.Sprintf("%s: %v", e.Detail, e.Err)
}

func (e *SeedError) Unwrap() error {
	return e.Err
}

// RecordReader is what Resolve needs from the record. *Store implements it.
type RecordReader interface {
	Read() (enabled bool, found bool, err error)
}

// SeedResult is what a seed reader found.
type SeedResult struct {
	Found   bool
	Enabled bool
	Detail  string
	Warning string
}

// Seed reads one legacy knob. Phase 1 only provides the readers, callers choose which one applies.
type Seed func() (SeedResult, error)

// Resolve applies, in order: explicit, then record, then seed, then Default.
//
// An explicit value is returned without calling record or seed, so a broken
// record never blocks it. A record or seed error is returned with a zero
// Decision and never falls through to the next step: a failed check must not
// start the process manager by default. Resolve has no side effects.
//
// Pass an untyped nil to skip the record or the seed.
func Resolve(explicit *bool, record RecordReader, seed Seed) (Decision, error) {
	if explicit != nil {
		return Decision{Enabled: *explicit, Source: SourceExplicit}, nil
	}
	if record != nil {
		enabled, found, err := record.Read()
		if err != nil {
			return Decision{}, fmt.Errorf("reading process manager record: %w", err)
		}
		if found {
			return Decision{Enabled: enabled, Source: SourceRecord}, nil
		}
	}
	if seed != nil {
		res, err := seed()
		if err != nil {
			return Decision{}, fmt.Errorf("reading legacy process manager setting: %w", err)
		}
		if res.Found {
			return Decision{Enabled: res.Enabled, Source: SourceSeed, Detail: res.Detail, Warning: res.Warning}, nil
		}
	}
	return Decision{Enabled: Default, Source: SourceDefault}, nil
}

// maxSeedFileSize bounds the work done on a legacy knob file. A larger file is a SeedError.
const maxSeedFileSize = 4 << 20

// readSeedFile reads a legacy knob file. A missing file is found=false with no error.
// It follows symlinks (datadog.yaml may legitimately be one), requires a regular
// file, and reads at most maxSeedFileSize bytes.
func readSeedFile(path string) (data []byte, found bool, err error) {
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !fi.Mode().IsRegular() {
		return nil, false, fmt.Errorf("%s is not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	// Reading one byte past the limit detects a file that grew after the Stat.
	data, err = io.ReadAll(io.LimitReader(f, maxSeedFileSize+1))
	if err != nil {
		return nil, false, err
	}
	if len(data) > maxSeedFileSize {
		return nil, false, fmt.Errorf("%s is larger than %d bytes", path, maxSeedFileSize)
	}
	return data, true, nil
}
