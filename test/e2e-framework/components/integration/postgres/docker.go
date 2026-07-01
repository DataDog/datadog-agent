// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package postgres provides an Agent E2E integration component that runs a
// PostgreSQL 16 server plus a continuous SQL workload generator via Docker
// Compose on a remote host. It is used by the aws/integrations/postgres scenario to give the
// host-installed Datadog Agent's postgres check a live backend to monitor over
// TCP at localhost:5432.
package postgres

import (
	_ "embed"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Host directory where the lab's Compose support files are staged before the
// Compose stack binds them into the postgres containers.
const labAssetDir = "/opt/datadog-postgres-lab"

//go:embed docker-compose.yaml
var dockerComposeYAML string

//go:embed postgres-init.sql
var initSQL string

//go:embed postgres-workload.sh
var workloadScript string

// Monitoring connection facts shared with the scenario so the Agent check
// configuration stays in sync with the seeded database.
const (
	// MonitorUser is the PostgreSQL user the Datadog Agent connects as. It is
	// created by postgres-init.sql and granted pg_monitor.
	MonitorUser = "datadog"
	// MonitorPassword is the password for MonitorUser. Lab-only credential; it
	// matches the value seeded in postgres-init.sql and config/postgres.yaml.
	MonitorPassword = "datadog_monitor_pw"
	// MonitorDBName is the database the check connects to.
	MonitorDBName = "labdb"
	// MonitorPort is the TCP port the server is published on (host loopback).
	MonitorPort = 5432
)

// NewDockerCompose stages the seed SQL and workload script on the host and
// returns a Compose manifest that runs PostgreSQL 16 and the workload
// generator. The returned resources must be set as Compose dependencies (the
// docker agent params / manager handle that) so the bind-mounted files exist
// before the stack starts.
//
// The manifest binds the staged files via absolute host paths, so it is
// independent of the Compose working directory chosen by the docker manager.
func NewDockerCompose(manager *docker.Manager) (docker.ComposeInlineManifest, []pulumi.Resource, error) {
	host := manager.Host

	deps, err := stageAssets(host)
	if err != nil {
		return docker.ComposeInlineManifest{}, nil, err
	}

	manifest := docker.ComposeInlineManifest{
		Name:    "postgres",
		Content: pulumi.String(renderCompose()),
	}
	return manifest, deps, nil
}

// stageAssets writes the init SQL and workload script to the host so the
// Compose bind mounts resolve to real files.
func stageAssets(host *remoteComp.Host) ([]pulumi.Resource, error) {
	fm := host.OS.FileManager()

	mkdir, err := fm.CreateDirectory(labAssetDir, true)
	if err != nil {
		return nil, err
	}

	initCmd, err := fm.CopyInlineFile(
		pulumi.String(initSQL),
		labAssetDir+"/postgres-init.sql",
		pulumi.DependsOn([]pulumi.Resource{mkdir}),
	)
	if err != nil {
		return nil, err
	}

	workloadCmd, err := fm.CopyInlineFile(
		pulumi.String(workloadScript),
		labAssetDir+"/postgres-workload.sh",
		pulumi.DependsOn([]pulumi.Resource{mkdir}),
	)
	if err != nil {
		return nil, err
	}

	return []pulumi.Resource{mkdir, initCmd, workloadCmd}, nil
}

// renderCompose rewrites the embedded compose so the relative bind mounts point
// at the absolute staged paths on the host.
func renderCompose() string {
	out := dockerComposeYAML
	out = strings.ReplaceAll(out, "./postgres-init.sql", labAssetDir+"/postgres-init.sql")
	out = strings.ReplaceAll(out, "./postgres-workload.sh", labAssetDir+"/postgres-workload.sh")
	return out
}
