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
	defaultAgentImageRepo            = "gcr.io/datadoghq/agent"
	defaultClusterAgentImageRepo     = "gcr.io/datadoghq/cluster-agent"
	defaultOTelAgentGatewayImageRepo = "gcr.io/datadoghq/ddot-collector"
	defaultAgentImageTag             = "latest"
	defaultAgent6ImageTag            = "6"
	defaultDevAgentImageRepo         = "datadog/agent-dev" // Used as default repository for images that are not stable and released yet, should not be used in the CI
	defaultOTAgentImageTag           = "nightly-full-main-jmx"
	jmxSuffix                        = "-jmx"
	otelSuffix                       = "-7-full"
	fipsSuffix                       = "-fips"
	linuxOnlySuffix                  = "-linux"
)

func dockerAgentFullImagePath(e config.Env, repositoryPath, imageTag string, otel bool, fips bool, jmx bool, windowsImage bool) string {
	// return agent image path if defined
	if e.AgentFullImagePath() != "" {
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
		case useOtel && useFIPS && useJMX:
			panic("Unsupported: no image with FIPS, JMX and OTel exists yet")
		case useOtel && useFIPS:
			panic("Unsupported: no image with FIPS and OTel exists yet")
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
			repositoryPath = defaultDevAgentImageRepo
		}
		if imageTag == "" {
			imageTag = defaultOTAgentImageTag
		}

		e.Ctx().Log.Info("The following image will be used in your test: "+fmt.Sprintf("%s:%s", repositoryPath, imageTag), nil)
		return utils.BuildDockerImagePath(repositoryPath, imageTag)
	}

	if useFIPS {
		if repositoryPath == "" {
			repositoryPath = defaultDevAgentImageRepo
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
		repositoryPath = defaultAgentImageRepo
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

func dockerClusterAgentFullImagePath(e config.Env, repositoryPath string, fips bool) string {
	// return cluster agent image path if defined
	if e.ClusterAgentFullImagePath() != "" {
		return e.ClusterAgentFullImagePath()
	}

	useFips := fips || e.AgentFIPS()

	// if agent pipeline id and commit sha are defined, use the image from the pipeline pushed on agent QA registry
	if e.PipelineID() != "" && e.CommitSHA() != "" {
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
			repositoryPath = defaultDevAgentImageRepo
		}
		imageTag := "main" + fipsSuffix
		e.Ctx().Log.Info("The following image will be used for dca in your test: "+fmt.Sprintf("%s:%s", repositoryPath, imageTag), nil)
		return utils.BuildDockerImagePath(repositoryPath, imageTag)
	}

	if repositoryPath == "" {
		repositoryPath = defaultClusterAgentImageRepo
	}

	return utils.BuildDockerImagePath(repositoryPath, dockerAgentImageTag(e, config.ClusterAgentSemverVersion))
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
		repositoryPath = defaultOTelAgentGatewayImageRepo
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
