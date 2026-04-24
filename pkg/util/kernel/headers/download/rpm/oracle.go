// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package rpm

import (
	"errors"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2/backend"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

// OracleBackend implements types.Backend for Oracle
type OracleBackend struct {
	dnfBackend *backend.Backend
	logger     types.Logger
	target     *types.Target
}

// GetKernelHeaders downloads the headers to the provided directory.
func (b *OracleBackend) GetKernelHeaders(directory string) error {
	for _, targetPackageName := range []string{"kernel-devel", "kernel-uek-devel"} {
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
func (b *OracleBackend) Close() {
}

// NewOracleBackend creates a new Oracle backend
func NewOracleBackend(target *types.Target, reposDir string, logger types.Logger) (*OracleBackend, error) {
	b, err := dnfv2.NewBackend(target.Distro.Release, reposDir)
	if err != nil {
		return nil, err
	}

	uekRepoPattern := regexp.MustCompile(`^ol\d_UEK.*`)

	// force enable UEK repos
	for i := range b.Repositories {
		repo := &b.Repositories[i]

		if uekRepoPattern.MatchString(repo.Name) {
			repo.Enabled = true
		}
	}

	return &OracleBackend{
		target:     target,
		logger:     logger,
		dnfBackend: b,
	}, nil
}
