// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package milvus

import (
	_ "embed"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed docker-compose.yaml
var dockerComposeYAML string

var DockerComposeManifest = docker.ComposeInlineManifest{
	Name:    "milvus",
	Content: pulumi.String(dockerComposeYAML),
}
