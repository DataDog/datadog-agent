// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package apt is a backend for APT
package apt

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aptly-dev/aptly/aptly"
	"github.com/aptly-dev/aptly/deb"
	"github.com/aptly-dev/aptly/http"
	"github.com/aptly-dev/aptly/pgp"
	"github.com/aptly-dev/aptly/utils"
	"github.com/xor-gate/ar"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/extract"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

// Backend implements types.Backend for APT
type Backend struct {
	target         *types.Target
	logger         types.Logger
	repoCollection []remoteRepo
}

// Close releases resources.
func (b *Backend) Close() {}

func (b *Backend) extractPackage(pkg, directory string) error {
	f, err := os.Open(pkg)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := ar.NewReader(f)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("decompress deb: %w", err)
		}
		b.logger.Debugf("Found header: %s", header.Name)

		if strings.HasPrefix(header.Name, "data.tar") {
			return extract.ExtractTarball(reader, header.Name, directory, b.logger)
		}
	}

	return errors.New("decompress deb: data.tar not found")
}

func (b *Backend) downloadPackage(downloader aptly.Downloader, verifier pgp.Verifier, query *deb.FieldQuery, directory string) (*deb.PackageDependencies, error) {
	var packageURL *url.URL
	var packageDeps *deb.PackageDependencies
	var checksums utils.ChecksumInfo

	stanza := make(deb.Stanza, 32)

	for _, repoInfo := range b.repoCollection {
		repo, err := deb.NewRemoteRepo(repoInfo.repoID, repoInfo.uri, repoInfo.distribution, repoInfo.components, []string{repoInfo.arch}, false, false, false, false)
		if err != nil {
			b.logger.Errorf("Failed to create remote repo: %s", err)
			continue
		}

		b.logger.Debugf("Fetching repository: name=%s, distribution=%s, components=%v, arch=%v", repo.Name, repo.Distribution, repo.Components, repo.Architectures)
		repo.SkipComponentCheck = true

		stanza.Clear()
		if err := repo.FetchBuffered(stanza, downloader, verifier, false); err != nil {
			b.logger.Debugf("Error fetching repo: %s", err)
			// not every repo has to be successful
			continue
		}

		b.logger.Debug("Downloading package indexes")
		// factory is not used by DownloadPackageIndexes so we can use nil here
		var factory *deb.CollectionFactory
		if err := repo.DownloadPackageIndexes(nil, downloader, nil, factory, false, false); err != nil {
			b.logger.Debugf("Failed to download package indexes: %s", err)
			// not every repo has to be successful
			continue
		}

		_, _, err = repo.ApplyFilter(-1, query, nil)
		if err != nil {
			b.logger.Debugf("Failed to apply filter: %s", err)
			// not every repo has to be successful
			continue
		}

		/*
			// For some reason, this overrides the `downloadPath` field of package so we don't
			// have the full remote path of the package. As a workaround, aptly was patched to
			// expose the repository package list using RemoteRepo.PackageList()

			if err := repo.FinalizeDownload(collectionFactory, progress); err != nil {
				return errors.Wrap(err, "finalize download")
			}

			refList := repo.RefList()
			packageList, err := deb.NewPackageListFromRefList(refList, collectionFactory.PackageCollection(), progress)
			if err != nil {
				return err
			}
		*/

		packageList := repo.PackageList()
		// A repository without a usable package must not prevent trying others.
		_ = packageList.ForEach(func(pkg *deb.Package) error {
			b.logger.Infof("Found package %s with version %s", pkg.Name, pkg.Version)

			packageFiles := pkg.Files()
			if len(packageFiles) == 0 {
				return errors.New("No package file for " + pkg.Name)
			}

			packageURL = repo.PackageURL(packageFiles[0].DownloadURL())
			checksums = packageFiles[0].Checksums
			packageDeps = pkg.Deps()
			b.logger.Infof("Package URL: %s", packageURL)
			return nil
		})

		// if we set packageURL we can exit the loop
		if packageURL != nil {
			break
		}
	}

	if packageURL == nil {
		return nil, errors.New("find package " + query.Value)
	}

	b.logger.Info("Downloading package")
	return packageDeps, b.downloadAndExtract(downloader, packageURL.String(), checksums, directory)
}

// downloadAndExtract authenticates package bytes against the signed repository
// metadata before allowing any archive content to reach the extraction directory.
func (b *Backend) downloadAndExtract(downloader aptly.Downloader, purl string, checksums utils.ChecksumInfo, directory string) error {
	validHash := func(value string, size int) bool {
		decoded, err := hex.DecodeString(value)
		return err == nil && len(decoded) == size
	}
	if checksums.Size < 0 || !(validHash(checksums.SHA256, 32) || validHash(checksums.SHA512, 64)) {
		return fmt.Errorf("package %s has no valid authenticated size and SHA-256/SHA-512 checksum", purl)
	}
	tmp, err := os.MkdirTemp(directory, ".download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	outputFile := filepath.Join(tmp, "package.deb")
	if err := downloader.DownloadWithChecksum(context.Background(), purl, outputFile, &checksums, false); err != nil {
		return fmt.Errorf("download %s to %s: %w", purl, directory, err)
	}
	return b.extractPackage(outputFile, directory)
}

func (b *Backend) createGpgVerifier() (*pgp.GoVerifier, error) {
	gpgVerifier := &pgp.GoVerifier{}

	for _, searchPattern := range []string{types.HostEtc("apt", "trusted.gpg"), types.HostEtc("apt", "trusted.gpg.d", "*.gpg"), "/usr/share/keyrings/*.gpg"} {
		keyrings, err := filepath.Glob(searchPattern)
		if err != nil {
			return nil, fmt.Errorf("find valid apt keyrings: %w", err)
		}
		for _, keyring := range keyrings {
			b.logger.Infof("Adding keyring from: %s", keyring)
			gpgVerifier.AddKeyring(keyring)
		}
	}

	if err := gpgVerifier.InitKeyring(false); err != nil {
		return nil, err
	}
	return gpgVerifier, nil
}

// GetKernelHeaders downloads the headers to the provided directory.
func (b *Backend) GetKernelHeaders(directory string) error {
	downloader := http.NewDownloader(0, 1, nil)

	gpgVerifier, err := b.createGpgVerifier()
	if err != nil {
		return err
	}

	kernelRelease := b.target.Uname.Kernel
	query := &deb.FieldQuery{
		Field:    "Name",
		Relation: deb.VersionPatternMatch,
		Value:    "linux-headers-" + kernelRelease,
	}
	b.logger.Infof("Looking for %s", query.Value)

	dependencies, err := b.downloadPackage(downloader, gpgVerifier, query, directory)
	if err != nil {
		return err
	}

	// Sometimes, the header package depends on other header packages
	// If this is the case, download the dependency in addition
	if dependencies != nil {
		for _, dep := range dependencies.Depends {
			if strings.Contains(dep, "linux") && strings.Contains(dep, "headers") {

				depName := strings.Split(dep, " ")[0]
				b.logger.Infof("Looking for dependency %s", dep)
				query = &deb.FieldQuery{
					Field:    "Name",
					Relation: deb.VersionPatternMatch,
					Value:    depName,
				}

				_, err = b.downloadPackage(downloader, gpgVerifier, query, directory)
				if err != nil {
					// Propagate verification and extraction failures so the download attempt cleans up partial headers.
					return fmt.Errorf("download dependent package %s: %w", depName, err)
				}
			}
		}
	}

	return nil
}

type remoteRepo struct {
	repoID       string
	uri          string
	distribution string
	components   []string
	arch         string
}

// NewBackend creates a backend for APT
func NewBackend(target *types.Target, aptConfigDir string, logger types.Logger) (*Backend, error) {
	var debArch string
	switch target.Uname.Machine {
	case "x86_64":
		debArch = "amd64"
	case "i386", "i686":
		debArch = "i386"
	case "aarch64":
		debArch = "arm64"
	case "s390":
		debArch = "s390"
	case "s390x":
		debArch = "s390x"
	case "ppc64le":
		debArch = "ppc64el"
	case "mips64el":
		debArch = "mips64el"
	default:
		return nil, fmt.Errorf("unsupported architecture '%s'", target.Uname.Machine)
	}

	backend := &Backend{
		target: target,
		logger: logger,
	}

	repoList, err := parseAPTConfigFolder(aptConfigDir)
	if err != nil {
		return nil, fmt.Errorf("parse APT folder: %w", err)
	}

	addRepo := func(rr remoteRepo) {
		backend.repoCollection = append(backend.repoCollection, rr)
		backend.logger.Debugf("Added repository '%s' %s %s %v %v", rr.repoID, rr.uri, rr.distribution, rr.components, rr.arch)
	}
	var snapshotRepos []remoteRepo
	for i, repo := range repoList {
		if !repo.Enabled || repo.SourceRepo {
			continue
		}
		prefix := target.Distro.Display
		repoID := fmt.Sprintf("%s-%d", prefix, i)
		var components []string
		if repo.Components != "" {
			components = strings.Split(repo.Components, " ")
		}

		if isSignedByUnreachableKey(repo) {
			backend.logger.Debugf("Skipping repo '%s' %s %s %v: unreachable key", repoID, repo.URI, repo.Distribution, components)
			continue
		}

		rr := remoteRepo{
			repoID:       repoID,
			uri:          overrideRepoURI(repo.URI, target),
			distribution: repo.Distribution,
			components:   components,
			arch:         debArch,
		}

		options := strings.Split(repo.Options, " ")
		for _, opt := range options {
			optName, optValue, found := strings.Cut(opt, "=")
			if !found {
				continue
			}
			if strings.ToLower(optName) == "arch" {
				rr.arch = optValue
				break
			}
		}

		addRepo(rr)

		if uri := getSnapshotRepoURI(repo.URI, target); uri != "" {
			for _, distribution := range getSnapshotDistributions(rr.distribution, target) {
				snapshotRepo := rr
				snapshotRepo.repoID = rr.repoID + "-snapshot-" + distribution
				snapshotRepo.uri = uri
				snapshotRepo.distribution = distribution
				snapshotRepos = append(snapshotRepos, snapshotRepo)
			}
		}
	}
	for _, rr := range snapshotRepos {
		addRepo(rr)
	}

	return backend, nil
}

func isOfficialDebianRepo(parsedURL *url.URL, target *types.Target) bool {
	return strings.ToLower(target.Distro.Display) == "debian" &&
		(parsedURL.Host == "deb.debian.org" || parsedURL.Host == "security.debian.org")
}

func overrideRepoURI(uri string, target *types.Target) string {
	parsedURL, err := url.Parse(uri)
	if err != nil || !isOfficialDebianRepo(parsedURL, target) {
		return uri
	}

	switch target.OSRelease["VERSION_ID"] {
	case "9", "10":
		parsedURL.Host = "archive.debian.org"
		return parsedURL.String()
	}

	return uri
}

// Debian kernels end their uname -v with their package version and changelog date, e.g.
// "#1 SMP PREEMPT_DYNAMIC Debian 6.1.148-1 (2025-08-26)".
var debianKernelBuildRegexp = regexp.MustCompile(`Debian \S+ \(([0-9]{4}-[0-9]{2}-[0-9]{2})\)$`)

// getSnapshotRepoURI returns the snapshot.debian.org copy of an official Debian repository two
// weeks after the build of the running kernel, since Debian drops the headers of superseded kernel
// ABIs from its mirrors.
func getSnapshotRepoURI(uri string, target *types.Target) string {
	parsedURL, err := url.Parse(uri)
	if err != nil || !isOfficialDebianRepo(parsedURL, target) {
		return ""
	}

	m := debianKernelBuildRegexp.FindStringSubmatch(target.Uname.Version)
	if m == nil {
		return ""
	}
	built, err := time.Parse(time.DateOnly, m[1])
	if err != nil {
		return ""
	}

	// by then, the kernel upload is listed in the release, proposed-updates, or security suite
	timestamp := built.AddDate(0, 0, 14).Format("20060102T150405Z")
	parsedURL.Host = "snapshot.debian.org"
	parsedURL.Path = path.Join("/archive", parsedURL.Path, timestamp) + "/"
	return parsedURL.String()
}

// getSnapshotDistributions returns the snapshot suites of a configured one, named after the host
// release since an alias like oldstable may designate another release at the snapshot time.
func getSnapshotDistributions(distribution string, target *types.Target) []string {
	codename := target.OSRelease["VERSION_CODENAME"]
	if codename == "" || target.OSRelease["VERSION_ID"] == "" { // testing and sid have no VERSION_ID
		return nil
	}

	if i := strings.IndexAny(distribution, "-/"); i >= 0 {
		return []string{codename + distribution[i:]}
	}
	// stable updates first appear in proposed-updates, until the next point release
	return []string{codename, codename + "-proposed-updates"}
}

func isSignedByUnreachableKey(repo *repository) bool {
	if repo.Options == "" {
		return false
	}

	options := strings.Split(repo.Options, " ")
	for _, opt := range options {
		optName, optValue, found := strings.Cut(opt, "=")
		if !found {
			continue
		}

		if strings.ToLower(optName) == "signed-by" {
			// if the key is not in `/etc/*` or `/usr/share/keyrings/*` then we cannot reach it
			if !strings.HasPrefix(optValue, "/etc") && !strings.HasPrefix(optValue, "/usr/share/keyrings") {
				return true
			}
		}
	}

	return false
}
