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
	"fmt"
	"hash/fnv"
	"os"
	"slices"
	"strconv"
	"strings"

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

	for i := range pkgs {
		// The dpkg entries of the architectures of a package share one slice.
		files := make([]string, 0, len(pkgs[i].InstalledFiles))
		for _, file := range pkgs[i].InstalledFiles {
			if !isDoc(file) {
				files = append(files, file)
			}
		}
		pkgs[i].InstalledFiles = files
	}
	return pkgs, nil
}

// docDirs hold the documentation and translations of packages, which processes
// read as data.
var docDirs = []string{"/usr/share/doc/", "/usr/share/info/", "/usr/share/licenses/", "/usr/share/locale/", "/usr/share/man/"}

func isDoc(path string) bool {
	return slices.ContainsFunc(docDirs, func(dir string) bool {
		return strings.HasPrefix(path, dir)
	})
}

// databases lists the files the scanners read packages from.
var databases = append(append([]string{statusPath, statusDPath, apkInstalledPath}, rpmdbPaths...), rpmdbWALs...)

// Fingerprint digests the size and modification time of the package databases in
// root, which every package change updates. It returns "" for a root it cannot open.
func (s *OSScanner) Fingerprint(root string) string {
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return ""
	}
	defer rootFS.Close()

	h := fnv.New64a()
	for _, db := range databases {
		if info, err := rootFS.Stat(strings.TrimSuffix(db, "/")); err == nil {
			fmt.Fprintf(h, "%s %d %d\n", db, info.Size(), info.ModTime().UnixNano())
		}
	}
	return strconv.FormatUint(h.Sum64(), 16)
}
