// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package macpkg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// requiredPayloadPaths are the paths a version directory must contain to be usable, relative to
// the version directory itself.
//
// The list is not a description of the package; it is the union of what the façades and the
// launchd job definitions resolve through. A version missing any of them is a version at least
// one job cannot start under, so naming it as the experiment would produce a host with a link
// pointing at something that cannot run.
//
// TestRequiredPayloadPathsCoverEveryJob pins this against the embedded definitions, so a job
// added with a new program path fails a test rather than a machine.
var requiredPayloadPaths = []string{
	filepath.Join("bin", "agent", "agent"),
	filepath.Join("embedded", "bin", "installer"),
	filepath.Join("embedded", "bin", "system-probe"),
	filepath.Join("embedded", "bin", "agent-data-plane"),
}

// PayloadCheck decides whether a version directory in the pool is complete.
//
// It is the gate on the link move. Writing straight into the version directory with no staging
// copy is safe not because the write is atomic -- it is not -- but because an incomplete tree has
// no name: nothing points at it until this check passes.
type PayloadCheck struct {
	// Root is the package's directory in the pool, e.g.
	// /opt/datadog-packages/datadog-agent. Empty means the default pool location.
	Root string
}

// NewPayloadCheck returns a PayloadCheck over the Agent's pool directory.
func NewPayloadCheck() PayloadCheck {
	return PayloadCheck{Root: filepath.Join(paths.PackagesPath, "datadog-agent")}
}

// VersionPath returns the pool directory the given version is installed at.
func (c PayloadCheck) VersionPath(version string) string {
	root := c.Root
	if root == "" {
		root = filepath.Join(paths.PackagesPath, "datadog-agent")
	}
	return filepath.Join(root, version)
}

// Complete reports whether every required path is present in the version's directory.
//
// A missing path is not an error: it is the answer. An error is returned only when the question
// could not be answered, so a caller cannot mistake "the disk is unreadable" for "the payload is
// incomplete".
func (c PayloadCheck) Complete(version string) (bool, error) {
	if version == "" {
		return false, errors.New("cannot check the payload of an unnamed version")
	}
	versionPath := c.VersionPath(version)
	info, err := os.Stat(versionPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("could not stat %s: %w", versionPath, err)
	}
	if !info.IsDir() {
		return false, nil
	}
	for _, required := range requiredPayloadPaths {
		// Stat, not Lstat: these paths are resolved by launchd, which follows symlinks, so a
		// link to a missing target is as incomplete as an absent file.
		if _, err := os.Stat(filepath.Join(versionPath, required)); err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("could not stat %s in %s: %w", required, versionPath, err)
		}
	}
	return true, nil
}

// digestFileName records which .pkg produced a version directory.
//
// It is written into the version directory rather than kept in the installer's database because
// the two must be discarded together: reclaiming a pool version and forgetting its digest is one
// operation, and a database that outlived a directory would claim a version was installed from a
// package whose payload is gone.
const digestFileName = ".installed-digest"

// RecordDigest records the digest of the .pkg a version directory was installed from.
func (c PayloadCheck) RecordDigest(version string, digest string) error {
	if version == "" || digest == "" {
		return errors.New("cannot record a digest for an unnamed version or an unnamed digest")
	}
	path := filepath.Join(c.VersionPath(version), digestFileName)
	if err := os.WriteFile(path, []byte(digest), 0644); err != nil {
		return fmt.Errorf("could not record the installed digest at %s: %w", path, err)
	}
	return nil
}

// Digest returns the digest recorded for a version, or the empty string when none is recorded.
//
// An absent record is not an error. It is what a version installed by an older Agent, or by the
// .dmg, looks like -- and the answer it gives is "this payload's provenance is unknown", which is
// a reason to reinstall rather than a reason to fail.
func (c PayloadCheck) Digest(version string) (string, error) {
	content, err := os.ReadFile(filepath.Join(c.VersionPath(version), digestFileName))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("could not read the installed digest of %s: %w", version, err)
	}
	return strings.TrimSpace(string(content)), nil
}
