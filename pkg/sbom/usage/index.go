// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package usage carries the file-to-component table a scan produces, so a
// runtime observer can name a component of that scan's SBOM without deriving
// its identity from a name and a version.
//
// The core agent builds an Index when it scans a workload and hands it to
// system-probe. system-probe attributes the file accesses it observes against
// that index and answers with a Report naming the same refs. The core agent
// stamps the result onto the SBOM it owns.
package usage

import (
	"slices"
	"strings"
	"time"
)

// ScanID names what a scan describes. It carries the kind as well as the
// identity, so a consumer can tell an image apart from one container's own
// filesystem without knowing how either identity is spelled.
type ScanID string

// Host is the ScanID of the host filesystem scan.
const Host ScanID = "host"

const (
	imagePrefix     = "image:"
	containerPrefix = "container:"
)

// ImageScan returns the ScanID of an image, named by its entity ID.
func ImageScan(imageID string) ScanID {
	if imageID == "" {
		return ""
	}
	return ScanID(imagePrefix + imageID)
}

// ContainerScan returns the ScanID of one container's own filesystem, read
// because the container diverged from the image it started from.
func ContainerScan(containerID string) ScanID {
	if containerID == "" {
		return ""
	}
	return ScanID(containerPrefix + containerID)
}

// IsContainer reports whether the scan describes one container's filesystem
// rather than an image or the host.
func (s ScanID) IsContainer() bool {
	return strings.HasPrefix(string(s), containerPrefix)
}

// Status reports what a core agent has to say about a scan.
type Status int

const (
	// Ready means the index carries a table.
	Ready Status = iota
	// Failed means this scan produced no table and will not be retried, so a
	// consumer should stop waiting for one.
	Failed
	// Gone means the workload is finished with, so a consumer should release
	// the index and any usage held against it.
	Gone
)

// Component names one component occurrence of the BOM built from the same
// scan. BOMRef is the occurrence identity in the final payload and stays in the
// core agent. Purl is non-unique package-coordinate metadata. The remaining
// fields cross to system-probe for SECL package.* resolution.
type Component struct {
	BOMRef string
	Purl   string
	// Reportable tells a wire-side runtime observer that this occurrence has a
	// unique final BOM ref and may therefore appear in a usage report. The core
	// derives the wire value from BOMRef and the path table.
	Reportable bool
	Name       string
	Version    string
	Epoch      int
	Release    string
	SrcVersion string
	SrcEpoch   int
	SrcRelease string

	// Application marks the component as the artifact that contains the others
	// reached through the same path, a Go binary or a language runtime. It is
	// what Lookup prefers when a path resolves to more than one component.
	Application bool
}

// Index is the file-to-component table of one exact scan/BOM instance.
//
// Hashes, Refs and Paths are parallel and sorted by hash, so a lookup is a
// binary search that returns a range rather than a single entry: one path can
// belong to several components, as every module compiled into a Go binary
// belongs to the one file.
//
// The builder fills all three. Only the hashes cross the process boundary,
// except to a consumer that asked for paths because it builds kernel-side
// filters from them, and an index received over the wire therefore carries one
// form or the other rather than both.
type Index struct {
	Scan       ScanID
	Generation uint64
	// IndexID scopes component refs to the exact BOM instance. It is the final
	// payload's CycloneDX serial number and is treated as an opaque value.
	IndexID    string
	Status     Status
	Components []Component
	Hashes     []uint64
	Refs       []uint32
	Paths      []string

	// UnmappedComponents is core-agent-local diagnostic state. These components
	// remain available to system-probe package resolution but cannot be stamped
	// because the source report could not be joined uniquely to a final BOM ref.
	UnmappedComponents int
	// HashCollisions counts hash values shared by distinct paths and therefore
	// removed from this index. It is core-agent-local diagnostic state.
	HashCollisions int
}

// Lookup returns the refs of the components the given path hash is attributed
// to. The containing artifact comes first where the index names one, so a caller
// that wants a single answer can take the first and one that wants them all can
// range. It returns nil when the hash names no component.
func (idx *Index) Lookup(hash uint64) []uint32 {
	first, found := slices.BinarySearch(idx.Hashes, hash)
	if !found {
		return nil
	}

	var refs []uint32
	app := -1
	for i := first; i < len(idx.Hashes) && idx.Hashes[i] == hash; i++ {
		ref := idx.Refs[i]
		if int(ref) >= len(idx.Components) {
			continue
		}
		if idx.Components[ref].Application {
			app = len(refs)
		}
		refs = append(refs, ref)
	}
	if app > 0 {
		refs[0], refs[app] = refs[app], refs[0]
	}
	return refs
}

// Component returns the component a ref names, or nil when the ref is out of
// range.
func (idx *Index) Component(ref uint32) *Component {
	if int(ref) >= len(idx.Components) {
		return nil
	}
	return &idx.Components[ref]
}

// Usage is what a runtime observer saw of one component.
type Usage struct {
	Ref      uint32
	LastSeen time.Time
	Suid     bool
	AsRoot   bool
}

// Report is the answer to one exact index instance. It names only the components
// that were seen running, so a component absent from a report was measured and
// found idle, which a consumer can tell apart from a component absent from the
// index and therefore never measurable at all.
type Report struct {
	Scan       ScanID
	Generation uint64
	IndexID    string
	Usage      []Usage
}

// ReportAck says whether a usage report was applied and identifies the index
// currently active for its scan. A rejected report can therefore be diagnosed
// without overloading generation zero.
type ReportAck struct {
	Scan       ScanID
	Generation uint64
	IndexID    string
	Applied    bool
}

// Capabilities names the scan sources a core agent has running, so a consumer
// learns which indexes to expect from a fact rather than from a configuration
// key belonging to another process.
type Capabilities struct {
	ContainerImage bool
	Container      bool
	Host           bool
}

// Any reports whether any scan source is running. With none, no index will ever
// arrive and every capability that needs one is unavailable.
func (c Capabilities) Any() bool {
	return c.ContainerImage || c.Container || c.Host
}

// Workloads reports whether a source covering containers is running. The host
// source alone leaves container workloads unindexed.
func (c Capabilities) Workloads() bool {
	return c.ContainerImage || c.Container
}
