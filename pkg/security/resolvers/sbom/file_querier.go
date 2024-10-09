// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && trivy

// Package sbom holds sbom related files
package sbom

import (
	"math"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
	"github.com/twmb/murmur3"
)

type fileQuerier struct {
	files []uint64
	pkgs  []*Package
}

const hashSentinel uint64 = math.MaxUint64

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
			pkgIndex := uint64(len(pkgs))
			pkgs = append(pkgs, pkg)

			files = append(files, hashSentinel, pkgIndex)

			for _, file := range resultPkg.InstalledFiles {
				seclog.Infof("indexing %s as %+v", file, pkg)

				hash := murmur3.StringSum64(file)
				if hash == hashSentinel {
					seclog.Errorf("failed to hash %s", file)
					continue
				}

				files = append(files, hash)
			}
		}
	}

	return fileQuerier{files: files, pkgs: pkgs}
}

func (fq *fileQuerier) queryHash(hash uint64) *Package {
	var currentPkg *Package
	for i, h := range fq.files {
		if h == hashSentinel {
			if i+1 < len(fq.files) {
				currentPkg = fq.pkgs[fq.files[i+1]]
			} else {
				currentPkg = nil
			}
			continue
		}

		if h == hash {
			return currentPkg
		}
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
