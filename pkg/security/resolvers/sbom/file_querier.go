// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package sbom holds sbom related files
package sbom

import (
	"slices"
	"strings"

	sbomtypes "github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/types"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/twmb/murmur3"
)

type fileQuerier struct {
	files []uint64
	pkgs  []*sbomtypes.Package

	usrMerged bool

	lastNegativeCache *fixedSizeQueue[uint64]
}

/*
files are stored in the following format:

| partSize | hash1 | hash2 | ... | partSize | hash3 | hash4 | ... | partSize | hash5 | hash6 | ... |

where partSize is the number of hashes in the part
and each part group is at the index of the given package
for example here hash5 would match pkgs[2]
*/

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
		fileCount += 2 + len(pkg.InstalledFiles)
	}

	files := make([]uint64, 0, fileCount)
	pkgs := make([]*sbomtypes.Package, 0, len(backing))

	for i := range report {
		// IMPORTANT: Store pointer into the retained backing slice, not into report,
		// so LastAccess updates are reflected in the stored packages and report's
		// InstalledFiles are not kept alive.
		pkgs = append(pkgs, &backing[i])

		files = append(files, uint64(len(report[i].InstalledFiles)))

		for _, file := range report[i].InstalledFiles {
			seclog.Tracef("indexing %s as %+v", file, backing[i])

			hash := murmur3.StringSum64(file)
			files = append(files, hash)
		}
	}

	return fileQuerier{files: files, pkgs: pkgs, usrMerged: usrMerged, lastNegativeCache: newFixedSizeQueue[uint64](2)}
}

func (fq *fileQuerier) queryHash(hash uint64) *sbomtypes.Package {
	// fast path, if no package in the report contains the file
	if !slices.Contains(fq.files, hash) {
		return nil
	}

	var i, pkgIndex uint64
	for i < uint64(len(fq.files)) {
		partSize := fq.files[i]

		for offset := uint64(0); offset < partSize; offset++ {
			if fq.files[i+1+offset] == hash {
				return fq.pkgs[pkgIndex]
			}
		}

		i += partSize + 1
		pkgIndex++
	}

	return nil
}

func (fq *fileQuerier) queryHashWithNegativeCache(hash uint64) *sbomtypes.Package {
	if fq.lastNegativeCache.contains(hash) {
		return nil
	}

	pkg := fq.queryHash(hash)
	if pkg == nil {
		if fq.lastNegativeCache == nil {
			fq.lastNegativeCache = newFixedSizeQueue[uint64](2)
		}
		fq.lastNegativeCache.push(hash)
	}

	return pkg
}

func (fq *fileQuerier) queryFile(path string) *sbomtypes.Package {
	if pkg := fq.queryHashWithNegativeCache(murmur3.StringSum64(path)); pkg != nil {
		return pkg
	}

	// On usr-merged layouts /bin and /usr/bin are the same tree, so the package
	// database and the resolved exec path may use either prefix for one file.
	if fq.usrMerged {
		if !strings.HasPrefix(path, "/usr") && (strings.HasPrefix(path, "/bin") || strings.HasPrefix(path, "/sbin") || strings.HasPrefix(path, "/lib")) {
			if result := fq.queryHashWithNegativeCache(murmur3.StringSum64("/usr" + path)); result != nil {
				return result
			}
		}

		if after, ok := strings.CutPrefix(path, "/usr"); ok && (strings.HasPrefix(after, "/bin") || strings.HasPrefix(after, "/sbin") || strings.HasPrefix(after, "/lib")) {
			if result := fq.queryHashWithNegativeCache(murmur3.StringSum64(after)); result != nil {
				return result
			}
		}
	}

	return nil
}

func (fq *fileQuerier) len() int {
	return len(fq.files)
}

type fixedSizeQueue[T comparable] struct {
	queue   []T
	maxSize int
}

func newFixedSizeQueue[T comparable](maxSize int) *fixedSizeQueue[T] {
	return &fixedSizeQueue[T]{maxSize: maxSize}
}

func (q *fixedSizeQueue[T]) push(value T) {
	if len(q.queue) == q.maxSize {
		q.queue = q.queue[1:]
	}

	q.queue = append(q.queue, value)
}

func (q *fixedSizeQueue[T]) contains(value T) bool {
	if q == nil {
		return false
	}

	return slices.Contains(q.queue, value)
}
