// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

// OpenSUSEBackend implements types.Backend for OpenSUSE
type OpenSUSEBackend struct {
	target     *types.Target
	logger     types.Logger
	dnfBackend *backend.Backend
}

// GetKernelHeaders downloads the headers to the provided directory.
func (b *OpenSUSEBackend) GetKernelHeaders(directory string) error {
	kernelRelease := b.target.Uname.Kernel

	pkgNevra := "kernel"
	flavourIndex := strings.LastIndex(kernelRelease, "-")
	if flavourIndex != -1 {
		pkgNevra += kernelRelease[flavourIndex:]
		kernelRelease = kernelRelease[:flavourIndex]
	}
	pkgNevra += "-devel"

	packagesToInstall := []string{pkgNevra}
	if pkgNevra != "kernel-devel" {
		packagesToInstall = append(packagesToInstall, "kernel-devel")
	}

	installedPackages := 0
	for _, targetPackageName := range packagesToInstall {
		pkgMatcher := func(pkg *repo.PkgInfoHeader) bool {
			return pkg.Name == targetPackageName &&
				kernelRelease == fmt.Sprintf("%s-%s", pkg.Version.Ver, pkg.Version.Rel) &&
				(pkg.Arch == b.target.Uname.Machine || pkg.Arch == "noarch")
		}

		pkg, data, err := b.dnfBackend.FetchPackage(pkgMatcher)
		if err != nil {
			b.logger.Errorf("fetch `%s` package: %v", targetPackageName, err)
			continue
		}

		if err := dnfv2.ExtractPackage(pkg, data, directory, b.target, b.logger); err != nil {
			b.logger.Errorf("extract `%s` package: %v", targetPackageName, err)
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
func (b *OpenSUSEBackend) Close() {}

// NewOpenSUSEBackend creates a new OpenSUSE backend
func NewOpenSUSEBackend(target *types.Target, reposDir string, logger types.Logger) (types.Backend, error) {
	b, err := dnfv2.NewBackend(target.Distro.Release, reposDir)
	if err != nil {
		return nil, err
	}

	return &OpenSUSEBackend{
		target:     target,
		logger:     logger,
		dnfBackend: b,
	}, nil
}
