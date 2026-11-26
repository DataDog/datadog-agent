// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usage

import (
	"errors"
	"fmt"
)

// The flags of a key in a Report.
const (
	FlagSetuid uint8 = 1 << iota // a process ran the file setuid
	FlagRoot                     // a process ran the file as root

	allFlags = FlagSetuid | FlagRoot
)

// KeyKind says which workload a Report covers.
type KeyKind uint8

const (
	// KeyHost is the host, outside containers.
	KeyHost KeyKind = iota
	// KeyImage is an image: the containers of ImageID share the report.
	KeyImage
	// KeyContainer is a container whose image system-probe could not name.
	KeyContainer
)

// MaxKeys bounds the keys of a Report.
const MaxKeys = 1 << 20

// Version is the version of the keys and of the fields of a Report, which
// changes with the way Key and AppendKeys name files.
const Version = 1

// A Report is the usage that system-probe records for a workload: the keys of
// the files that ran, in increasing order, with the last time each ran and its
// flags.
type Report struct {
	Keys     []uint64
	LastSeen []int64 // unix seconds, by key
	Flags    []uint8 // by key

	// ObservedSince is when the recording of the workload started, in unix
	// seconds.
	ObservedSince int64
	// Truncated reports that the recorder reached its bound of keys, so that
	// a file absent from Keys may have run.
	Truncated bool

	KeyKind      KeyKind
	ImageID      string   // the image, with KeyImage
	ContainerIDs []string // the containers the report covers, with KeyImage and KeyContainer

	// Replaced lists, by key, the builds of host files that a package
	// replaced while processes ran them.
	Replaced []Replaced
}

// Replaced is the usage of a build of a host file before a package replaced
// it.
type Replaced struct {
	Key          uint64
	ReplacedAt   int64 // unix seconds
	LastSeen     int64 // unix seconds
	Flags        uint8
	StillRunning bool // a process still runs the replaced build
}

// Validate reports the first inconsistency of r, which comes from another
// process.
func (r *Report) Validate() error {
	n := len(r.Keys)
	switch {
	case n > MaxKeys:
		return fmt.Errorf("usage report: %d keys, want at most %d", n, MaxKeys)
	case len(r.LastSeen) != n || len(r.Flags) != n:
		return fmt.Errorf("usage report: %d keys, %d times and %d flags, want as many", n, len(r.LastSeen), len(r.Flags))
	case r.ObservedSince < 0:
		return fmt.Errorf("usage report: observed since %d", r.ObservedSince)
	}
	for i, key := range r.Keys {
		if i > 0 && key <= r.Keys[i-1] {
			return errors.New("usage report: keys out of order")
		}
		if r.LastSeen[i] <= 0 || r.Flags[i]&^allFlags != 0 {
			return fmt.Errorf("usage report: key %#x last seen %d with flags %#x", key, r.LastSeen[i], r.Flags[i])
		}
	}

	switch r.KeyKind {
	case KeyHost:
		if r.ImageID != "" || len(r.ContainerIDs) != 0 {
			return errors.New("usage report: host report with an image or containers")
		}
	case KeyImage:
		if r.ImageID == "" {
			return errors.New("usage report: image report without an image")
		}
	case KeyContainer:
		if r.ImageID != "" || len(r.ContainerIDs) != 1 {
			return fmt.Errorf("usage report: container report with image %q and %d containers", r.ImageID, len(r.ContainerIDs))
		}
	default:
		return fmt.Errorf("usage report: key kind %d", r.KeyKind)
	}

	if len(r.Replaced) > 0 && r.KeyKind != KeyHost {
		return errors.New("usage report: replaced builds outside the host")
	}
	if len(r.Replaced) > MaxKeys {
		return fmt.Errorf("usage report: %d replaced builds, want at most %d", len(r.Replaced), MaxKeys)
	}
	for i, rep := range r.Replaced {
		if i > 0 && rep.Key <= r.Replaced[i-1].Key {
			return errors.New("usage report: replaced builds out of order")
		}
		if rep.ReplacedAt <= 0 || rep.LastSeen <= 0 || rep.Flags&^allFlags != 0 {
			return fmt.Errorf("usage report: replaced build of %#x at %d, last seen %d with flags %#x", rep.Key, rep.ReplacedAt, rep.LastSeen, rep.Flags)
		}
	}
	return nil
}
