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
	"regexp"
	"strconv"
	"strings"

	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/bootstrap"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// agentPackage is the OCI package name for the Datadog Agent.
const agentPackage = "datadog-agent"

// agentPackageImage is the image name as it appears in OCI URLs.
// Used as the map key for env.RegistryOverrideByImage.
const agentPackageImage = "agent-package"

// envInstallerRegistryURLAgent is the per-image registry override the
// oci downloader and env.FromEnv() both honor. We set this from a
// non-stable channel so the parent's own download and the child
// re-exec's downstream Agent fetches all hit the same registry.
const envInstallerRegistryURLAgent = "DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE"

// Channels we accept.
const (
	channelStable = "stable"
	channelBeta   = "beta"
)

// betaRegistry is the OCI registry that hosts beta / RC Agent builds.
const betaRegistry = "install.datad0g.com"

// pipelineRegistry is the OCI registry that hosts per-pipeline Agent builds.
const pipelineRegistry = "installtesting.datad0g.com"

// envInstallerDefaultVersionAgent is the env var the oci downloader and
// env.FromEnv() both honor for per-package version overrides. We set this
// for pipeline builds so the child re-exec inherits the pinned tag.
const envInstallerDefaultVersionAgent = "DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT"

// releaseSuffixRe matches a trailing `-N` release suffix (e.g. `-1`).
var releaseSuffixRe = regexp.MustCompile(`-\d+$`)

// agentDistChannel validates e.AgentDistChannel ("stable", "beta").
func agentDistChannel(e *env.Env) (string, error) {
	switch e.AgentDistChannel {
	case "", channelStable:
		return channelStable, nil
	case channelBeta:
		return channelBeta, nil
	default:
		return "", fmt.Errorf("DD_AGENT_DIST_CHANNEL must be one of: %s, %s. Current value: %s", channelStable, channelBeta, e.AgentDistChannel)
	}
}

// requestedAgentVersion returns the OCI tag the user has asked the running
// installer to install — or "" if they have not asked for a specific version,
// in which case setup should run in-process and install the version baked
// into this binary.
//
// Supported inputs:
//   - DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT — returned verbatim
//     (matches bootstrap.getInstallerOCI; lets CI pin an exact OCI tag).
//   - DD_AGENT_MAJOR_VERSION only — returns the moving major tag (e.g. "7").
//   - DD_AGENT_MAJOR_VERSION + DD_AGENT_MINOR_VERSION — joined and normalized
//     (`~` → `-`, append `-N` release suffix when missing). Major defaults to
//     "7" if only minor is set.
//   - Neither — returns "".
func requestedAgentVersion(e *env.Env) (string, error) {
	if override := e.DefaultPackagesVersionOverride[agentPackage]; override != "" {
		return override, nil
	}
	major, minor := e.AgentMajorVersion, e.AgentMinorVersion
	if major == "" && minor == "" {
		return "", nil
	}
	// Fleet automation only supports Agent 7 (Linux install script
	// validates similarly — it accepts 6 or 7). Reject anything else
	// loudly rather than composing a garbage tag.
	if major != "" && major != "7" {
		return "", fmt.Errorf("DD_AGENT_MAJOR_VERSION must be 7. Current value: %s", major)
	}
	if major == "" {
		major = "7"
	}
	if minor == "" {
		return major, nil
	}
	// Agent 7.72 is the first publicly-released datadog-installer.exe
	// where `installer.exe` with no args invokes `setup --flavor default`
	// on Windows; the handoff re-execs the child with that exact
	// invocation, so older Agents either don't recognize the "default"
	// flavor or lack the `setup` subcommand entirely.
	if n, err := parseMinorPrefix(minor); err != nil || n < 72 {
		return "", fmt.Errorf("DD_AGENT_MINOR_VERSION must be 72 or newer on Windows. Current value: %s", minor)
	}
	minor = strings.ReplaceAll(minor, "~", "-")
	v := major + "." + minor
	if strings.Count(v, ".") >= 2 && !releaseSuffixRe.MatchString(v) {
		v += "-1"
	}
	return v, nil
}

// applyAgentDistOptions resolves the options that select which Agent build to
// install:
//
//   - DD_AGENT_PIPELINE_ID — a specific CI pipeline build (see applyAgentPipelineID)
//   - DD_AGENT_DIST_CHANNEL — the release channel, stable or beta (see applyAgentDistChannel)
func applyAgentDistOptions(e *env.Env) error {
	if err := applyAgentPipelineID(e); err != nil {
		return err
	}
	return applyAgentDistChannel(e)
}

// applyAgentDistChannel reads DD_AGENT_DIST_CHANNEL and overrides the
// registry used for agent-package OCI fetches before running setup.
//
// Cases:
//
//   - DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE is already set → no-op (user wins)
//   - channel == channelStable or unset → no-op
//   - channel == channelBeta → default flips to betaRegistry
//
// Scope: agent-package only (suffix _AGENT_PACKAGE). APM SSI and other
// non-Agent packages keep resolving via the site-based default.
//
// Limitation: oci.getRefAndKeychains treats overrides as mutually
// exclusive with the default-registry fallback list:
//
//   - per-image override shadows env.RegistryOverride (populated from
//     installer.registry.url in datadog.yaml) and the hardcoded defaults
//   - only the chosen registry is tried — a host's global mirror is not
//     attempted for beta-channel fetches
//   - same shape impacts cross-registry upgrade / downgrade / rollback
//     when a build only exists in one registry but operations target another
//
// Future work in pkg/fleet/installer/oci/download.go could extend
// getRefAndKeychains to try {per-image override, global override,
// defaults} in order rather than short-circuiting on the first override.
func applyAgentDistChannel(e *env.Env) error {
	channel, err := agentDistChannel(e)
	if err != nil {
		return err
	}
	if channel != channelBeta {
		return nil
	}
	if _, set := e.RegistryOverrideByImage[agentPackageImage]; set {
		return nil
	}
	e.RegistryOverrideByImage[agentPackageImage] = betaRegistry
	return os.Setenv(envInstallerRegistryURLAgent, betaRegistry)
}

// applyAgentPipelineID reads DD_AGENT_PIPELINE_ID and overrides both the
// registry and the version used for agent-package OCI fetches so setup
// installs a specific CI pipeline build.
//
// Cases:
//
//   - DD_AGENT_PIPELINE_ID unset → no-op
//   - DD_AGENT_DIST_CHANNEL, DD_AGENT_MAJOR_VERSION, or DD_AGENT_MINOR_VERSION
//     is set → error (these high-level options are mutually exclusive with the
//     pipeline build)
//   - DD_INSTALLER_REGISTRY_URL_AGENT_PACKAGE or
//     DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT is already set → that
//     explicit override wins for its axis (no-op), matching applyAgentDistChannel
//   - otherwise → registry flips to pipelineRegistry, version to pipeline-<id>
func applyAgentPipelineID(e *env.Env) error {
	id := e.AgentPipelineID
	if id == "" {
		return nil
	}
	if e.AgentDistChannel != "" {
		return fmt.Errorf("DD_AGENT_PIPELINE_ID and DD_AGENT_DIST_CHANNEL=%s are mutually exclusive", e.AgentDistChannel)
	}
	if e.AgentMajorVersion != "" || e.AgentMinorVersion != "" {
		return errors.New("DD_AGENT_PIPELINE_ID is mutually exclusive with DD_AGENT_MAJOR_VERSION and DD_AGENT_MINOR_VERSION")
	}
	tag := "pipeline-" + id
	if _, set := e.RegistryOverrideByImage[agentPackageImage]; !set {
		e.RegistryOverrideByImage[agentPackageImage] = pipelineRegistry
		if err := os.Setenv(envInstallerRegistryURLAgent, pipelineRegistry); err != nil {
			return err
		}
	}
	if _, set := e.DefaultPackagesVersionOverride[agentPackage]; !set {
		e.DefaultPackagesVersionOverride[agentPackage] = tag
		if err := os.Setenv(envInstallerDefaultVersionAgent, tag); err != nil {
			return err
		}
	}
	return nil
}

// parseMinorPrefix returns the numeric minor at the start of s — e.g. "78"
// for any of "78", "78.0", "78.0~rc.2", "78.0-beta-extensions".
func parseMinorPrefix(s string) (int, error) {
	if i := strings.IndexAny(s, ".-~"); i >= 0 {
		s = s[:i]
	}
	return strconv.Atoi(s)
}

// runAgentInstaller downloads the Agent OCI package at the given tag,
// extracts its datadog-installer.exe layer, and re-execs setup from that
// downloaded binary so the installer matches the Agent version being
// installed. The child inherits the parent's os.Environ() plus the
// recursion-guard marker (DD_INSTALLER_FROM_VERSION_HANDOFF=true) so it
// does not re-handoff.
//
// Caller is responsible for gating this on the recursion-guard marker and
// requestedAgentVersion != "" before calling.
func runAgentInstaller(ctx context.Context, e *env.Env, flavor, tag string) error {
	if err := paths.SetupInstallerDataDir(); err != nil {
		return fmt.Errorf("could not ensure installer data dir: %w", err)
	}
	if err := os.MkdirAll(paths.RootTmpDir, 0755); err != nil {
		return fmt.Errorf("could not create tmp dir: %w", err)
	}
	tmpDir, err := os.MkdirTemp(paths.RootTmpDir, "setup-reexec")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	url := oci.PackageURL(e, agentPackage, tag)
	fmt.Fprintf(os.Stdout, "Downloading installer for Datadog Agent %s ...\n", tag)
	// bootstrap.DownloadInstallerExe handles the OCI installer.exe layer
	// (Agent 7.79+) and falls back to MSI admin-install extraction for
	// older fleet-supported Agents (7.72–7.78), so older versions go
	// through the same version-matched-installer path as new ones.
	exePath, err := bootstrap.DownloadInstallerExe(ctx, e, url, tmpDir)
	if err != nil {
		return formatDownloadError(tag, err)
	}

	// Re-exec the downloaded installer with the parent's os.Environ()
	// inherited as-is, plus the recursion-guard marker. We deliberately do
	// not go through InstallerExec / env.ToEnv: this is the user-invoked
	// setup path, so the child should see only what the user originally set.
	// The child re-derives APIKey / Site / registry config from the same
	// datadog.yaml the parent reads.
	args := []string{"setup"}
	if flavor != "" {
		args = append(args, "--flavor", flavor)
	}
	cmd := exec.CommandContext(ctx, exePath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), envFromVersionHandoff+"=true")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("re-exec'd setup failed: %w", err)
	}
	return nil
}

// formatDownloadError turns the oci Downloader's multi-registry error into a
// user-friendly summary: one line per registry attempted, each categorized
// (tag not found / auth failed / DNS / unreachable / timeout / other).
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
			return "tag not found or access is denied (HTTP 403)"
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
