// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package dogstatsd provides Pulumi component definitions for deploying DogStatsD test applications.
package dogstatsd

import (
	_ "embed"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed docker-compose.yaml
var dockerComposeContent string

// DockerComposeManifest is the Docker Compose manifest for the DogStatsD test application.
var DockerComposeManifest = docker.ComposeInlineManifest{
	Name:    "dogstatsd-sender",
	Content: pulumi.String(strings.ReplaceAll(dockerComposeContent, "{APPS_VERSION}", apps.Version)),
}
