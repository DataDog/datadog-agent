// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package apps resolves the container images of the workload apps published by
// test-infra-definitions.
package apps

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
)

const Version = "v0.0.8"

// PublicRegistry is the public registry (ghcr.io) where test-infra-definitions
// publishes the workload apps images. It is used as the fallback when no
// internal registry is available (local runs).
const publicRegistry = "ghcr.io/datadog"

// ImagePath returns the image repository (registry prefix and app repository
// name, without the tag) of a workload app for the given environment.
//
// Images are pulled from the internal registry of the environment when one is
// configured: the agent-qa ECR registry on AWS, the datadog-agent-qa Artifact
// Registry on GCP and the agentqa ACR on Azure. Pulling from the internal
// registries is more reliable than the public ghcr.io registry and keeps e2e
// traffic internal. When the environment has no internal registry (local
// runs, environments without a pull-through cache), the public ghcr.io
// registry is used instead so tests remain runnable without VPN access.
func ImagePath(e config.Env, repo string) string {
	// The local environment reports "none" as its internal registry since it
	// has no access to the internal registries; the placeholder is normalized
	// away so images fall back to the public registry.
	reg := e.InternalRegistry()
	if reg == "none" {
		reg = ""
	}
	return repositoryPath(reg, repo)
}

// repositoryPath prefixes the app repository with the given registry, falling
// back to the public ghcr.io registry when the registry is empty (no internal
// registry available).
func repositoryPath(registry, repo string) string {
	if registry == "" {
		return publicRegistry + "/" + repo
	}
	return registry + "/" + repo
}

// Image returns the full image reference (repository and version tag) of a
// workload app for the given environment. See ImagePath for the registry
// resolution strategy.
func Image(e config.Env, repo string) string {
	return ImagePath(e, repo) + ":" + Version
}
