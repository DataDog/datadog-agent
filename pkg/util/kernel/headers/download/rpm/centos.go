// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package rpm

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2/backend"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2/repo"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

// CentOSBackend implements types.Backend for CentOS
type CentOSBackend struct {
	dnfBackend *backend.Backend
	target     *types.Target
	logger     types.Logger
}

func getRedhatRelease() (string, error) {
	redhatReleasePath := types.HostEtc("redhat-release")
	redhatRelease, err := os.ReadFile(redhatReleasePath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", redhatReleasePath, err)
	}

	re := regexp.MustCompile(`.* release ([0-9.]*)`)
	submatches := re.FindStringSubmatch(string(redhatRelease))
	if len(submatches) == 2 {
		return submatches[1], nil
	}

	return "", fmt.Errorf("parse release from %s", redhatReleasePath)
}

// GetKernelHeaders downloads the headers to the provided directory.
func (b *CentOSBackend) GetKernelHeaders(directory string) error {
	pkgNevra := "kernel-devel"
	pkgMatcher := dnfv2.DefaultPkgMatcher(pkgNevra, b.target.Uname.Kernel)

	pkg, data, err := b.dnfBackend.FetchPackage(pkgMatcher)
	if err != nil {
		return fmt.Errorf("fetch `%s` package: %w", pkgNevra, err)
	}

	return dnfv2.ExtractPackage(pkg, data, directory, b.target, b.logger)
}

// Close releases resources.
func (b *CentOSBackend) Close() {}

// NewCentOSBackend creates a new CentOS backend
func NewCentOSBackend(target *types.Target, reposDir string, logger types.Logger) (*CentOSBackend, error) {
	release, err := getRedhatRelease()
	if err != nil {
		return nil, fmt.Errorf("detect CentOS release: %w", err)
	}

	version, _ := strconv.Atoi(strings.SplitN(release, ".", 2)[0])
	versionStr := strconv.Itoa(version)

	b, err := dnfv2.NewBackend(versionStr, reposDir)
	if err != nil {
		return nil, err
	}

	if version >= 8 {
		gpgKey := "file:///etc/pki/rpm-gpg/RPM-GPG-KEY-centosofficial"
		baseURL := fmt.Sprintf("http://vault.centos.org/%s/BaseOS/$basearch/os/", release)
		b.AppendRepository(repo.Repo{
			Name:     fmt.Sprintf("C%s-base", release),
			BaseURL:  baseURL,
			Enabled:  true,
			GpgCheck: true,
			GpgKeys:  []string{gpgKey},
		})
	} else {
		gpgKey := fmt.Sprintf("file:///etc/pki/rpm-gpg/RPM-GPG-KEY-CentOS-%d", version)
		baseURL := fmt.Sprintf("http://vault.centos.org/%s/os/$basearch/", release)
		updatesURL := fmt.Sprintf("http://vault.centos.org/%s/updates/$basearch/", release)
		b.AppendRepository(repo.Repo{
			Name:     fmt.Sprintf("C%s-base", release),
			BaseURL:  baseURL,
			Enabled:  true,
			GpgCheck: true,
			GpgKeys:  []string{gpgKey},
		})
		b.AppendRepository(repo.Repo{
			Name:     fmt.Sprintf("C%s-updates", release),
			BaseURL:  updatesURL,
			Enabled:  true,
			GpgCheck: true,
			GpgKeys:  []string{gpgKey},
		})
	}

	return &CentOSBackend{
		target:     target,
		logger:     logger,
		dnfBackend: b,
	}, nil
}
