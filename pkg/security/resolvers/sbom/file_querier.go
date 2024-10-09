// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && trivy

// Package sbom holds sbom related files
package sbom

import (
	"cmp"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
	"github.com/twmb/murmur3"
)

type fileQuerier struct {
	files []fqEntry
	pkgs  []*Package
}

type fqEntry struct {
	hash     uint64
	pkgIndex uint64
}

func newFileQuerier(report *trivy.Report) fileQuerier {
	fileCount := 0
	pkgCount := 0
	for _, result := range report.Results {
		for _, resultPkg := range result.Packages {
			fileCount += len(resultPkg.InstalledFiles)
			pkgCount++
		}
	}

	files := make([]fqEntry, 0, fileCount)
	pkgs := make([]*Package, 0, pkgCount)
	for _, result := range report.Results {
		for _, resultPkg := range result.Packages {
			pkg := &Package{
				Name:       resultPkg.Name,
				Version:    resultPkg.Version,
				SrcVersion: resultPkg.SrcVersion,
			}
			pkgIndex := uint64(len(pkgs))
			pkgs = append(pkgs, pkg)

			for _, file := range resultPkg.InstalledFiles {
				seclog.Infof("indexing %s as %+v", file, pkg)
				files = append(files, fqEntry{hash: murmur3.StringSum64(file), pkgIndex: pkgIndex})
			}
		}
	}

	slices.SortFunc(files, func(a, b fqEntry) int {
		return cmp.Compare(a.hash, b.hash)
	})

	return fileQuerier{files: files}
}

func (fq *fileQuerier) queryHash(hash uint64) *Package {
	i, found := slices.BinarySearchFunc(fq.files, hash, func(entry fqEntry, hash uint64) int {
		return cmp.Compare(entry.hash, hash)
	})
	if !found {
		return nil
	}
	return fq.pkgs[fq.files[i].pkgIndex]
}

func (fq *fileQuerier) queryFile(path string) *Package {
	if pkg := fq.queryHash(murmur3.StringSum64(path)); pkg != nil {
		return pkg
	}

	if strings.HasPrefix(path, "/usr") {
		return fq.queryHash(murmur3.StringSum64(path[4:]))
	}

	return nil
}

func (fq *fileQuerier) len() int {
	return len(fq.files)
}
