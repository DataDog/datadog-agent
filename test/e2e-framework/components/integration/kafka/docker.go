// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package kafka exposes the Kafka workload used by the kafka integration E2E
// scenario. It runs a single KRaft-mode Kafka broker in Docker Compose plus
// seed-topic and continuous producer/consumer workload containers, so the
// host-resident Datadog Agent's JMX "kafka" check has a live broker to monitor
// over JMX at localhost:9999.
package kafka

import (
	_ "embed"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed docker-compose.yaml
var dockerComposeYAML string

// DockerComposeManifest deploys a KRaft-mode Kafka broker plus seed-topic and
// continuous producer/consumer workload containers. The compose manifest is
// self-contained (no host bind mounts), so the scenario can pass it straight to
// the docker manager's ComposeStrUp.
var DockerComposeManifest = docker.ComposeInlineManifest{
	Name:    "kafka",
	Content: pulumi.String(dockerComposeYAML),
}
