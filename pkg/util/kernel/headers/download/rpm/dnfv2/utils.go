// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dnfv2

import (
	"fmt"
	"os"
	"path"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/extract"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2/backend"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2/repo"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

// NewBackend creates a DNFv2 Backend
func NewBackend(release string, reposDir string) (*backend.Backend, error) {
	builtinVars, err := computeBuiltinVariables(release)
	if err != nil {
		return nil, fmt.Errorf("compute DNF builting variables: %w", err)
	}

	varsDir := []string{"/etc/dnf/vars/", "/etc/yum/vars/"}
	b, err := backend.NewBackend(reposDir, varsDir, builtinVars)
	if err != nil {
		return nil, fmt.Errorf("create fedora dnf backend: %w", err)
	}

	return b, nil
}

func computePkgKernel(pkg *repo.PkgInfoHeader) string {
	return fmt.Sprintf("%s-%s.%s", pkg.Version.Ver, pkg.Version.Rel, pkg.Arch)
}

// DefaultPkgMatcher matches packages based on name, version, and arch
func DefaultPkgMatcher(pkgName, kernelVersion string) repo.PkgMatchFunc {
	return func(pkg *repo.PkgInfoHeader) bool {
		return pkg.Name == pkgName && kernelVersion == computePkgKernel(pkg)
	}
}

// ExtractPackage extracts the RPM package described by `pkg` and contained in `data` to the provided directory
func ExtractPackage(pkg *repo.PkgInfo, data []byte, directory string, target *types.Target, logger types.Logger) error {
	pkgFileName := fmt.Sprintf("%s-%s.rpm", pkg.Header.Name, computePkgKernel(&pkg.Header))
	pkgFileName = path.Join(directory, pkgFileName)
	if err := os.WriteFile(pkgFileName, data, 0o644); err != nil {
		return err
	}

	return extract.ExtractRPMPackage(pkgFileName, directory, target.Uname.Kernel, logger)
}
