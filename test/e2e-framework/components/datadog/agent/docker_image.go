// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agent

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"

	"github.com/Masterminds/semver/v3"
)

const (
	defaultAgentImageTag   = "latest"
	defaultAgent6ImageTag  = "6"
	defaultOTAgentImageTag = "nightly-full-main-jmx"
	jmxSuffix              = "-jmx"
	otelSuffix             = "-7-full"
	otelFIPSSuffix         = "-7-fips-full"
	fipsSuffix             = "-fips"
	linuxOnlySuffix        = "-linux"
)

func defaultAgentImageRepo(e config.Env) string {
	return e.DatadogPublicRegistry() + "/agent"
}

func defaultClusterAgentImageRepo(e config.Env) string {
	return e.DatadogPublicRegistry() + "/cluster-agent"
}

func defaultOTelAgentGatewayImageRepo(e config.Env) string {
	return e.DatadogPublicRegistry() + "/ddot-collector"
}

// defaultDevAgentImageRepo holds the FIPS and OTel agent variants. Its tags float on
// main rather than tracking a build, so this is only a fallback for runs that have no
// pipeline image to use. These images are only published to Docker Hub, so they resolve
// through the Docker Hub mirror rather than [config.Env.DatadogPublicRegistry].
func defaultDevAgentImageRepo(e config.Env) string {
	return e.InternalDockerhubMirror() + "/datadog/agent-dev"
}

// DockerAgentFullImagePath resolves the node-agent image using the standard
// environment settings (fullImagePath → pipeline+SHA → version → latest).
func DockerAgentFullImagePath(e config.Env) string {
	return dockerAgentFullImagePath(e, "", "", false, false, false, false)
}

// DockerClusterAgentFullImagePath resolves the cluster-agent image using the
// standard environment settings (fullImagePath → pipeline+SHA → version → latest).
func DockerClusterAgentFullImagePath(e config.Env) string {
	return dockerClusterAgentFullImagePath(e, "", "", false)
}

// dockerAgentFullImagePath resolves the node-agent image. Precedence:
// an explicit imageTag > the environment-level full image path > pipeline+SHA >
// the environment-level version > latest. That is, a caller asking for a specific
// tag outranks anything derived from the environment.
//
// An explicit imageTag is used verbatim: otel/fips may still swap in the dev repository,
// but no flag ever appends a suffix to it, so the caller owns the whole tag (including
// "-fips" and "-jmx" when it wants a variant image). Those flags only drive the default
// tag when imageTag is empty.
func dockerAgentFullImagePath(e config.Env, repositoryPath, imageTag string, otel bool, fips bool, jmx bool, windowsImage bool) string {
	if e.AgentFullImagePath() != "" && imageTag == "" {
		return e.AgentFullImagePath()
	}

	useOtel := otel
	useFIPS := fips || e.AgentFIPS()
	useJMX := jmx
	useLinuxOnly := e.AgentLinuxOnly() && !windowsImage

	// if agent pipeline id and commit sha are defined, use the image from the pipeline pushed on agent QA registry
	if e.PipelineID() != "" && e.CommitSHA() != "" && imageTag == "" {
		tag := fmt.Sprintf("%s-%s", e.PipelineID(), e.CommitSHA())
		switch {
		case useOtel && useFIPS:
			// OTel full images are already Linux-only and include JMX.
			tag += otelFIPSSuffix
		case useLinuxOnly && useFIPS && useJMX:
			tag += fipsSuffix + linuxOnlySuffix + jmxSuffix
		case useLinuxOnly && useFIPS:
			tag += fipsSuffix + linuxOnlySuffix
		case useOtel:
			// OTel images (-7-full) are already Linux-only and have jmx
			tag += otelSuffix
		case useFIPS && useJMX:
			tag += fipsSuffix + jmxSuffix
		case useFIPS:
			tag += fipsSuffix
		case useLinuxOnly && useJMX:
			tag += linuxOnlySuffix + jmxSuffix
		case useLinuxOnly:
			tag += linuxOnlySuffix
		case useJMX:
			tag += jmxSuffix
		}

		exists, err := e.InternalRegistryImageTagExists(fmt.Sprintf("%s/agent-qa", e.InternalRegistry()), tag)
		if err != nil || !exists {
			panic(fmt.Sprintf("image %s/agent-qa:%s not found in the internal registry", e.InternalRegistry(), tag))
		}
		return utils.BuildDockerImagePath(fmt.Sprintf("%s/agent-qa", e.InternalRegistry()), tag)
	}

	if useOtel {
		if repositoryPath == "" {
			repositoryPath = defaultDevAgentImageRepo(e)
		}
		if imageTag == "" {
			imageTag = defaultOTAgentImageTag
		}

		e.Ctx().Log.Info("The following image will be used in your test: "+fmt.Sprintf("%s:%s", repositoryPath, imageTag), nil)
		return utils.BuildDockerImagePath(repositoryPath, imageTag)
	}

	if useFIPS {
		if repositoryPath == "" {
			repositoryPath = defaultDevAgentImageRepo(e)
		}
		if imageTag == "" {
			if useJMX {
				imageTag = "main" + fipsSuffix + jmxSuffix
			} else {
				imageTag = "main" + fipsSuffix
			}
		}
		e.Ctx().Log.Info("The following image will be used in your test: "+fmt.Sprintf("%s:%s", repositoryPath, imageTag), nil)
		return utils.BuildDockerImagePath(repositoryPath, imageTag)
	}

	if repositoryPath == "" {
		repositoryPath = defaultAgentImageRepo(e)
	}

	if imageTag == "" {
		imageTag = dockerAgentImageTag(e, config.AgentSemverVersion)
		if useJMX {
			imageTag += jmxSuffix
		}
	}

	e.Ctx().Log.Info("The following image will be used in your test: "+fmt.Sprintf("%s:%s", repositoryPath, imageTag), nil)
	return utils.BuildDockerImagePath(repositoryPath, imageTag)
}

// dockerClusterAgentFullImagePath resolves the cluster-agent image, with the same
// precedence as [dockerAgentFullImagePath]: an explicit imageTag > the
// environment-level full image path > pipeline+SHA > the environment-level version.
// An explicit imageTag is likewise used verbatim, fips only selecting the repository
// and the default tag.
func dockerClusterAgentFullImagePath(e config.Env, repositoryPath, imageTag string, fips bool) string {
	if e.ClusterAgentFullImagePath() != "" && imageTag == "" {
		return e.ClusterAgentFullImagePath()
	}

	useFips := fips || e.AgentFIPS()

	// if agent pipeline id and commit sha are defined, use the image from the pipeline pushed on agent QA registry
	if e.PipelineID() != "" && e.CommitSHA() != "" && imageTag == "" {
		tag := fmt.Sprintf("%s-%s", e.PipelineID(), e.CommitSHA())

		if e.AgentFIPS() {
			tag += fipsSuffix
		}

		exists, err := e.InternalRegistryImageTagExists(fmt.Sprintf("%s/cluster-agent-qa", e.InternalRegistry()), tag)
		if err != nil || !exists {
			panic(fmt.Sprintf("image %s/cluster-agent-qa:%s not found in the internal registry", e.InternalRegistry(), tag))
		}
		return utils.BuildDockerImagePath(fmt.Sprintf("%s/cluster-agent-qa", e.InternalRegistry()), tag)
	}

	if useFips {
		if repositoryPath == "" {
			repositoryPath = defaultDevAgentImageRepo(e)
		}
		if imageTag == "" {
			imageTag = "main" + fipsSuffix
		}
		e.Ctx().Log.Info("The following image will be used for dca in your test: "+fmt.Sprintf("%s:%s", repositoryPath, imageTag), nil)
		return utils.BuildDockerImagePath(repositoryPath, imageTag)
	}

	if repositoryPath == "" {
		repositoryPath = defaultClusterAgentImageRepo(e)
	}

	if imageTag == "" {
		imageTag = dockerAgentImageTag(e, config.ClusterAgentSemverVersion)
	}

	return utils.BuildDockerImagePath(repositoryPath, imageTag)
}

func dockerOTelAgentGatewayFullImagePath(e config.Env, repositoryPath, imageTag string) string {
	// if agent pipeline id and commit sha are defined, use the image from the pipeline pushed on agent QA registry
	if e.PipelineID() != "" && e.CommitSHA() != "" && imageTag == "" {
		tag := fmt.Sprintf("%s-%s", e.PipelineID(), e.CommitSHA())

		exists, err := e.InternalRegistryImageTagExists(fmt.Sprintf("%s/otel-agent-qa", e.InternalRegistry()), tag)
		if err != nil || !exists {
			panic(fmt.Sprintf("image %s/otel-agent-qa:%s not found in the internal registry", e.InternalRegistry(), tag))
		}
		return utils.BuildDockerImagePath(fmt.Sprintf("%s/otel-agent-qa", e.InternalRegistry()), tag)
	}

	if repositoryPath == "" {
		repositoryPath = defaultOTelAgentGatewayImageRepo(e)
	}

	if imageTag == "" {
		imageTag = dockerAgentImageTag(e, config.AgentSemverVersion)
	}

	e.Ctx().Log.Info("The following image will be used in your test: "+fmt.Sprintf("%s:%s", repositoryPath, imageTag), nil)
	return utils.BuildDockerImagePath(repositoryPath, imageTag)
}

func dockerAgentImageTag(e config.Env, semverVersion func(config.Env) (*semver.Version, error)) string {
	// default tag
	var agentImageTag string
	if e.MajorVersion() == "6" {
		agentImageTag = defaultAgent6ImageTag
	} else {
		agentImageTag = defaultAgentImageTag
	}

	// try parse agent version
	agentVersion, err := semverVersion(e)
	if agentVersion != nil && err == nil {
		agentImageTag = agentVersion.String()
	} else {
		e.Ctx().Log.Debug("Unable to parse agent version, using latest", nil)
	}

	return agentImageTag
}
