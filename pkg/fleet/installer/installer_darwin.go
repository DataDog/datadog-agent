// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package installer

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/macpkg"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// installExperiment installs a per-version .pkg as the experiment.
//
// It is a peer of the OCI flow rather than a branch inside it, because almost nothing is shared.
// The OCI flow downloads an image, extracts its layers into a temporary directory and hands that
// directory to the repository to move into place. Here the payload is a .pkg, and only
// installer(8) may unpack one: its -target names a volume, not a directory, so the destination is
// baked into the package at build time and the payload is already inside the pool by the time this
// process gets control again. What is left for the installer to do is decide whether the payload
// is usable and then give it a name.
func (i *installerImpl) installExperiment(ctx context.Context, packageURL string) error {
	pkgName, version, err := parsePackageRef(packageURL)
	if err != nil {
		return installerErrors.Wrap(installerErrors.ErrDownloadFailed, err)
	}
	repository := i.packages.Get(pkgName)
	state, err := repository.GetState()
	if err != nil {
		return installerErrors.Wrap(installerErrors.ErrFilesystemIssue, fmt.Errorf("could not read the state of %s: %w", pkgName, err))
	}
	if !state.HasStable() {
		return fmt.Errorf("cannot start an experiment for %s: it is not installed", pkgName)
	}
	if state.Stable == version {
		return fmt.Errorf("cannot start an experiment for %s: %s is already the stable version", pkgName, version)
	}

	check := macpkg.NewPayloadCheck()
	if err := i.materializeVersion(ctx, check, repository, pkgName, packageURL, version); err != nil {
		return err
	}

	if err := i.hooks.PreStartExperiment(ctx, pkgName); err != nil {
		return fmt.Errorf("could not install experiment: %w", err)
	}
	// AdoptExperiment rather than SetExperiment: there is nothing to move, and SetExperiment's
	// leading cleanup would delete the payload that was just installed, because a version no
	// link names is by definition garbage.
	if err := repository.AdoptExperiment(ctx, version); err != nil {
		return installerErrors.Wrap(installerErrors.ErrFilesystemIssue, fmt.Errorf("could not set experiment: %w", err))
	}
	if err := i.config.RemoveExperiment(ctx); err != nil {
		return fmt.Errorf("could not remove config experiment: %w", err)
	}
	if err := i.hooks.PostStartExperiment(ctx, pkgName); err != nil {
		return fmt.Errorf("could not install experiment: %w", err)
	}
	return nil
}

// materializeVersion makes sure the version's payload is in the pool and complete.
//
// A version whose directory is already there and passes the completeness check is reused rather
// than reinstalled. That is the whole reason the check exists as a separate thing from the
// install: the common case for a reused version is a revert followed by the backend retrying the
// same version, or a promote that was interrupted, and reinstalling would mean re-downloading
// several hundred megabytes and re-running the system installer to arrive at the tree that is
// already on the disk. Anything less than complete is reinstalled, not repaired: a partial tree
// has no provenance, and the only thing that can produce a correct one is the package it came
// from.
func (i *installerImpl) materializeVersion(ctx context.Context, check macpkg.PayloadCheck, repository packageRepository, pkgName string, packageURL string, version string) error {
	present, err := repository.HasVersion(version)
	if err != nil {
		return installerErrors.Wrap(installerErrors.ErrFilesystemIssue, err)
	}
	if present {
		complete, err := check.Complete(version)
		if err != nil {
			return installerErrors.Wrap(installerErrors.ErrFilesystemIssue, err)
		}
		if complete {
			log.Infof("Reusing the %s payload already in the pool for %s", version, pkgName)
			return nil
		}
		log.Warnf("The %s payload in the pool is incomplete, reinstalling it", version)
	}

	downloader := macpkg.NewDownloader(i.env, i.env.HTTPClient())
	pkg, err := downloader.Download(ctx, packageURL, version)
	if err != nil {
		return installerErrors.Wrap(installerErrors.ErrDownloadFailed, fmt.Errorf("could not download package: %w", err))
	}
	defer func() {
		if err := pkg.Cleanup(); err != nil {
			log.Warnf("Could not clean up after the download: %v", err)
		}
	}()

	// The package is verified before it is handed to installer(8), which runs its contents as
	// root. The expected digest is empty here because the task names a URL and not a digest;
	// the signature and notarization checks are what the trust decision actually rests on, and
	// they are pinned to Datadog's team identifier rather than to Apple's approval alone.
	if err := macpkg.NewVerifier().Verify(ctx, pkg, ""); err != nil {
		return installerErrors.Wrap(installerErrors.ErrDownloadFailed, fmt.Errorf("could not verify package: %w", err))
	}

	if err := (macpkg.SystemInstaller{}).Install(ctx, pkg.Path); err != nil {
		return installerErrors.Wrap(installerErrors.ErrFilesystemIssue, fmt.Errorf("could not install package: %w", err))
	}

	// The completeness check is the gate on the link move, and it is asked after the install
	// rather than trusted from installer(8)'s exit status: the system installer reports on
	// whether it could run the package, not on whether the tree it produced is one the Agent
	// can start under.
	complete, err := check.Complete(version)
	if err != nil {
		return installerErrors.Wrap(installerErrors.ErrFilesystemIssue, err)
	}
	if !complete {
		return installerErrors.Wrap(
			installerErrors.ErrFilesystemIssue,
			fmt.Errorf("the package installed for %s did not produce a complete payload at %s", version, check.VersionPath(version)),
		)
	}
	if err := check.RecordDigest(version, pkg.Digest); err != nil {
		// The digest is provenance, not correctness: the payload is complete either way, and
		// the cost of a missing record is one reinstall of a version that would have been
		// reused.
		log.Warnf("Could not record the installed digest for %s: %v", version, err)
	}
	return nil
}

// packageRepository is the slice of repository.Repository this flow uses. It is an interface so
// materializeVersion can be tested without a pool on disk.
type packageRepository interface {
	HasVersion(name string) (bool, error)
}

// parsePackageRef recovers the package name and version from the URL a task names.
//
// On macOS the version has to be known before anything is downloaded, because the artifact is
// addressed by version: there is no index to resolve and no manifest to read, since the OCI index
// is selected by GOOS and carries no darwin image. The URL's tag is the version -- it is what
// oci.PackageURL puts there -- and the repository name is the package.
func parsePackageRef(packageURL string) (pkgName string, version string, err error) {
	parsed, err := url.Parse(packageURL)
	if err != nil {
		return "", "", fmt.Errorf("could not parse %q: %w", packageURL, err)
	}
	ref := parsed.Opaque
	if ref == "" {
		ref = parsed.Host + parsed.Path
	}
	ref = strings.TrimSuffix(ref, "/")
	if ref == "" {
		return "", "", fmt.Errorf("%q names no package", packageURL)
	}
	last := path.Base(ref)
	// The digest check comes before the cut, not after it. In name@sha256:hex the "@" is on the
	// left of the first colon, so a check on the tag half sees a bare hex string and takes it
	// for a version -- and the payload would then be installed into a pool directory named by
	// the digest, which every later link move would treat as a version.
	if strings.Contains(last, "@") {
		return "", "", fmt.Errorf("%q is addressed by digest, which does not name a version", packageURL)
	}
	name, tag, found := strings.Cut(last, ":")
	if !found || tag == "" {
		return "", "", fmt.Errorf("%q does not name a version: a macOS experiment is addressed by version, so the URL must carry one", packageURL)
	}
	// "agent-package:7.99.1" is the datadog-agent package. The mapping is the inverse of
	// oci.PackageURL's, which is the only thing that produces these URLs.
	name = strings.TrimSuffix(name, "-package")
	if name == "" {
		return "", "", fmt.Errorf("%q names no package", packageURL)
	}
	if !strings.HasPrefix(name, "datadog-") {
		name = "datadog-" + name
	}
	return name, tag, nil
}
