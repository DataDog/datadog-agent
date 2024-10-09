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
	files map[uint64]*Package
}

func newEmptyFileQuerier() fileQuerier {
	return fileQuerier{files: make(map[uint64]*Package)}
}

func newFileQuerier(report *trivy.Report) fileQuerier {
	files := make(map[uint64]*Package)
	for _, result := range report.Results {
		for _, resultPkg := range result.Packages {
			pkg := &Package{
				Name:       resultPkg.Name,
				Version:    resultPkg.Version,
				SrcVersion: resultPkg.SrcVersion,
			}
			for _, file := range resultPkg.InstalledFiles {
				seclog.Tracef("indexing %s as %+v", file, pkg)
				files[murmur3.StringSum64(file)] = pkg
			}
		}
	}
	return fileQuerier{files: files}
}

func (fq *fileQuerier) queryFile(path string) *Package {
	if pkg := fq.files[murmur3.StringSum64(path)]; pkg != nil {
		return pkg
	}

	if strings.HasPrefix(path, "/usr") {
		return fq.files[murmur3.StringSum64(path[4:])]
	}

	return nil
}

func (fq *fileQuerier) len() int {
	return len(fq.files)
}
