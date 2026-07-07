// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package bootstrap provides logic to self-bootstrap the installer.
package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
)

func install(ctx context.Context, env *env.Env, url string, experiment bool) error {
	err := os.MkdirAll(paths.RootTmpDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	tmpDir, err := os.MkdirTemp(paths.RootTmpDir, "")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	cmd, err := downloadInstaller(ctx, env, url, tmpDir)
	if err != nil {
		return installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			err,
		)
	}
	if experiment {
		return cmd.InstallExperiment(ctx, url)
	}
	return cmd.Install(ctx, url, nil)
}

// extractInstallerFromOCI downloads the installer binary from the agent package in the registry and returns an installer executor
func downloadInstaller(ctx context.Context, env *env.Env, url string, tmpDir string) (*exec.InstallerExec, error) {
	downloader := oci.NewDownloader(env, env.HTTPClient())
	downloadedPackage, err := downloader.Download(ctx, url)
	if err != nil {
		return nil, installerErrors.Wrap(
			installerErrors.ErrDownloadFailed,
			fmt.Errorf("could not download package: %w", err),
		)
	}
	if downloadedPackage.Name != AgentPackage {
		return getLocalInstaller(env)
	}

	installerBinPath := filepath.Join(tmpDir, "installer")
	err = downloadedPackage.ExtractLayers(oci.DatadogPackageInstallerLayerMediaType, installerBinPath) // Returns nil if the layer doesn't exist
	if err != nil {
		return nil, fmt.Errorf("failed to extract layers: %w", err)
	}
	if _, err := os.Stat(installerBinPath); err != nil {
		return nil, err
	}
	// The installer extracted from the agent package's installer layer may be a
	// FIPS-flavor build (compiled with requirefips). Such binaries panic at init
	// unless the embedded OpenSSL FIPS provider has been self-tested and a
	// matching libcrypto is loaded. Point the child at the running process's own
	// embedded FIPS tree so it finds a compatible, configured provider.
	//
	// We detect this by checking whether the running binary's tree has the FIPS
	// provider files rather than relying on a build-time flag, because the daemon
	// binary that calls us may be a deb-installed binary whose build system did
	// not set the goexperiment.systemcrypto flag even though the tree is FIPS.
	//
	// NOTE: Version-skew assumption. The bootstrap installer.layer is pointed at the
	// *running daemon's* embedded FIPS tree (OPENSSL_CONF, OPENSSL_MODULES, LD_LIBRARY_PATH)
	// rather than the experiment tree being downloaded. This means the installer.layer starts
	// with the stable tree's libcrypto + fips.so + fipsmodule.cnf.
	//
	// This is intentional: under AT_SECURE (see datadog-agent-finalize.rb) LD_LIBRARY_PATH
	// is ignored, so the bootstrap installer must find libcrypto via its hardcoded RPATH
	// (/opt/datadog-agent/embedded/lib). Pointing OPENSSL_CONF/MODULES at the same stable
	// tree ensures consistency (libcrypto and fips.so are the same build).
	//
	// Version-skew risk: if the experiment ships a libcrypto that is ABI-incompatible with
	// the stable fips.so, the bootstrap installer's requirefips init would fail. OpenSSL 3.x
	// maintains ABI compatibility within the major soname (libcrypto.so.3), so this is
	// acceptable for now. If a future release bumps the soname or breaks ABI, this code must
	// be updated to point the bootstrap installer at its own embedded tree.
	extraEnv := fipsEnvFromRunningInstaller()
	return exec.NewInstallerExecWithExtraEnv(env, installerBinPath, extraEnv), nil
}

// fipsEnvFromRunningInstaller returns the OpenSSL FIPS provider environment
// that a requirefips bootstrap installer binary needs to start on this machine.
// It derives the provider paths from the running binary's own embedded tree
// (OPENSSL_CONF, OPENSSL_MODULES, LD_LIBRARY_PATH) so the child loads the
// version-matched libcrypto and fips.so from the same tree rather than the
// host's system OpenSSL (which may be a different version and fail the FIPS
// self-test). Returns nil when the running binary's tree does not have a FIPS
// provider (non-FIPS install), leaving library resolution to the child's
// defaults — a no-op for non-requirefips binaries.
func fipsEnvFromRunningInstaller() []string {
	exePath, err := exec.GetExecutable()
	if err != nil {
		return nil
	}
	// <install>/embedded/bin/installer → <install>/embedded
	embedded := filepath.Dir(filepath.Dir(exePath))
	return env.FIPSProviderEnv(embedded)
}

func getInstallerOCI(_ context.Context, env *env.Env) (string, error) {
	version := "latest"
	if env.DefaultPackagesVersionOverride[InstallerPackage] != "" {
		version = env.DefaultPackagesVersionOverride[InstallerPackage]
	}
	return oci.PackageURL(env, InstallerPackage, version), nil
}
