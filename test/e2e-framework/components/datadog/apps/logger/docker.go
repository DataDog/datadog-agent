// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logger

import (
	_ "embed"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed docker-compose.yaml
var dockerComposeContent string

var DockerComposeManifest = docker.ComposeInlineManifest{
	Name:    "logger-test",
	Content: pulumi.String(strings.ReplaceAll(dockerComposeContent, "{APPS_VERSION}", apps.Version)),
}
