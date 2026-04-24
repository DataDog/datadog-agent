// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package rpm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2/backend"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2/repo"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

// SLESBackend implements types.Backend for SLES
type SLESBackend struct {
	target        *types.Target
	flavour       string
	kernelRelease string
	logger        types.Logger
	dnfBackend    *backend.Backend
}

// GetKernelHeaders downloads the headers to the provided directory.
func (b *SLESBackend) GetKernelHeaders(directory string) error {
	pkgNevra := "kernel" + b.flavour + "-devel"
	packagesToInstall := []string{pkgNevra, "kernel-devel"}

	installedPackages := 0
	for _, targetPackageName := range packagesToInstall {
		pkgMatcher := func(pkg *repo.PkgInfoHeader) bool {
			return pkg.Name == targetPackageName &&
				b.kernelRelease == fmt.Sprintf("%s-%s", pkg.Version.Ver, pkg.Version.Rel) &&
				(pkg.Arch == b.target.Uname.Machine || pkg.Arch == "noarch")
		}
		pkg, data, err := b.dnfBackend.FetchPackage(pkgMatcher)
		if err != nil {
			b.logger.Errorf("fetch `%s` package: %w", pkgNevra, err)
			continue
		}

		if err := dnfv2.ExtractPackage(pkg, data, directory, b.target, b.logger); err != nil {
			b.logger.Errorf("extract `%s` package: %w", pkgNevra, err)
			continue
		}

		installedPackages++
	}

	if installedPackages == 0 {
		return errors.New("no valid packages")
	}
	return nil
}

// Close releases resources.
func (b *SLESBackend) Close() {
}

// NewSLESBackend creates a new SLES backend
func NewSLESBackend(target *types.Target, reposDir string, logger types.Logger) (types.Backend, error) {
	b, err := dnfv2.NewBackend(target.Distro.Release, reposDir)
	if err != nil {
		return nil, err
	}

	kernelRelease := target.Uname.Kernel
	flavour := "-generic"
	flavourIndex := strings.LastIndex(kernelRelease, "-")
	if flavourIndex != -1 {
		flavour = kernelRelease[flavourIndex:]
		kernelRelease = kernelRelease[:flavourIndex]
	}

	// On not registered systems, we use the repositories from
	// https://download.opensuse.org/repositories/Kernel:
	if version := target.OSRelease["VERSION"]; version != "" {
		addKernelRepository := func(version string) {
			versionSplit := strings.SplitN(version, "-", 2)
			version = "SLE" + version
			repoID := "Kernel_" + version
			subPath := "standard"
			if versionSplit[0] == "15" {
				subPath = "pool"
			}
			baseurl := fmt.Sprintf("https://download.opensuse.org/repositories/Kernel:/%s/%s/", version, subPath)
			gpgKey := fmt.Sprintf("https://download.opensuse.org/repositories/Kernel:/%s/%s/repodata/repomd.xml.key", version, subPath)

			b.AppendRepository(repo.Repo{
				Name:     repoID,
				BaseURL:  baseurl,
				Enabled:  true,
				GpgCheck: true,
				GpgKeys:  []string{gpgKey},
			})
		}

		addKernelRepository(version)
		addKernelRepository(version + "-UPDATES")
		if flavour != "-generic" {
			addKernelRepository(version + strings.ToUpper(flavour))
		}
	}

	// On SLES 15.2 without a subscription, the kernel headers can be found on the 'jump' repository
	if versionID := target.OSRelease["VERSION_ID"]; versionID != "" {
		repoID := "Jump-" + versionID
		baseurl := fmt.Sprintf("https://download.opensuse.org/distribution/jump/%s/repo/oss/", versionID)

		b.AppendRepository(repo.Repo{
			Name:    repoID,
			BaseURL: baseurl,
			Enabled: true,
		})
	}

	return &SLESBackend{
		target:        target,
		flavour:       flavour,
		kernelRelease: kernelRelease,
		logger:        logger,
		dnfBackend:    b,
	}, nil
}
