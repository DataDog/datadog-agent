// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && trivy

// Package sbom holds sbom related files
package sbom

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
	"github.com/twmb/murmur3"
)

type fileQuerier struct {
	files []uint64
	pkgs  []*Package
}

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
				seclog.Infof("indexing %s as %+v", file, pkg)

				hash := murmur3.StringSum64(file)
				files = append(files, hash)
			}
		}
	}

	return fileQuerier{files: files, pkgs: pkgs}
}

func (fq *fileQuerier) queryHash(hash uint64) *Package {
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
