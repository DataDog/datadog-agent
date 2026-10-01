// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package sbom holds sbom related files
package sbom

import (
	"slices"
	"sort"
	"strings"

	sbomtypes "github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/types"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/twmb/murmur3"
)

// fileQuerier maps the installed files of a workload to the packages owning
// them. It holds the hashes of the file paths in increasing order, each with
// the index of the package listing it, so a lookup is a binary search. A path
// listed by two packages resolves to the first of them in the report.
type fileQuerier struct {
	hashes []uint64 // murmur3 hashes of the installed file paths, sorted
	owners []uint32 // owners[i] is the index in pkgs of the package listing hashes[i]
	pkgs   []*sbomtypes.Package

	usrMerged bool
}

// newFileQuerier builds the file->package index from report. It stores murmur3
// hashes of the installed file paths — never the paths themselves — together with
// pointers into backing, the caller's long-lived per-package metadata slice. Using
// backing (rather than report) for the pointers keeps LastAccess/SuidBit/
// AccessedByRoot updates visible to the forwarding path while allowing report and
// its plain-text InstalledFiles to be garbage-collected once this returns.
// backing[i] must correspond to report[i].Package.
func newFileQuerier(report []sbomtypes.PackageWithInstalledFiles, backing []sbomtypes.Package, usrMerged bool) fileQuerier {
	fileCount := 0
	for _, pkg := range report {
		fileCount += len(pkg.InstalledFiles)
	}

	fq := fileQuerier{
		hashes:    make([]uint64, 0, fileCount),
		owners:    make([]uint32, 0, fileCount),
		pkgs:      make([]*sbomtypes.Package, 0, len(backing)),
		usrMerged: usrMerged,
	}

	for i := range report {
		// IMPORTANT: Store pointer into the retained backing slice, not into report,
		// so LastAccess updates are reflected in the stored packages and report's
		// InstalledFiles are not kept alive.
		fq.pkgs = append(fq.pkgs, &backing[i])

		for _, file := range report[i].InstalledFiles {
			seclog.Tracef("indexing %s as %+v", file, backing[i])

			fq.hashes = append(fq.hashes, murmur3.StringSum64(file))
			fq.owners = append(fq.owners, uint32(i))
		}
	}

	sort.Sort(fileOrder{hashes: fq.hashes, owners: fq.owners})

	return fq
}

// fileOrder sorts installed files by the hash of their path, then by the index
// of the package listing them, moving each hash with its owner.
type fileOrder struct {
	hashes []uint64
	owners []uint32
}

func (o fileOrder) Len() int { return len(o.hashes) }

func (o fileOrder) Less(i, j int) bool {
	if o.hashes[i] != o.hashes[j] {
		return o.hashes[i] < o.hashes[j]
	}
	return o.owners[i] < o.owners[j]
}

func (o fileOrder) Swap(i, j int) {
	o.hashes[i], o.hashes[j] = o.hashes[j], o.hashes[i]
	o.owners[i], o.owners[j] = o.owners[j], o.owners[i]
}

// queryHash returns the package listing the file whose path hashes to hash. The
// search lands on the first entry of that hash, which belongs to the first
// package listing it.
func (fq *fileQuerier) queryHash(hash uint64) *sbomtypes.Package {
	i, found := slices.BinarySearch(fq.hashes, hash)
	if !found {
		return nil
	}
	return fq.pkgs[fq.owners[i]]
}

func (fq *fileQuerier) queryFile(path string) *sbomtypes.Package {
	if pkg := fq.queryHash(murmur3.StringSum64(path)); pkg != nil {
		return pkg
	}

	// On usr-merged layouts /bin and /usr/bin are the same tree, so the package
	// database and the resolved exec path may use either prefix for one file.
	if fq.usrMerged {
		if !strings.HasPrefix(path, "/usr") && (strings.HasPrefix(path, "/bin") || strings.HasPrefix(path, "/sbin") || strings.HasPrefix(path, "/lib")) {
			if result := fq.queryHash(murmur3.StringSum64("/usr" + path)); result != nil {
				return result
			}
		}

		if after, ok := strings.CutPrefix(path, "/usr"); ok && (strings.HasPrefix(after, "/bin") || strings.HasPrefix(after, "/sbin") || strings.HasPrefix(after, "/lib")) {
			if result := fq.queryHash(murmur3.StringSum64(after)); result != nil {
				return result
			}
		}
	}

	return nil
}

func (fq *fileQuerier) len() int {
	return len(fq.hashes)
}
