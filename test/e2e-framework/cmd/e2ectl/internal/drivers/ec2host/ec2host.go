// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ec2host drives Pulumi VM/fakeintake provisioning. Its data-only config
// and automatic validation are shared with the executor; Agent installation is
// still a separate operation using the non-Pulumi installer.
package ec2host

import (
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
	ec2config "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/ec2host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

type Driver struct{}

func (d *Driver) ID() string { return workerclient.BaseEC2Host }
func (d *Driver) Description() string {
	return "AWS EC2 VM and optional ECS Fargate fakeintake (Pulumi)"
}

func (d *Driver) Installers() []installer.Installer {
	return []installer.Installer{&installer.HostScript{}}
}

// params is prepared by the generic schema adapter. Forward its normalized YAML
// from cfg: unlike re-marshalling with omitempty, this preserves explicit zeros
// and applied defaults for any future fields. No second DTO is declared here.
// Start provisions through the executor. Failure marking (status=error) is
// owned by the command layer, not repeated here: cmdStart re-reads the
// entry and marks it failed whenever Start returns an error.
func (d *Driver) Start(_ ec2config.Config, cfg *config.File, entry envstore.Entry, store *envstore.Store) error {
	fmt.Println("provisioning EC2 host (Pulumi executor), this takes a few minutes...")
	if err := workerclient.Run(entry.Dir, executorJob(workerclient.ActionProvision, cfg, entry)); err != nil {
		return err
	}
	if err := readFakeintakeOutput(entry, cfg.FakeIntakeEnabled(), &entry.Meta); err != nil {
		return err
	}
	entry.Meta.Status = envstore.StatusReady
	return store.UpdateMeta(entry)
}

func (d *Driver) Stop(_ ec2config.Config, cfg *config.File, entry envstore.Entry, store *envstore.Store) error {
	fmt.Println("destroying EC2 host and its fakeintake (Pulumi executor)...")
	if err := workerclient.Run(entry.Dir, executorJob(workerclient.ActionDestroy, cfg, entry)); err != nil {
		return err
	}
	return store.Delete(entry.Name)
}

func executorJob(action string, cfg *config.File, entry envstore.Entry) workerclient.Job {
	fixtures := cfg.Environment.Fixtures
	return workerclient.Job{
		ProtocolVersion: workerclient.ProtocolVersion,
		Action:          action,
		Base:            workerclient.BaseEC2Host,
		Params:          string(cfg.Environment.Section),
		Fixtures:        &fixtures,
		StackName:       stackName(entry.Name),
		EnvDir:          entry.Dir,
	}
}

// readFakeintakeOutput uses the endpoint exported by the Pulumi scenario,
// including its resource binding. It does not connect to or mutate the VM.
func readFakeintakeOutput(entry envstore.Entry, enabled bool, meta *envstore.Meta) error {
	if !enabled {
		meta.FakeIntakeURL = ""
		meta.FakeIntakePort = 0
		return nil
	}
	var fi outputs.FakeintakeOutput
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "fakeIntake", &fi); err != nil {
		return fmt.Errorf("reading Pulumi-provisioned fakeintake: %w", err)
	}
	if fi.URL == "" {
		return fmt.Errorf("snapshot %s has no URL for the Pulumi-provisioned fakeintake", entry.SnapshotPath())
	}
	meta.FakeIntakeURL = fi.URL
	meta.FakeIntakePort = int(fi.Port)
	return nil
}

var stackNameRegexp = regexp.MustCompile(`[^a-zA-Z0-9-_.]`)

func stackName(name string) string {
	return "e2ectl-" + stackNameRegexp.ReplaceAllString(name, "-")
}
