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
	// Additionally: the binary's RPATH contains a $ORIGIN-relative entry. Under
	// AT_SECURE (triggered when the parent process has CapPrm=all — which the FIPS
	// agent daemon inherits from its systemd CapabilityBoundingSet — and execs a
	// binary without file capabilities), $ORIGIN expansion is silently disabled.
	// The binary's RPATH also contains an absolute fallback for its normal install
	// location, but that directory does not contain libcrypto on a deb-only install.
	//
	// Work around this by creating a symlink at the path that $ORIGIN would have
	// resolved to (/opt/datadog-packages/embedded/lib, i.e. tmpDir/../../embedded/lib)
	// pointing to the running installer's embedded lib. This path is a
	// Datadog-owned directory and does not affect system-wide library loading.
	// The symlink is created with the running installer's lib so the libcrypto
	// version matches the FIPS provider config (OPENSSL_CONF/OPENSSL_MODULES) also
	// derived from the running installer.
	extraEnv := fipsEnvFromRunningInstaller()
	if extraEnv != nil {
		setupFIPSBootstrapLibPath(installerBinPath)
	}
	return exec.NewInstallerExecWithExtraEnv(env, installerBinPath, extraEnv), nil
}

// setupFIPSBootstrapLibPath creates a symlink at /opt/datadog-packages/embedded/lib
// (= installerBinPath/../../embedded/lib, the path $ORIGIN would resolve to from the
// bootstrap installer binary's temp location) pointing to the running installer's own
// embedded lib directory.
//
// Under AT_SECURE the dynamic linker silently drops $ORIGIN-based RPATH entries.
// The bootstrap installer.layer binary falls back to an absolute RPATH entry for its
// normal install location which — on a deb-only install — does not contain libcrypto.
// By creating this symlink we ensure the $ORIGIN-equivalent path is populated with
// the running installer's version-matched libcrypto regardless of the AT_SECURE state.
//
// This path is Datadog-owned and does not affect system-wide library loading.
// If the symlink already points elsewhere (e.g., from a previous run with a different
// version), we update it so the libcrypto version always matches the current FIPS config.
func setupFIPSBootstrapLibPath(installerBinPath string) {
	exePath, err := exec.GetExecutable()
	if err != nil {
		return
	}
	// <install>/embedded/bin/installer → <install>/embedded/lib
	embeddedLib := filepath.Join(filepath.Dir(filepath.Dir(exePath)), "lib")
	// installerBinPath = /opt/datadog-packages/tmp/XXXX/installer
	// symlink target  = /opt/datadog-packages/embedded/lib
	tmpDir := filepath.Dir(installerBinPath)
	linkPath := filepath.Join(tmpDir, "..", "..", "embedded", "lib")
	linkPath = filepath.Clean(linkPath)
	_ = os.MkdirAll(filepath.Dir(linkPath), 0755)
	// Remove stale symlink (may point to old version) and re-create
	_ = os.Remove(linkPath)
	_ = os.Symlink(embeddedLib, linkPath)
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
	opensslConf := filepath.Join(embedded, "ssl", "openssl.cnf")
	opensslModules := filepath.Join(embedded, "lib", "ossl-modules")
	for _, p := range []string{opensslConf, opensslModules} {
		if _, statErr := os.Stat(p); statErr != nil {
			return nil
		}
	}
	return []string{
		"OPENSSL_CONF=" + opensslConf,
		"OPENSSL_MODULES=" + opensslModules,
		"LD_LIBRARY_PATH=" + filepath.Join(embedded, "lib"),
	}
}

func getInstallerOCI(_ context.Context, env *env.Env) (string, error) {
	version := "latest"
	if env.DefaultPackagesVersionOverride[InstallerPackage] != "" {
		version = env.DefaultPackagesVersionOverride[InstallerPackage]
	}
	return oci.PackageURL(env, InstallerPackage, version), nil
}
