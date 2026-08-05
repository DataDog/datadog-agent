// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package rpm

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2/backend"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

// RockyBackend implements types.Backend for RedHat
type RockyBackend struct {
	dnfBackend *backend.Backend
	logger     types.Logger
	target     *types.Target
}

// GetKernelHeaders downloads the headers to the provided directory.
func (b *RockyBackend) GetKernelHeaders(directory string) error {
	pkgNevra := "kernel-devel"
	pkgMatcher := dnfv2.DefaultPkgMatcher(pkgNevra, b.target.Uname.Kernel)

	pkg, data, err := b.dnfBackend.FetchPackage(pkgMatcher)
	if err != nil {
		return fmt.Errorf("fetch `%s` package: %w", pkgNevra, err)
	}

	return dnfv2.ExtractPackage(pkg, data, directory, b.target, b.logger)
}

// Close releases resources.
func (b *RockyBackend) Close() {
}

// NewRockyBackend creates a new RedHat backend
func NewRockyBackend(target *types.Target, reposDir string, logger types.Logger) (*RockyBackend, error) {
	b, err := dnfv2.NewBackend(target.Distro.Release, reposDir)
	if err != nil {
		return nil, err
	}

	verNum, err := strconv.ParseFloat(target.Distro.Release, 64)
	if err == nil && verNum < 9.7 {
		for i := range b.Repositories {
			repo := &b.Repositories[i]
			if repo.MirrorList == "" {
				continue
			}
			mirrorURL, err := url.Parse(repo.MirrorList)
			if err != nil {
				continue
			}
			if mirrorURL.Host != "mirrors.rockylinux.org" {
				continue
			}

			arch := mirrorURL.Query().Get("arch")
			fullRepo := mirrorURL.Query().Get("repo")
			if arch == "" || fullRepo == "" {
				continue
			}
			parts := strings.Split(fullRepo, "-")
			if len(parts) < 2 {
				continue
			}
			repoName, ver := parts[0], parts[1]
			subrepo := "os"
			if len(parts) >= 3 {
				subrepo = parts[2]
			}
			mirrorURL.Host = "dl.rockylinux.org"
			switch subrepo {
			case "source":
				mirrorURL.Path = fmt.Sprintf("/vault/rocky/%s/%s/%s/tree", ver, repoName, subrepo)
			case "debug":
				mirrorURL.Path = fmt.Sprintf("/vault/rocky/%s/%s/%s/%s/tree", ver, repoName, arch, subrepo)
			default:
				mirrorURL.Path = fmt.Sprintf("/vault/rocky/%s/%s/%s/%s", ver, repoName, arch, subrepo)
			}
			mirrorURL.RawQuery = ""
			repo.BaseURL = mirrorURL.String()
			repo.MirrorList = ""
		}
	}

	return &RockyBackend{
		target:     target,
		logger:     logger,
		dnfBackend: b,
	}, nil
}
