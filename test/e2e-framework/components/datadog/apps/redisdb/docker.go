// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package redisdb defines the Docker Compose manifest for the redisdb
// integration E2E lab. The topology runs a Redis 7.0 master, a streaming
// replica, and a Python load generator that exercises the redisdb check.
//
// Usage – embed the manifest into a DockerHost scenario:
//
//	import "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redisdb"
//
//	// inside a Pulumi program / provisioner:
//	docker.ComposeUp(ctx, redisdb.DockerComposeManifest, ...)
package redisdb

import (
	"embed"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	// LoadAssetsPath is the embedded directory copied to the remote Docker host.
	LoadAssetsPath = "load"
	// RemoteAssetsPath is where the scenario copies embedded load assets.
	RemoteAssetsPath = "/tmp/redisdb-assets"
)

//go:embed docker-compose.yaml
var dockerComposeYAML string

//go:embed load/*
var LoadAssets embed.FS

// DockerComposeManifest is the inline Compose manifest for the redisdb lab.
// It can be passed directly to docker.ComposeUp or used as a
// docker.ComposeInlineManifest value in a scenario.
var DockerComposeManifest = docker.ComposeInlineManifest{
	Name:    "redisdb",
	Content: pulumi.String(dockerComposeYAML),
}
