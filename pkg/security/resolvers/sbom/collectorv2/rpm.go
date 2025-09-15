// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && trivy

// Package collectorv2 holds sbom related files
package collectorv2

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/types"
	rpmdb "github.com/knqyf263/go-rpmdb/pkg"
)

var rpmdbPaths = []string{
	// Berkeley DB
	"usr/lib/sysimage/rpm/Packages",
	"var/lib/rpm/Packages",

	// NDB
	"usr/lib/sysimage/rpm/Packages.db",
	"var/lib/rpm/Packages.db",

	// SQLite3
	"usr/lib/sysimage/rpm/rpmdb.sqlite",
	"var/lib/rpm/rpmdb.sqlite",
}

type rpmScanner struct {
}

func (s *rpmScanner) Name() string {
	return "rpm"
}

func (s *rpmScanner) ListPackages(_ context.Context, root *os.Root) (types.Result, error) {
	for _, rpmdbPath := range rpmdbPaths {
		if _, err := root.Stat(rpmdbPath); err != nil {
			continue
		}

		// sadly, we need to escape the root here :(
		rpmdbFullPath := filepath.Join(root.Name(), rpmdbPath)
		db, err := rpmdb.Open(rpmdbFullPath)
		if err != nil {
			return types.Result{}, fmt.Errorf("failed to open rpmdb at path %s: %w", rpmdbPath, err)
		}
		defer db.Close()

		pkgs, err := db.ListPackages()
		if err != nil {
			return types.Result{}, fmt.Errorf("failed to list packages in rpmdb at path %s: %w", rpmdbPath, err)
		}

		packages := make([]ftypes.Package, 0, len(pkgs))
		for _, pkg := range pkgs {
			packages = append(packages, ftypes.Package{
				Name:       pkg.Name,
				Version:    pkg.Version,
				SrcVersion: pkg.Version,
			})
		}
		return types.Result{Packages: packages}, nil
	}

	return types.Result{}, fmt.Errorf("no rpmdb found in any of the known paths: %w", os.ErrNotExist)
}
