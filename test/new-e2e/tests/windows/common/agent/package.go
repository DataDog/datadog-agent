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
	defaultFlavor        = "base"
	agentS3BucketRelease = "ddagent-windows-stable"
	betaChannel          = "beta"
	betaURL              = "https://s3.amazonaws.com/dd-agent-mstesting/builds/beta/installers_v2.json"
	stableChannel        = "stable"
	stableURL            = "https://ddagent-windows-stable.s3.amazonaws.com/installers_v2.json"
)

// Environment variable constants
const (
	PackageFlavorEnvVar = "WINDOWS_AGENT_FLAVOR"
)

// Package contains identifying information about an Agent MSI package.
type Package struct {
	// PipelineID is the pipeline ID used to lookup the MSI URL from the CI pipeline artifacts.
	PipelineID string
	// Channel is the channel used to lookup the MSI URL for the Version from the installers_v2.json file.
	Channel string
	// Version is the version of the MSI, e.g. 7.49.0-1, 7.49.0-rc.3-1, or a major version, e.g. 7
	Version string
	// Arch is the architecture of the MSI, e.g. x86_64
	Arch string
	// URL is the URL the MSI can be downloaded from
	URL string
	// Flavor is the Agent Flavor (e.g. `base`, `fips`, `iot``)
	Flavor string
	// Product is the installers json package name (e.g. `datadog-agent`, `datadog-fips-agent`)
	Product string
}

// AgentVersion returns a string containing version number and the pre only, e.g. `0.0.0-beta.1`
func (p *Package) AgentVersion() string {
	// Trim the package suffix and parse the remaining version info
	ver, _ := version.New(strings.TrimSuffix(p.Version, "-1"), "")
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

// LookupChannelFromEnv looks at environment variabes to select the agent channel, if the value
// is found it is returned along with true, otherwise a default value and false are returned.
//
// WINDOWS_AGENT_CHANNEL: beta, stable
//
// Default channel: stable
func LookupChannelFromEnv() (string, bool) {
	channel := os.Getenv("WINDOWS_AGENT_CHANNEL")
	if channel != "" {
		return channel, true
	}
	return stableChannel, false
}

// LookupFlavorFromEnv looks at environment variables to select the agent flavor, if the value
// is found it is returned along with true, otherwise an empty string and false are returned.
//
// WINDOWS_AGENT_FLAVOR: base, fips
//
// Default Flavor: base
func LookupFlavorFromEnv() (string, bool) {
	flavor := os.Getenv(PackageFlavorEnvVar)
	if flavor != "" {
		return flavor, true
	}
	return defaultFlavor, false
}

// LookupVersionFromEnv looks at environment variabes to select the agent version, if the value
// is found it is returned along with true, otherwise a default value and false are returned.
//
// In order of priority:
//
// WINDOWS_AGENT_VERSION: The complete version, e.g. 7.49.0-1, 7.49.0-rc.3-1, or a major version, e.g. 7
//
// AGENT_MAJOR_VERSION: The major version of the agent, 6 or 7
//
// If only a major version is provided, the latest version of that major version is used.
//
// Default version: 7
func LookupVersionFromEnv() (string, bool) {
	version := os.Getenv("WINDOWS_AGENT_VERSION")
	if version != "" {
		return version, true
	}

	// Currently commonly used in CI, should we keep it or transition to WINDOWS_AGENT_VERSION?
	version = os.Getenv("AGENT_MAJOR_VERSION")
	if version != "" {
		return version, true
	}

	return defaultMajorVersion, false
}

// LookupArchFromEnv looks at environment variabes to select the agent arch, if the value
// is found it is returned along with true, otherwise a default value and false are returned.
//
// WINDOWS_AGENT_ARCH: The arch of the agent, x86_64
//
// Default arch: x86_64
func LookupArchFromEnv() (string, bool) {
	arch := os.Getenv("WINDOWS_AGENT_ARCH")
	if arch != "" {
		return arch, true
	}
	return defaultArch, false
}

// LookupChannelURLFromEnv looks at environment variabes to select the agent channel URL, if the value
// is found it is returned along with true, otherwise a default value and false are returned.
//
// WINDOWS_AGENT_CHANNEL_URL: URL to installers_v2.json
//
// See also LookupChannelFromEnv()
//
// Default channel: stable
func LookupChannelURLFromEnv() (string, bool) {
	channelURL := os.Getenv("WINDOWS_AGENT_CHANNEL_URL")
	if channelURL != "" {
		return channelURL, true
	}

	channel, channelFound := LookupChannelFromEnv()
	channelURL, err := GetChannelURL(channel)
	if err != nil {
		// passthru the found var from the channel name lookup
		return channelURL, channelFound
	}

	return stableURL, false
}

// GetPackageFromEnv looks at environment variabes to select the Agent MSI URL.
//
// The returned Package contains the MSI URL and other identifying information.
// Some Package fields will be populated but may not be related to the returned URL.
// For example, if a URL is provided directly, the Channel, Version, Arch, and Flavor fields
// have no effect on the returned URL. They are returned anyway so they can be used for
// other purposes, such as logging, stack name, instance options, test assertions, etc.
//
// These environment variables are mutually exclusive, only one should be set, listed here in the order they are considered:
//
// WINDOWS_AGENT_MSI_URL: manually provided URL (all other parameters are informational only)
//
// E2E_PIPELINE_ID: use the URL from a specific CI pipeline, major version and arch are used, channel is blank
//
// WINDOWS_AGENT_VERSION: The complete version, e.g. 7.49.0-1, 7.49.0-rc.3-1, or a major version, e.g. 7, arch and channel are used
//
// Other environment variables:
//
// WINDOWS_AGENT_CHANNEL: beta or stable
//
// WINDOWS_AGENT_ARCH: The arch of the agent, x86_64
//
// WINDOWS_AGENT_FLAVOR: The flavor of the agent, base or fips
//
// If a channel is not provided and the version contains `-rc.`, the beta channel is used.
//
// See other Lookup*FromEnv functions for more options and details.
//
// If none of the above are set, the latest stable version is used.
func GetPackageFromEnv() (*Package, error) {
	// Collect env opts
	channel, channelFound := LookupChannelFromEnv()
	version, _ := LookupVersionFromEnv()
	arch, _ := LookupArchFromEnv()
	flavor, _ := LookupFlavorFromEnv()
	pipelineID, pipelineIDFound := os.LookupEnv("E2E_PIPELINE_ID")

	majorVersion := strings.Split(version, ".")[0]

	var err error

	if !channelFound {
		// if channel is not provided, check if we can infer it from the version,
		// If version contains `-rc.`, assume it is a beta version
		if strings.Contains(strings.ToLower(version), `-rc.`) {
			channel = betaChannel
		}
	}

	// If a direct URL is provided, use it
	url := os.Getenv("WINDOWS_AGENT_MSI_URL")
	if url != "" {
		return &Package{
			Channel: channel,
			Version: version,
			Arch:    arch,
			URL:     url,
			Flavor:  flavor,
		}, nil
	}

	// check if we should use the URL from a specific CI pipeline
	if pipelineIDFound {
		url, err := GetPipelineMSIURL(pipelineID, majorVersion, arch, flavor, "")
		if err != nil {
			return nil, err
		}
		return &Package{
			PipelineID: pipelineID,
			Version:    version,
			Arch:       arch,
			URL:        url,
			Flavor:     flavor,
		}, nil
	}

	// if version is a complete version, e.g. 7.49.0-1, use it as is
	if strings.Contains(version, ".") {
		// if channel URL or name is provided, lookup its URL
		channelURL, channelURLFound := LookupChannelURLFromEnv()
		if !channelURLFound {
			channelURL, err = GetChannelURL(channel)
			if err != nil {
				return nil, err
			}
		}
		// Get MSI URL
		productName, err := GetFlavorProductName(flavor)
		if err != nil {
			return nil, err
		}
		url, err := installers.GetProductURL(channelURL, productName, version, arch)
		if err != nil {
			return nil, err
		}
		return &Package{
			Channel: channel,
			Version: version,
			Arch:    arch,
			URL:     url,
			Flavor:  flavor,
		}, nil
	}

	// Default to latest stable
	url, err = GetLatestMSIURL(majorVersion, arch, flavor)
	if err != nil {
		return nil, err
	}
	return &Package{
		Channel: stableChannel,
		Version: version,
		Arch:    arch,
		URL:     url,
		Flavor:  flavor,
	}, nil
}

// GetLastStablePackageFromEnv returns the latest stable agent MSI URL.
//
// These environment variables are mutually exclusive, only one should be set, listed here in the order they are considered:
//
// LAST_STABLE_WINDOWS_AGENT_MSI_URL: manually provided URL (all other parameters are informational only)
//
// LAST_STABLE_VERSION: The complete version, e.g. 7.49.0-1, 7.49.0-rc.3-1, or a major version, e.g. 7, arch and channel are used
//
// The value of LAST_STABLE_VERSION is set in release.json, and can be acquired by running:
// invoke release.get-release-json-value "last_stable::$AGENT_MAJOR_VERSION"
func GetLastStablePackageFromEnv() (*Package, error) {
	arch, _ := LookupArchFromEnv()
	flavor, _ := LookupFlavorFromEnv()
	ver := os.Getenv("LAST_STABLE_VERSION")
	if ver == "" {
		return nil, errors.New("LAST_STABLE_VERSION is not set")
	}
	// TODO: Append -1, should we update release.json to include it?
	ver = ver + "-1"

	var err error

	url := os.Getenv("LAST_STABLE_WINDOWS_AGENT_MSI_URL")
	if url == "" {
		// Manual URL not provided, lookup the URL using the version
		url, err = GetStableMSIURL(ver, arch, flavor)
		if err != nil {
			return nil, err
		}
	}

	return &Package{
		Channel: stableChannel,
		Version: ver,
		Arch:    arch,
		URL:     url,
		Flavor:  flavor,
	}, nil
}

// GetUpgradeTestPackageFromEnv returns the upgrade test package to use in upgrade test
//
// UPGRADABLE_WINDOWS_AGENT_MSI_URL: manually provided URL (all other parameters are informational only)
func GetUpgradeTestPackageFromEnv() (*Package, error) {
	// Collect env opts
	// version string will be same as main pipeline agent as the build does not change the emebeded versions
	version, _ := LookupVersionFromEnv()
	arch, _ := LookupArchFromEnv()
	flavor, _ := LookupFlavorFromEnv()
	pipelineID, pipelineIDFound := os.LookupEnv("E2E_PIPELINE_ID")

	majorVersion := strings.Split(version, ".")[0]

	// if UPGRADABLE_WINDOWS_AGENT_MSI_URL provided use it
	url := os.Getenv("UPGRADABLE_WINDOWS_AGENT_MSI_URL")
	if url != "" {
		return &Package{
			Channel: stableChannel,
			Version: version,
			Arch:    arch,
			URL:     url,
			Flavor:  flavor,
		}, nil
	}

	// check if we should use the URL from a specific CI pipeline
	if pipelineIDFound {
		url, err := GetPipelineMSIURL(pipelineID, majorVersion, arch, flavor, "-upgrade-test")
		if err != nil {
			return nil, err
		}
		return &Package{
			PipelineID: pipelineID,
			Version:    version,
			Arch:       arch,
			URL:        url,
			Flavor:     flavor,
		}, nil
	}

	// if not in pipeline or provided in env, then fail
	return nil, errors.New("no upgradable package found")
}

// PackageOption defines a function type for modifying a Package
type PackageOption func(*Package) error

// NewPackage creates a new Package with the provided options
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
	return pkg, nil
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

// WithURLFromPipeline gets the Agent MSI URL from the pipeline
func WithURLFromPipeline(pipelineID string) PackageOption {
	return func(p *Package) error {
		url, err := pipeline.GetPipelineArtifact(pipelineID, pipeline.AgentS3BucketTesting, pipeline.DefaultMajorVersion, func(artifact string) bool {
			return strings.Contains(artifact, p.Product) && strings.HasSuffix(artifact, ".msi")
		})
		if err != nil {
			return err
		}
		p.PipelineID = pipelineID
		p.URL = url
		return nil
	}
}

// WithURLFromInstallersJSON gets the Agent MSI URL from the installers JSON
//
// Note: Depends on the Product and Arch fields. Ensure they are set first when using non-default values.
func WithURLFromInstallersJSON(jsonURL, version string) PackageOption {
	return func(p *Package) error {
		if p.Product == "" {
			return errors.New("product must be set before calling WithURLFromInstallersJSON")
		}
		if p.Arch == "" {
			return errors.New("arch must be set before calling WithURLFromInstallersJSON")
		}
		url, err := installers.GetProductURL(jsonURL, p.Product, version, p.Arch)
		if err != nil {
			return err
		}
		p.Version = version
		p.URL = url
		return nil
	}
}

// WithDevEnvOverrides creates a Package with development environment overrides
//
// Example: local MSI package file
//
//	export CURRENT_AGENT_MSI_URL="file:///path/to/msi/package.msi"
//
// Example: from a different pipeline
//
//	export CURRENT_AGENT_MSI_PIPELINE="123456"
//
// Example: stable version from installers_v2.json
//
//	export CURRENT_AGENT_MSI_VERSION="7.65.0-1"
//
// Example: beta version from installers_v2.json
//
//	export CURRENT_AGENT_MSI_VERSION="7.65.0-rc.1-1"
//
// Example: custom build from installers_v2.json
//
//	export CURRENT_AGENT_MSI_CHANNEL="beta"
//	export CURRENT_AGENT_MSI_VERSION="7.65.0-custombuild-1"
//
// Example: custom URL
//
//	export CURRENT_AGENT_MSI_URL="https://s3.amazonaws.com/dd-agent-mstesting/builds/beta/ddagent-cli-7.64.0-rc.9.msi"
func WithDevEnvOverrides(devenvPrefix string) PackageOption {
	return func(p *Package) error {
		if flavor, ok := os.LookupEnv(devenvPrefix + "_MSI_FLAVOR"); ok {
			if err := WithFlavor(flavor)(p); err != nil {
				return err
			}
		}
		if product, ok := os.LookupEnv(devenvPrefix + "_MSI_PRODUCT"); ok {
			if err := WithProduct(product)(p); err != nil {
				return err
			}
		}
		if arch, ok := os.LookupEnv(devenvPrefix + "_MSI_ARCH"); ok {
			if err := WithArch(arch)(p); err != nil {
				return err
			}
		}
		if channel, ok := os.LookupEnv(devenvPrefix + "_MSI_CHANNEL"); ok {
			if err := WithChannel(channel)(p); err != nil {
				return err
			}
		}
		if version, ok := os.LookupEnv(devenvPrefix + "_MSI_VERSION"); ok {
			if p.Channel == "" {
				channel := stableChannel
				// if channel is not provided, check if we can infer it from the version,
				// If version contains `-rc.`, assume it is a beta version
				if strings.Contains(strings.ToLower(version), `-rc.`) {
					channel = betaChannel
				}
				if err := WithChannel(channel)(p); err != nil {
					return err
				}
			}
			jsonURL, err := GetChannelURL(p.Channel)
			if err != nil {
				return err
			}
			if customJSONURL, ok := os.LookupEnv(devenvPrefix + "_MSI_JSON_URL"); ok {
				jsonURL = customJSONURL
			}
			if err := WithURLFromInstallersJSON(jsonURL, version)(p); err != nil {
				return err
			}
		}
		if url, ok := os.LookupEnv(devenvPrefix + "_MSI_URL"); ok {
			if err := WithURL(url)(p); err != nil {
				return err
			}
		}
		if pipelineID, ok := os.LookupEnv(devenvPrefix + "_MSI_PIPELINE"); ok {
			if err := WithURLFromPipeline(pipelineID)(p); err != nil {
				return err
			}
		}
		return nil
	}
}
