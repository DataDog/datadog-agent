// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ec2host drives Pulumi VM/fakeintake provisioning. Its data-only config
// and automatic validation are shared with the executor; Agent installation is
// still a separate operation using the non-Pulumi installer. The lifecycle
// itself is the shared Pulumi-executor one (drivers/pulumiworker).
package ec2host

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/drivers/pulumiworker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
	ec2config "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/ec2host"
)

// Driver is the ec2-host driver: a plain Pulumi-executor lifecycle.
type Driver struct {
	pulumiworker.Lifecycle
}

// New returns the registered ec2-host driver: a VM host with the official
// install script (default) and pipeline DEB installers.
func New() *Driver {
	return &Driver{Lifecycle: *pulumiworker.New(
		workerclient.BaseEC2Host,
		"EC2 host",
		"AWS EC2 VM and optional ECS Fargate fakeintake (Pulumi)",
		[]installer.Installer{&installer.HostScript{}, &installer.Package{}},
		nil,
	)}
}

func (d *Driver) Start(_ ec2config.Config, cfg *config.File, entry envstore.Entry, store *envstore.Store) error {
	return d.Lifecycle.Start(cfg, entry, store)
}

func (d *Driver) Stop(_ ec2config.Config, cfg *config.File, entry envstore.Entry, store *envstore.Store) error {
	return d.Lifecycle.Stop(cfg, entry, store)
}
