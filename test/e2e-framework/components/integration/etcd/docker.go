// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package etcd exposes the etcd workload used by the etcd integration E2E scenario.
package etcd

import (
	_ "embed"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Version is the etcd container image tag exercised by the scenario.
const Version = "v3.5.17"

//go:embed docker-compose.yaml
var dockerComposeYAML string

// DockerComposeManifest deploys a single-node etcd plus a load generator that
// continuously exercises the etcd v3 API. The etcd container carries Datadog
// autodiscovery labels so the Docker Agent schedules the `etcd` check against
// its Prometheus endpoint without any host-side conf.d file.
var DockerComposeManifest = docker.ComposeInlineManifest{
	Name:    "etcd",
	Content: pulumi.String(strings.ReplaceAll(dockerComposeYAML, "{ETCD_VERSION}", Version)),
}
