// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package collectorv2 holds sbom related files
package collectorv2

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	sbomtypes "github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/types"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

const apkInstalledPath = "lib/apk/db/installed"

type apkScanner struct{}

func (s *apkScanner) Name() string {
	return "apk"
}

func (s *apkScanner) ListPackages(_ context.Context, root *os.Root) ([]sbomtypes.PackageWithInstalledFiles, error) {
	f, err := root.Open(apkInstalledPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open apk database (%s): %w", apkInstalledPath, err)
	}
	defer f.Close()

	pkgs, err := parseAPKDatabase(f)
	if err != nil {
		return nil, fmt.Errorf("failed to parse apk database: %w", err)
	}
	return pkgs, nil
}

type apkPackage struct {
	name    string
	version string
	epoch   int
	release string
	files   []string
}

func parseAPKDatabase(r io.Reader) ([]sbomtypes.PackageWithInstalledFiles, error) {
	var packages []sbomtypes.PackageWithInstalledFiles
	var current apkPackage
	var currentDir string

	finalize := func() {
		if current.name == "" || current.version == "" {
			return
		}
		packages = append(packages, sbomtypes.PackageWithInstalledFiles{
			Package: sbomtypes.Package{
				Name:       current.name,
				Version:    current.version,
				Epoch:      current.epoch,
				Release:    current.release,
				SrcVersion: current.version,
				SrcEpoch:   current.epoch,
				SrcRelease: current.release,
			},
			InstalledFiles: current.files,
		})
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			finalize()
			current = apkPackage{}
			currentDir = ""
			continue
		}

		if len(line) < 2 || line[1] != ':' {
			seclog.Warnf("apk: unexpected line format: %q", line)
			continue
		}

		key := line[0]
		value := line[2:]

		switch key {
		case 'P':
			current.name = value
		case 'V':
			current.epoch, current.version, current.release = parseAPKVersion(value)
		case 'F':
			currentDir = value
		case 'R':
			current.files = append(current.files, "/"+filepath.Join(currentDir, value))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan apk database: %w", err)
	}

	// Handle the last package if the file does not end with a blank line
	finalize()

	return packages, nil
}

// parseAPKVersion parses an APK version string (e.g. "1.2.3-r4" or "2:1.2.3-r4")
// and returns (epoch, version, release). The release is the Alpine-specific "-rN" suffix.
func parseAPKVersion(v string) (epoch int, version, release string) {
	// Epoch prefix: "2:1.2.3-r4"
	if i := strings.IndexByte(v, ':'); i != -1 {
		if e, err := strconv.Atoi(v[:i]); err == nil {
			epoch = e
		}
		v = v[i+1:]
	}

	// Release suffix: last "-r<N>" component
	if i := strings.LastIndex(v, "-r"); i != -1 {
		release = v[i+1:] // "rN"
		version = v[:i]
	} else {
		version = v
	}

	return epoch, version, release
}
