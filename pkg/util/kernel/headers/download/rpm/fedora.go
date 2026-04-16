// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package rpm

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2/backend"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2/repo"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

// FedoraBackend implements types.Backend for Fedora
type FedoraBackend struct {
	dnfBackend *backend.Backend
	logger     types.Logger
	target     *types.Target
}

// GetKernelHeaders downloads the headers to the provided directory.
func (b *FedoraBackend) GetKernelHeaders(directory string) error {
	for _, targetPackageName := range []string{"kernel-devel", "kernel-headers"} {
		pkgMatcher := dnfv2.DefaultPkgMatcher(targetPackageName, b.target.Uname.Kernel)

		pkg, data, err := b.dnfBackend.FetchPackage(pkgMatcher)
		if err != nil {
			b.logger.Errorf("fetch `%s` package: %v", targetPackageName, err)
			continue
		}

		return dnfv2.ExtractPackage(pkg, data, directory, b.target, b.logger)
	}
	return errors.New("no valid packages")
}

// Close releases resources.
func (b *FedoraBackend) Close() {}

// NewFedoraBackend creates a new Fedora backend
func NewFedoraBackend(target *types.Target, reposDir string, logger types.Logger) (*FedoraBackend, error) {
	b, err := dnfv2.NewBackend(target.Distro.Release, reposDir)
	if err != nil {
		return nil, err
	}

	const (
		updatesArchiveRepoBaseURL = "https://fedoraproject-updates-archive.fedoraproject.org/fedora/$releasever/$basearch/"
		updatesArchiveGpgKeyPath  = "file:///etc/pki/rpm-gpg/RPM-GPG-KEY-fedora-$releasever-$basearch"
	)

	// updates archive as a fallback
	b.AppendRepository(repo.Repo{
		Name:     "updates-archive",
		BaseURL:  updatesArchiveRepoBaseURL,
		Enabled:  true,
		GpgCheck: true,
		GpgKeys:  []string{updatesArchiveGpgKeyPath},
	})

	return &FedoraBackend{
		target:     target,
		logger:     logger,
		dnfBackend: b,
	}, nil
}
