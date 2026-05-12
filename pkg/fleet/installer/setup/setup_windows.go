// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package setup

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// envSetupReexec marks a child process spawned by handoffToRequestedAgentInstallerVersion so
// it unconditionally skips the version check and runs setup in-process —
// hard recursion guard.
const envSetupReexec = "DD_INSTALLER_SETUP_REEXEC"

// agentPackage is the OCI package name for the Datadog Agent.
const agentPackage = "datadog-agent"

// releaseSuffixRe matches a trailing `-N` release suffix (e.g. `-1`).
var releaseSuffixRe = regexp.MustCompile(`-\d+$`)

// formatDownloadError turns the oci Downloader's multi-registry error into a
// user-friendly summary: one line per registry attempted, each categorized
// (tag not found / auth failed / DNS / unreachable / timeout / other) so the
// user knows which knob to turn — version, credentials, proxy, network.
func formatDownloadError(tag string, err error) error {
	regErrs := oci.RegistryErrors(err)
	if len(regErrs) == 0 {
		// Shouldn't happen for a multi-registry Download, but fall back to
		// the raw error rather than swallowing it.
		return fmt.Errorf("could not download agent package %s: %w", tag, err)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "could not download agent package %s:\n", tag)
	for _, re := range regErrs {
		fmt.Fprintf(&sb, "  - %s: %s\n", re.Registry, categorizeDownloadError(re.Err))
	}
	fmt.Fprintln(&sb, "Check DD_AGENT_MAJOR_VERSION / DD_AGENT_MINOR_VERSION, DD_INSTALLER_REGISTRY_URL, proxy settings, and credentials as appropriate.")
	return errors.New(sb.String())
}

// categorizeDownloadError summarizes a single per-registry error in
// human-friendly terms, drilling through `transport.Error`, `*net.DNSError`,
// `net.Error` (timeouts), and common syscall errno values.
func categorizeDownloadError(err error) string {
	var te *transport.Error
	if errors.As(err, &te) {
		switch te.StatusCode {
		case http.StatusNotFound:
			return "tag not found (HTTP 404)"
		case http.StatusUnauthorized:
			return "authentication required (HTTP 401)"
		case http.StatusForbidden:
			return "authentication failed (HTTP 403)"
		case http.StatusProxyAuthRequired:
			return "proxy authentication required (HTTP 407)"
		default:
			return fmt.Sprintf("registry returned HTTP %d", te.StatusCode)
		}
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound {
			return fmt.Sprintf("DNS lookup failed: host %s does not resolve", dnsErr.Name)
		}
		return fmt.Sprintf("DNS lookup failed: %v", dnsErr)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "network timeout"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "request timed out"
	}
	// Fall through — keep the raw text so we don't hide anything.
	return err.Error()
}

// resolveAgentOCITag translates DD_AGENT_MAJOR/MINOR_VERSION into the OCI tag
// the Datadog Agent package publishes. Built on env.GetAgentVersion plus two
// small fixups:
//   - `~` → `-` (the Linux/macOS scripts accept `~rc.N`, OCI tags use `-rc.N`)
//   - append `-N` release suffix if the input has a patch component but lacks
//     one (so `7.78.0` → `7.78.0-1`)
//
// Returns `"latest"` when no version is requested. Bare minor (e.g.
// `MINOR=78`) maps to `7.78`, which the registry serves as a moving tag
// pointing to the latest patch.
func resolveAgentOCITag(e *env.Env) string {
	v := strings.ReplaceAll(e.GetAgentVersion(), "~", "-")
	if v == "latest" {
		return v
	}
	if strings.Count(v, ".") >= 2 && !releaseSuffixRe.MatchString(v) {
		v += "-1"
	}
	return v
}

// handoffToRequestedAgentInstallerVersion downloads the version-specific
// `datadog-installer.exe` matching DD_AGENT_MAJOR/MINOR_VERSION and re-execs
// setup from it. Returns (true, err) when re-exec was attempted (caller should
// return without running setup in-process); (false, nil) when no version was
// requested or the recursion guard is active and setup should proceed
// in-process.
func handoffToRequestedAgentInstallerVersion(ctx context.Context, e *env.Env, flavor string) (bool, error) {
	if os.Getenv(envSetupReexec) == "true" {
		return false, nil
	}
	tag := resolveAgentOCITag(e)
	if tag == "latest" {
		return false, nil
	}

	if err := paths.SetupInstallerDataDir(); err != nil {
		return false, fmt.Errorf("could not ensure installer data dir: %w", err)
	}
	if err := os.MkdirAll(paths.RootTmpDir, 0755); err != nil {
		return false, fmt.Errorf("could not create tmp dir: %w", err)
	}
	tmpDir, err := os.MkdirTemp(paths.RootTmpDir, "setup-reexec")
	if err != nil {
		return false, fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	url := oci.PackageURL(e, agentPackage, tag)
	pkg, err := oci.NewDownloader(e, e.HTTPClient()).Download(ctx, url)
	if err != nil {
		return false, formatDownloadError(tag, err)
	}
	exePath := filepath.Join(tmpDir, "datadog-installer.exe")
	if err := pkg.ExtractLayers(oci.DatadogPackageInstallerLayerMediaType, exePath); err != nil {
		return false, fmt.Errorf("could not extract installer.exe layer: %w", err)
	}
	if _, err := os.Stat(exePath); err != nil {
		return false, fmt.Errorf("agent version %s does not publish a datadog-installer.exe layer (Agent 7.70+ required for Windows setup re-exec)", tag)
	}

	// Re-exec the downloaded installer with the parent's os.Environ()
	// inherited as-is, plus a recursion-guard marker. We deliberately do not
	// go through InstallerExec / env.ToEnv: this is the user-invoked setup
	// path, so the child should see only what the user originally set. The
	// child re-derives APIKey / Site / registry config from the same
	// datadog.yaml the parent reads.
	args := []string{"setup"}
	if flavor != "" {
		args = append(args, "--flavor", flavor)
	}
	cmd := exec.CommandContext(ctx, exePath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), envSetupReexec+"=true")
	if err := cmd.Run(); err != nil {
		return true, fmt.Errorf("re-exec'd setup failed: %w", err)
	}
	return true, nil
}
