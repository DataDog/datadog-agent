package logger

import (
	_ "embed"
	"strings"

	"github.com/DataDog/test-infra-definitions/components/datadog/apps"
	"github.com/DataDog/test-infra-definitions/components/docker"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed docker-compose.yaml
var dockerComposeContent string

var DockerComposeManifest = docker.ComposeInlineManifest{
	Name:    "logger-test",
	Content: pulumi.String(strings.ReplaceAll(dockerComposeContent, "{APPS_VERSION}", apps.Version)),
}
