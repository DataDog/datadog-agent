// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package redisdb provides an Agent E2E integration component that runs a
// standalone Redis 7.2 OSS server plus a continuous command workload generator
// via Docker Compose on a remote host. It is used by the aws/integrations/redisdb
// scenario to give the host-installed Datadog Agent's redisdb check a live
// server to monitor over TCP at localhost:6379.
package redisdb

import (
	_ "embed"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Host directory where the lab's Compose support files are staged before the
// Compose stack binds them into the workload container.
const labAssetDir = "/opt/datadog-redisdb-lab"

//go:embed docker-compose.yaml
var dockerComposeYAML string

//go:embed redis-workload.sh
var workloadScript string

// NewDockerCompose stages the workload script on the host and returns a Compose
// manifest that runs Redis 7.2 plus the continuous workload generator. The
// returned resources must be set as Compose dependencies so the bind-mounted
// script exists before the stack starts.
//
// The manifest binds the staged script via an absolute host path, so it is
// independent of the Compose working directory chosen by the docker manager.
func NewDockerCompose(manager *docker.Manager) (docker.ComposeInlineManifest, []pulumi.Resource, error) {
	deps, err := stageAssets(manager.Host)
	if err != nil {
		return docker.ComposeInlineManifest{}, nil, err
	}

	manifest := docker.ComposeInlineManifest{
		Name:    "redisdb",
		Content: pulumi.String(renderCompose()),
	}
	return manifest, deps, nil
}

// stageAssets writes the workload script to the host so the Compose bind mount
// resolves to a real file.
func stageAssets(host *remoteComp.Host) ([]pulumi.Resource, error) {
	fm := host.OS.FileManager()

	mkdir, err := fm.CreateDirectory(labAssetDir, true)
	if err != nil {
		return nil, err
	}

	workloadCmd, err := fm.CopyInlineFile(
		pulumi.String(workloadScript),
		labAssetDir+"/redis-workload.sh",
		pulumi.DependsOn([]pulumi.Resource{mkdir}),
	)
	if err != nil {
		return nil, err
	}

	return []pulumi.Resource{mkdir, workloadCmd}, nil
}

// renderCompose rewrites the embedded compose so the relative bind mount points
// at the absolute staged path on the host.
func renderCompose() string {
	return strings.ReplaceAll(dockerComposeYAML, "./redis-workload.sh", labAssetDir+"/redis-workload.sh")
}
