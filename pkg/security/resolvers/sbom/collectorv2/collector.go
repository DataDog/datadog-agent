// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package collectorv2 holds sbom related files
package collectorv2

import (
	"context"
	"errors"
	"os"

	sbomtypes "github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/types"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// OSScanner is responsible for scanning the host OS for packages
type OSScanner struct {
	scanners []actualScanner
}

type actualScanner interface {
	Name() string
	ListPackages(ctx context.Context, root *os.Root) ([]sbomtypes.PackageWithInstalledFiles, error)
}

// NewOSScanner returns a new OSScanner
func NewOSScanner() *OSScanner {
	return &OSScanner{
		scanners: []actualScanner{
			&dpkgScanner{},
			&rpmScanner{},
			&apkScanner{},
		},
	}
}

// ScanInstalledPackages scans the given rootfs and returns a list of installed packages
func (s *OSScanner) ScanInstalledPackages(ctx context.Context, root string) ([]sbomtypes.PackageWithInstalledFiles, error) {
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer rootFS.Close()

	var pkgs []sbomtypes.PackageWithInstalledFiles
	for _, scanner := range s.scanners {
		result, err := scanner.ListPackages(ctx, rootFS)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				seclog.Errorf("failed to list packages with %s scanner: %v", scanner.Name(), err)
			}
			continue
		}
		pkgs = append(pkgs, result...)
	}
	return pkgs, nil
}
