// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dockerhost drives the Pulumi ec2docker scenario: an EC2 VM with the
// Docker runtime and optional ECS Fargate fakeintake. Its data-only config and
// AMI validation are shared with the executor; Agent installation is a
// separate operation with the non-Pulumi host installers, exactly like
// ec2-host (the snapshot exports the same SSH remote host).
package dockerhost

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/drivers/pulumiworker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
	dockerconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/dockerhost"
)

// Driver is the docker-host driver: a plain Pulumi-executor lifecycle.
type Driver struct {
	pulumiworker.Lifecycle
}

// New returns the registered docker-host driver. The installers are the SSH
// host ones: the VM is a plain Ubuntu host with Docker, so the install script
// and pipeline DEB paths work unchanged (the package target selects the
// systemd services, not the container core).
func New() *Driver {
	return &Driver{Lifecycle: *pulumiworker.New(
		workerclient.BaseDockerHost,
		"EC2 Docker host",
		"AWS EC2 VM with a Docker runtime and optional ECS Fargate fakeintake (Pulumi)",
		[]installer.Installer{&installer.HostScript{}, &installer.Package{}},
		nil,
	)}
}

func (d *Driver) Start(_ dockerconfig.Config, cfg *config.File, entry envstore.Entry, store *envstore.Store) error {
	return d.Lifecycle.Start(cfg, entry, store)
}

func (d *Driver) Stop(_ dockerconfig.Config, cfg *config.File, entry envstore.Entry, store *envstore.Store) error {
	return d.Lifecycle.Stop(cfg, entry, store)
}
