// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package agent includes helpers related to the Datadog Agent on Windows
package agent

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent/installers/v2"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/pipeline"
)

const (
	defaultMajorVersion  = "7"
	defaultArch          = "x86_64"
	agentS3BucketRelease = "ddagent-windows-stable"
	betaChannel          = "beta"
	betaURL              = "https://s3.amazonaws.com/dd-agent-mstesting/builds/beta/installers_v2.json"
	stableChannel        = "stable"
	stableURL            = "https://ddagent-windows-stable.s3.amazonaws.com/installers_v2.json"
)

// Package contains identifying information about an Agent MSI package.
type Package struct {
	// --- Resolution fields (used by Resolve()) ---

	// PipelineID is the pipeline ID used to lookup the MSI URL from the CI pipeline artifacts.
	PipelineID string
	// Channel is the channel used to lookup the MSI URL for the Version from the installers_v2.json file.
	Channel string
	// Version is the package version for resolution (e.g. "7.75.0-1", "7.49.0-rc.3-1")
	Version string
	// Arch is the architecture of the MSI, e.g. x86_64
	Arch string
	// URL is the URL the MSI can be downloaded from
	URL string
	// Flavor is the Agent Flavor (e.g. `base`, `fips`, `iot`)
	Flavor string
	// Product is the installers json package name (e.g. `datadog-agent`, `datadog-fips-agent`)
	Product string

	// --- Assertion fields (not used by Resolve()) ---

	// AssertAgentVersion is the expected agent display version (e.g. "7.75.0").
	// Used by AgentVersion() for test assertions and logging.
	AssertAgentVersion string
	// AssertPackageVersion is the expected url-safe package version (e.g. "7.75.0-1").
	// Used for Fleet status assertions in tests.
	AssertPackageVersion string
}

// AgentVersion returns the agent display version for assertions (e.g. "7.75.0", "7.78.0-devel").
//
// If AssertAgentVersion is set (via _ASSERT_VERSION), it is returned directly.
// Otherwise, the version is derived from the resolution Version field by trimming
// the "-1" suffix and parsing. This fallback supports tests that construct Package
// structs directly with hardcoded versions.
func (p *Package) AgentVersion() string {
	if p.AssertAgentVersion != "" {
		// TODO: we're currently only asserting 7.77.0-devel style versions,
		//       it would be an improvement to assert on the full version (git sha, pipeline)
		ver, err := version.New(p.AssertAgentVersion, "")
		if err != nil {
			panic(fmt.Errorf("unexpected error parsing version %v: %w", p.Version, err))
		}
		return ver.GetNumberAndPre()
	}
	// Fallback: derive from the resolution Version field
	ver, err := version.New(strings.TrimSuffix(p.Version, "-1"), "")
	if err != nil {
		// release pipeline builds have a url-safe version that fails to parse
		// Example: 7.66.0.git.0.8005fe1.pipeline.65816352-1
		// so restore the "non-url-safe" version and try again
		v := strings.Replace(p.Version, ".git.", "+git", 1)
		ver, err = version.New(strings.TrimSuffix(v, "-1"), "")
		if err != nil {
			panic(fmt.Errorf("unexpected error parsing version %v: %w", v, err))
		}
	}
	return ver.GetNumberAndPre()
}

// GetBetaMSIURL returns the URL for the beta agent MSI
//
// majorVersion: 6, 7
// arch: x86_64
// flavor: base, fips
func GetBetaMSIURL(version string, arch string, flavor string) (string, error) {
	return GetMSIURL(betaChannel, version, arch, flavor)
}

// GetStableMSIURL returns the URL for the stable agent MSI
//
// majorVersion: 6, 7
// arch: x86_64
// flavor: base, fips
func GetStableMSIURL(version string, arch string, flavor string) (string, error) {
	return GetMSIURL(stableChannel, version, arch, flavor)
}

// GetMSIURL returns the URL for the agent MSI
//
// channel: beta, stable
// majorVersion: 6, 7
// arch: x86_64
// flavor: base, fips
func GetMSIURL(channel string, version string, arch string, flavor string) (string, error) {
	channelURL, err := GetChannelURL(channel)
	if err != nil {
		return "", err
	}

	productName, err := GetFlavorProductName(flavor)
	if err != nil {
		return "", err
	}

	return installers.GetProductURL(channelURL, productName, version, arch)
}

// GetFlavorProductName returns the product name for the flavor
//
// flavor: base, fips
func GetFlavorProductName(flavor string) (string, error) {
	switch flavor {
	case "":
		return "datadog-agent", nil
	case "base":
		return "datadog-agent", nil
	case "fips":
		return "datadog-fips-agent", nil
	default:
		return "", fmt.Errorf("unknown flavor %v", flavor)
	}
}

// GetChannelURL returns the URL for the channel name
//
// channel: beta, stable
func GetChannelURL(channel string) (string, error) {
	if strings.EqualFold(channel, betaChannel) {
		return betaURL, nil
	} else if strings.EqualFold(channel, stableChannel) {
		return stableURL, nil
	}

	return "", fmt.Errorf("unknown channel %v", channel)
}

// GetLatestMSIURL returns the URL for the latest agent MSI
//
// majorVersion: 6, 7
// arch: x86_64
func GetLatestMSIURL(majorVersion string, arch string, flavor string) (string, error) {
	// why do we use amd64 for the latest URL and x86_64 everywhere else?
	if arch == "x86_64" {
		arch = "amd64"
	}
	productName, err := GetFlavorProductName(flavor)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`https://s3.amazonaws.com/`+agentS3BucketRelease+`/%s-%s-latest.%s.msi`,
		productName, majorVersion, arch), nil
}

// GetPipelineMSIURL returns the URL for the agent MSI built by the pipeline
//
// majorVersion: 6, 7
// arch: x86_64
// flavor: base, fips
func GetPipelineMSIURL(pipelineID string, majorVersion string, arch string, flavor string, nameSuffix string) (string, error) {
	productName, err := GetFlavorProductName(flavor)
	if err != nil {
		return "", err
	}
	productName = fmt.Sprintf("%s%s", productName, nameSuffix)
	// Manual URL example: https://s3.amazonaws.com/dd-agent-mstesting?prefix=pipelines/A7/25309493
	fmt.Printf("Looking for agent MSI for pipeline majorVersion %v %v\n", majorVersion, pipelineID)
	artifactURL, err := pipeline.GetPipelineArtifact(pipelineID, pipeline.AgentS3BucketTesting, majorVersion, func(artifact string) bool {
		// In case there are multiple artifacts, try to match the right one
		// This is only here as a workaround for a CI issue that can cause artifacts
		// from different pipelines to be mixed together. This should be removed once
		// the issue is resolved.
		// TODO: CIREL-1970
		// Example: datadog-agent-7.52.0-1-x86_64.msi
		// Example: datadog-agent-7.53.0-devel.git.512.41b1225.pipeline.30353507-1-x86_64.msi
		if !strings.Contains(artifact, fmt.Sprintf("%s-%s", productName, majorVersion)) {
			return false
		}

		// Not all pipelines include the pipeline ID in the artifact name, but if it is there then match against it
		if strings.Contains(artifact, "pipeline.") &&
			!strings.Contains(artifact, "pipeline."+pipelineID) {
			return false
		}
		if !strings.Contains(artifact, fmt.Sprintf("-%s.msi", arch)) {
			return false
		}
		// match!
		return true
	})
	if err != nil {
		return "", fmt.Errorf("no agent MSI found for pipeline %v arch %v flavor: %v: %w", pipelineID, arch, flavor, err)
	}
	return artifactURL, nil
}

// GetPackageFromEnv looks at environment variables to select the Agent MSI URL.
//
// The returned Package contains the MSI URL and other identifying information.
// Some Package fields will be populated but may not be related to the returned URL.
// For example, if a URL is provided directly, the Channel, Version, Arch, and Flavor fields
// have no effect on the returned URL. They are returned anyway so they can be used for
// other purposes, such as logging, stack name, instance options, test assertions, etc.
//
// Package fields are configured via CURRENT_AGENT_* environment variables.
// See [WithDevEnvOverrides] for the full list. Set CURRENT_AGENT_PIPELINE or
// CURRENT_AGENT_SOURCE_VERSION to select the package source.
//
// Optional [PackageOption] arguments are applied as defaults before the
// CURRENT_AGENT_* overrides, so environment variables always take priority.
//
// If none of the above are set, the latest stable version is used.
func GetPackageFromEnv(defaults ...PackageOption) (*Package, error) {
	var opts []PackageOption
	opts = append(opts, defaults...)
	opts = append(opts, WithDevEnvOverrides("CURRENT_AGENT"))
	pkg, err := NewPackage(opts...)
	if err != nil {
		return nil, err
	}

	// Fallback: if nothing resolved to a URL, default to the latest stable MSI.
	if pkg.URL == "" {
		url, err := GetLatestMSIURL(defaultMajorVersion, pkg.Arch, pkg.Flavor)
		if err != nil {
			return nil, err
		}
		pkg.URL = url
		if pkg.Channel == "" {
			pkg.Channel = stableChannel
		}
	}

	return pkg, nil
}

// GetLastStablePackageFromEnv returns the latest stable agent MSI package.
//
// It delegates to NewPackage with WithDevEnvOverrides("STABLE_AGENT"), which reads
// the STABLE_AGENT_* environment variables to determine the MSI URL.
//
// In CI, these variables are set by the pipeline script from release.json or
// LAST_STABLE_PIPELINE_ID. See [WithDevEnvOverrides] for the full list of
// supported environment variables.
//
// Optional [PackageOption] arguments are applied as defaults before the
// STABLE_AGENT_* overrides, so environment variables always take priority.
func GetLastStablePackageFromEnv(defaults ...PackageOption) (*Package, error) {
	opts := append(defaults, WithDevEnvOverrides("STABLE_AGENT"))
	pkg, err := NewPackage(opts...)
	if err != nil {
		return nil, err
	}
	if pkg.URL == "" {
		return nil, errors.New("STABLE_AGENT_SOURCE_VERSION, STABLE_AGENT_PIPELINE, or a more specific STABLE_AGENT_MSI_* override is required")
	}
	return pkg, nil
}

// GetUpgradeTestPackageFromEnv returns the upgrade test package to use in upgrade test.
//
// The upgrade test MSI is a variant of the current agent built by the same pipeline,
// identified by a "-upgrade-test" suffix in the S3 artifact name.
//
// Resolution priority:
//  1. CURRENT_AGENT_MSI_URL (or UPGRADE_AGENT_MSI_URL) -- direct URL
//  2. CURRENT_AGENT_PIPELINE (or CURRENT_AGENT_MSI_PIPELINE) -- pipeline lookup with "-upgrade-test" suffix
//
// Arch and flavor are read from the CURRENT_AGENT_MSI_* overrides (see [WithDevEnvOverrides]).
func GetUpgradeTestPackageFromEnv() (*Package, error) {
	// Build a base package from CURRENT_AGENT_* to pick up arch, flavor, version, pipeline, etc.
	base := &Package{
		Product: "datadog-agent",
		Arch:    defaultArch,
	}
	if err := WithDevEnvOverrides("CURRENT_AGENT")(base); err != nil {
		return nil, err
	}

	// Direct URL override (UPGRADE_AGENT_MSI_URL)
	if url := os.Getenv("UPGRADE_AGENT_MSI_URL"); url != "" {
		base.URL = url
		return base, nil
	}

	// Pipeline lookup with "-upgrade-test" suffix
	if base.PipelineID != "" {
		url, err := GetPipelineMSIURL(base.PipelineID, defaultMajorVersion, base.Arch, base.Flavor, "-upgrade-test")
		if err != nil {
			return nil, err
		}
		base.URL = url
		return base, nil
	}

	return nil, errors.New("UPGRADE_AGENT_MSI_URL or CURRENT_AGENT_PIPELINE (or CURRENT_AGENT_MSI_PIPELINE) is required")
}

// PackageOption defines a function type for modifying a Package
type PackageOption func(*Package) error

// NewPackage creates a new Package with the provided options.
//
// After all options are applied, Resolve() is called to fill in derived fields
// (e.g. URL from PipelineID or Version+Channel). Options that set URL directly
// (WithURL) make Resolve() a no-op.
func NewPackage(opts ...PackageOption) (*Package, error) {
	pkg := &Package{
		Product: "datadog-agent",
		Arch:    "x86_64",
	}
	for _, opt := range opts {
		if err := opt(pkg); err != nil {
			return nil, err
		}
	}
	if err := pkg.Resolve(); err != nil {
		return nil, err
	}
	return pkg, nil
}

// Resolve fills in derived fields after all options have been applied.
// It only performs I/O if URL is not already set.
//
// Resolution priority (first match wins):
//  1. URL already set -- no-op
//  2. PipelineID set -- fetches MSI URL from S3 pipeline artifacts
//  3. Version set -- infers channel if needed, fetches URL from installers_v2.json
func (p *Package) Resolve() error {
	if p.URL != "" {
		return nil
	}

	// If the pipeline ID is set, fetch the MSI URL from the pipeline artifacts
	if p.PipelineID != "" {
		arch := p.Arch
		if arch == "" {
			arch = defaultArch
		}
		url, err := GetPipelineMSIURL(p.PipelineID, defaultMajorVersion, arch, p.Flavor, "")
		if err != nil {
			return err
		}
		p.URL = url
		return nil
	}

	// If the version is set, fetch the MSI URL from the installers JSON
	if p.Version != "" {
		if p.Channel == "" {
			// If the channel is not set, infer it from the version
			p.Channel = stableChannel
			if strings.Contains(strings.ToLower(p.Version), `-rc.`) {
				p.Channel = betaChannel
			}
		}
		channelURL, err := GetChannelURL(p.Channel)
		if err != nil {
			return err
		}
		product := p.Product
		if product == "" {
			product = "datadog-agent"
		}
		// Fetch the MSI URL from the installers JSON
		url, err := installers.GetProductURL(channelURL, product, p.Version, p.Arch)
		if err != nil {
			return err
		}
		p.URL = url
	}

	return nil
}

// WithChannel sets the channel for the Package
//
// Example: beta, stable
func WithChannel(channel string) PackageOption {
	return func(p *Package) error {
		p.Channel = channel
		return nil
	}
}

// WithVersion sets the version for the Package
//
// If using installers_v2.json, the version must match the version key in the json file
//
// Example: 7.65.0-1, 7.65.0-rc.1-1
func WithVersion(version string) PackageOption {
	return func(p *Package) error {
		p.Version = version
		return nil
	}
}

// WithArch sets the architecture for the Package
//
// Default is x86_64
//
// If using installers_v2.json, the arch must match the arch key in the json file
//
// Example: x86_64
func WithArch(arch string) PackageOption {
	return func(p *Package) error {
		p.Arch = arch
		return nil
	}
}

// WithFlavor sets the flavor for the Package
//
// # Default is empty, which is the base flavor
//
// Example: base, fips
func WithFlavor(flavor string) PackageOption {
	return func(p *Package) error {
		p.Flavor = flavor
		return nil
	}
}

// WithProduct sets the product for the Package
//
// If using installers_v2.json, the product must match the product key in the json file
//
// Example: datadog-agent, datadog-fips-agent
func WithProduct(product string) PackageOption {
	return func(p *Package) error {
		p.Product = product
		return nil
	}
}

// WithURL sets the URL for the MSI Package
func WithURL(url string) PackageOption {
	return func(p *Package) error {
		p.URL = url
		return nil
	}
}

// WithPipelineID sets the pipeline ID for the Package
func WithPipelineID(pipelineID string) PackageOption {
	return func(p *Package) error {
		p.PipelineID = pipelineID
		return nil
	}
}

// WithDevEnvOverrides applies environment variable overrides to the Package.
//
// This is a pure field-setter: it reads environment variables and sets struct fields,
// but does not perform any I/O. URL resolution is deferred to [Package.Resolve],
// which is called automatically at the end of [NewPackage].
//
// # Assertion variables (never affect resolution)
//
//	{PREFIX}_ASSERT_VERSION         - Agent display version for test assertions (e.g. "7.75.0")
//	{PREFIX}_ASSERT_PACKAGE_VERSION - URL-safe package version for test assertions (e.g. "7.75.0-1")
//
// # Resolution variables (mutually exclusive)
//
//	{PREFIX}_SOURCE_VERSION - Package version for lookup (e.g. "7.75.0-1"), clears any pipeline set by prior options
//	{PREFIX}_PIPELINE       - Pipeline ID, resolves MSI from S3 pipeline artifacts
//
// # MSI-specific overrides (take priority over resolution vars)
//
//	{PREFIX}_MSI_FLAVOR   - Agent flavor (e.g. "base", "fips")
//	{PREFIX}_MSI_PRODUCT  - Product name (e.g. "datadog-agent")
//	{PREFIX}_MSI_ARCH     - Architecture (e.g. "x86_64")
//	{PREFIX}_MSI_CHANNEL  - Channel (e.g. "stable", "beta")
//	{PREFIX}_MSI_VERSION  - Package version (e.g. "7.75.0-1")
//	{PREFIX}_MSI_URL      - Direct MSI URL (skips Resolve)
//	{PREFIX}_MSI_PIPELINE - Pipeline ID for MSI (overrides _PIPELINE)
//
// Examples:
//
//	export STABLE_AGENT_SOURCE_VERSION="7.75.0-1"
//	export STABLE_AGENT_PIPELINE="123456"
//	export CURRENT_AGENT_MSI_URL="file:///path/to/msi/package.msi"
func WithDevEnvOverrides(devenvPrefix string) PackageOption {
	return func(p *Package) error {
		// Assertion metadata (never affects resolution)
		if v, ok := os.LookupEnv(devenvPrefix + "_ASSERT_VERSION"); ok {
			p.AssertAgentVersion = v
		}
		if v, ok := os.LookupEnv(devenvPrefix + "_ASSERT_PACKAGE_VERSION"); ok {
			p.AssertPackageVersion = v
		}

		// Resolution: _SOURCE_VERSION and _PIPELINE are mutually exclusive
		_, hasSourceVersion := os.LookupEnv(devenvPrefix + "_SOURCE_VERSION")
		_, hasPipeline := os.LookupEnv(devenvPrefix + "_PIPELINE")
		if hasSourceVersion && hasPipeline {
			return fmt.Errorf("%s_SOURCE_VERSION and %s_PIPELINE are mutually exclusive", devenvPrefix, devenvPrefix)
		}
		if hasSourceVersion {
			v := os.Getenv(devenvPrefix + "_SOURCE_VERSION")
			p.Version = v
			// clear the pipeline ID so it doesn't affect the resolution
			p.PipelineID = ""
		}
		if hasPipeline {
			p.PipelineID = os.Getenv(devenvPrefix + "_PIPELINE")
		}

		// MSI-specific overrides (highest priority)
		if flavor, ok := os.LookupEnv(devenvPrefix + "_MSI_FLAVOR"); ok {
			p.Flavor = flavor
		}
		if product, ok := os.LookupEnv(devenvPrefix + "_MSI_PRODUCT"); ok {
			p.Product = product
		}
		if arch, ok := os.LookupEnv(devenvPrefix + "_MSI_ARCH"); ok {
			p.Arch = arch
		}
		if channel, ok := os.LookupEnv(devenvPrefix + "_MSI_CHANNEL"); ok {
			p.Channel = channel
		}
		if version, ok := os.LookupEnv(devenvPrefix + "_MSI_VERSION"); ok {
			p.Version = version
		}
		if url, ok := os.LookupEnv(devenvPrefix + "_MSI_URL"); ok {
			p.URL = url
		}
		if pipelineID, ok := os.LookupEnv(devenvPrefix + "_MSI_PIPELINE"); ok {
			p.PipelineID = pipelineID
		}

		return nil
	}
}
