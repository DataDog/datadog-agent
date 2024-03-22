// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package agent includes helpers related to the Datadog Agent on Windows
package agent

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent/installers/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	defaultMajorVersion           = "7"
	defaultArch                   = "x86_64"
	agentInstallerListProductName = "datadog-agent"
	agentS3BucketRelease          = "ddagent-windows-stable"
	agentS3BucketTesting          = "dd-agent-mstesting"
	betaChannel                   = "beta"
	betaURL                       = "https://s3.amazonaws.com/dd-agent-mstesting/builds/beta/installers_v2.json"
	stableChannel                 = "stable"
	stableURL                     = "https://ddagent-windows-stable.s3.amazonaws.com/installers_v2.json"
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
}

// AgentVersion returns a string containing version number and the pre only, e.g. `0.0.0-beta.1`
func (p *Package) AgentVersion() string {
	// Trim the package suffix and parse the remaining version info
	ver, _ := version.New(strings.TrimSuffix(p.Version, "-1"), "")
	return ver.GetNumberAndPre()
}

// GetBetaMSIURL returns the URL for the beta agent MSI
// majorVersion: 6, 7
// arch: x86_64
func GetBetaMSIURL(version string, arch string) (string, error) {
	return GetMSIURL(betaChannel, version, arch)
}

// GetStableMSIURL returns the URL for the stable agent MSI
// majorVersion: 6, 7
// arch: x86_64
func GetStableMSIURL(version string, arch string) (string, error) {
	return GetMSIURL(stableChannel, version, arch)
}

// GetMSIURL returns the URL for the agent MSI
// channel: beta, stable
// majorVersion: 6, 7
// arch: x86_64
func GetMSIURL(channel string, version string, arch string) (string, error) {
	channelURL, err := GetChannelURL(channel)
	if err != nil {
		return "", err
	}

	return installers.GetProductURL(channelURL, agentInstallerListProductName, version, arch)
}

// GetChannelURL returns the URL for the channel name
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
// majorVersion: 6, 7
// arch: x86_64
func GetLatestMSIURL(majorVersion string, arch string) string {
	// why do we use amd64 for the latest URL and x86_64 everywhere else?
	if arch == "x86_64" {
		arch = "amd64"
	}
	return fmt.Sprintf(`https://s3.amazonaws.com/`+agentS3BucketRelease+`/datadog-agent-%s-latest.%s.msi`,
		majorVersion, arch)
}

// GetPipelineMSIURL returns the URL for the agent MSI built by the pipeline
// majorVersion: 6, 7
// arch: x86_64
func GetPipelineMSIURL(pipelineID string, majorVersion string, arch string) (string, error) {
	// dd-agent-mstesting is a public bucket so we can use anonymous credentials
	config, err := awsConfig.LoadDefaultConfig(context.Background(), awsConfig.WithCredentialsProvider(aws.AnonymousCredentials{}))
	if err != nil {
		return "", err
	}

	s3Client := s3.NewFromConfig(config)

	// Manual URL example: https://s3.amazonaws.com/dd-agent-mstesting?prefix=pipelines/A7/25309493
	result, err := s3Client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
		Bucket: aws.String(agentS3BucketTesting),
		Prefix: aws.String(fmt.Sprintf("pipelines/A%s/%s", majorVersion, pipelineID)),
	})

	if err != nil {
		return "", err
	}

	if len(result.Contents) <= 0 {
		return "", fmt.Errorf("no agent MSI found for pipeline %v", pipelineID)
	}

	// In case there are multiple artifacts, try to match the right one
	// This is only here as a workaround for a CI issue that can cause artifacts
	// from different pipelines to be mixed together. This should be removed once
	// the issue is resolved.
	// TODO: CIREL-1970
	for _, obj := range result.Contents {
		// Example: datadog-agent-7.52.0-1-x86_64.msi
		// Example: datadog-agent-7.53.0-devel.git.512.41b1225.pipeline.30353507-1-x86_64.msi
		if !strings.Contains(*obj.Key, fmt.Sprintf("datadog-agent-%s", majorVersion)) {
			continue
		}
		// Not all pipelines include the pipeline ID in the artifact name, but if it is there then match against it
		if strings.Contains(*obj.Key, "pipeline.") &&
			!strings.Contains(*obj.Key, fmt.Sprintf("pipeline.%s", pipelineID)) {
			continue
		}
		if !strings.Contains(*obj.Key, fmt.Sprintf("-%s.msi", arch)) {
			continue
		}

		return fmt.Sprintf("https://s3.amazonaws.com/%s/%s", agentS3BucketTesting, *obj.Key), nil
	}

	return "", fmt.Errorf("no agent MSI found for pipeline %v and arch %v", pipelineID, arch)
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
// For example, if a URL is provided directly, the Channel, Version, and Arch fields
// have no effect on the returned URL. They are returned anyway so they can be used for
// other purposes, such as logging, stack name, instance options, test assertions, etc.
//
// These environment variables are mutually exclusive, only one should be set, listed here in the order they are considered:
//
// WINDOWS_AGENT_MSI_URL: manually provided URL (all other parameters are informational only)
//
// CI_PIPELINE_ID: use the URL from a specific CI pipeline, major version and arch are used, channel is blank
//
// WINDOWS_AGENT_VERSION: The complete version, e.g. 7.49.0-1, 7.49.0-rc.3-1, or a major version, e.g. 7, arch and channel are used
//
// Other environment variables:
//
// WINDOWS_AGENT_CHANNEL: beta or stable
//
// WINDOWS_AGENT_ARCH: The arch of the agent, x86_64
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
	pipelineID, pipelineIDFound := os.LookupEnv("CI_PIPELINE_ID")

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
		}, nil
	}

	// check if we should use the URL from a specific CI pipeline
	if pipelineIDFound {
		url, err := GetPipelineMSIURL(pipelineID, majorVersion, arch)
		if err != nil {
			return nil, err
		}
		return &Package{
			PipelineID: pipelineID,
			Version:    version,
			Arch:       arch,
			URL:        url,
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
		url, err := installers.GetProductURL(channelURL, agentInstallerListProductName, version, arch)
		if err != nil {
			return nil, err
		}
		return &Package{
			Channel: channel,
			Version: version,
			Arch:    arch,
			URL:     url,
		}, nil
	}

	// Default to latest stable
	url = GetLatestMSIURL(majorVersion, arch)
	return &Package{
		Channel: stableChannel,
		Version: version,
		Arch:    arch,
		URL:     url,
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
	ver := os.Getenv("LAST_STABLE_VERSION")
	if ver == "" {
		return nil, fmt.Errorf("LAST_STABLE_VERSION is not set")
	}
	// TODO: Append -1, should we update release.json to include it?
	ver = fmt.Sprintf("%s-1", ver)

	var err error

	url := os.Getenv("LAST_STABLE_WINDOWS_AGENT_MSI_URL")
	if url == "" {
		// Manual URL not provided, lookup the URL using the version
		url, err = GetStableMSIURL(ver, arch)
		if err != nil {
			return nil, err
		}
	}

	return &Package{
		Channel: stableChannel,
		Version: ver,
		Arch:    arch,
		URL:     url,
	}, nil
}
