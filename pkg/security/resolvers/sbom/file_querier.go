// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && trivy

// Package sbom holds sbom related files
package sbom

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
	"github.com/twmb/murmur3"
)

type fileQuerier struct {
	files []uint64
	pkgs  []*Package

	lastNegativeCache *fixedSizeQueue[uint64]
}

/*
files are stored in the following format:

| partSize | hash1 | hash2 | ... | partSize | hash3 | hash4 | ... | partSize | hash5 | hash6 | ... |

where partSize is the number of hashes in the part
and each part group is at the index of the given package
for example here hash5 would match pkgs[2]
*/

func newFileQuerier(report *trivy.Report) fileQuerier {
	fileCount := 0
	pkgCount := 0
	for _, result := range report.Results {
		for _, resultPkg := range result.Packages {
			fileCount += 2 + len(resultPkg.InstalledFiles)
			pkgCount++
		}
	}

	files := make([]uint64, 0, fileCount)
	pkgs := make([]*Package, 0, pkgCount)

	for _, result := range report.Results {
		for _, resultPkg := range result.Packages {
			pkg := &Package{
				Name:       resultPkg.Name,
				Version:    resultPkg.Version,
				SrcVersion: resultPkg.SrcVersion,
			}
			pkgs = append(pkgs, pkg)

			files = append(files, uint64(len(resultPkg.InstalledFiles)))

			for _, file := range resultPkg.InstalledFiles {
				seclog.Tracef("indexing %s as %+v", file, pkg)

				hash := murmur3.StringSum64(file)
				files = append(files, hash)
			}
		}
	}

	return fileQuerier{files: files, pkgs: pkgs, lastNegativeCache: newFixedSizeQueue[uint64](2)}
}

func (fq *fileQuerier) queryHash(hash uint64) *Package {
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

func (fq *fileQuerier) queryHashWithNegativeCache(hash uint64) *Package {
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

func (fq *fileQuerier) queryFile(path string) *Package {
	if pkg := fq.queryHashWithNegativeCache(murmur3.StringSum64(path)); pkg != nil {
		return pkg
	}

	if strings.HasPrefix(path, "/usr") {
		return fq.queryHashWithNegativeCache(murmur3.StringSum64(path[4:]))
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

	for _, v := range q.queue {
		if v == value {
			return true
		}
	}

	return false
}
