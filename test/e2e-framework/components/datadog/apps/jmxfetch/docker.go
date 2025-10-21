package jmxfetch

import (
	_ "embed"
	"strings"

	"github.com/DataDog/test-infra-definitions/components/datadog/apps"
	"github.com/DataDog/test-infra-definitions/components/docker"

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
