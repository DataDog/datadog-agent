// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package jmxfetch

import (
	_ "embed"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed docker-compose.yaml
var dockerComposeContent string

//go:embed docker-compose-all-metrics.yaml
var dockerComposeAllMetricsContent string

//go:embed docker-compose-slow-metrics.yaml
var dockerComposeSlowMetricsContent string

var DockerComposeManifest = docker.ComposeInlineManifest{
	Name:    "jmx-test-app",
	Content: pulumi.String(strings.ReplaceAll(dockerComposeContent, "{APPS_VERSION}", apps.Version)),
}

var DockerComposeAllMetricsManifest = docker.ComposeInlineManifest{
	Name:    "jmx-test-all-metrics",
	Content: pulumi.String(dockerComposeAllMetricsContent),
}

var DockerComposeSlowMetricsManifest = docker.ComposeInlineManifest{
	Name:    "jmx-test-slow-metrics",
	Content: pulumi.String(dockerComposeSlowMetricsContent),
}
