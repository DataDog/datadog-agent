// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

// Package ec2host is the EC2 host driver. The Pulumi executor provisions the
// VM and, when enabled, the framework's ECS Fargate fakeintake. The core reads
// their connection outputs from the snapshot and installs the Agent separately
// through the non-Pulumi installer. No container runtime is needed on the VM.
package ec2host

import (
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"

	"go.yaml.in/yaml/v3"
)

// Driver is the EC2 host driver.
type Driver struct{}

// ID implements driver.Driver.
func (d *Driver) ID() string { return workerclient.BaseEC2Host }

// Section is the host-specific config (`environment.ec2-host`). executorParams
// forwards these fields together with the common environment.fakeintake option.
type Section struct {
	OS           string `yaml:"os"`
	Arch         string `yaml:"arch"`
	InstanceType string `yaml:"instance-type,omitempty"`
}

// SupportedOS/Arch are the driver's own tables; they never leak into the core.
var (
	supportedOS   = []string{"ubuntu-22.04", "ubuntu-24.04"}
	supportedArch = []string{"amd64", "arm64"}
)

var stackNameRegexp = regexp.MustCompile(`[^a-zA-Z0-9-_.]`)

// Validate implements driver.Driver.
func (d *Driver) Validate(cfg *config.File) []error {
	if cfg.Environment.Section == nil {
		return []error{fmt.Errorf("environment.ec2-host: section required (os and arch)")}
	}
	var s Section
	if err := config.StrictDecode(cfg.Environment.Section, &s); err != nil {
		return []error{fmt.Errorf("environment.ec2-host: %v", err)}
	}
	var errs []error
	if !contains(supportedOS, s.OS) {
		errs = append(errs, fmt.Errorf("environment.ec2-host.os: %q is not supported (supported: %s)", s.OS, join(supportedOS)))
	}
	if !contains(supportedArch, s.Arch) {
		errs = append(errs, fmt.Errorf("environment.ec2-host.arch: %q is not supported (supported: %s)", s.Arch, join(supportedArch)))
	}
	return errs
}

// Installers implements driver.Driver: the shared install-script installer.
func (d *Driver) Installers() []installer.Installer {
	return []installer.Installer{&installer.HostScript{}}
}

// Start provisions the VM and optional fakeintake together through Pulumi.
// Agent installation is a separate, non-Pulumi operation.
func (d *Driver) Start(cfg *config.File, entry envstore.Entry, store *envstore.Store) error {
	params, err := executorParams(cfg)
	if err != nil {
		return err
	}
	stack := stackName(entry.Name) // deterministic: recomputed at stop time, never stored

	fmt.Println("provisioning EC2 host (Pulumi executor), this takes a few minutes...")
	if err := workerclient.Run(entry.Dir, workerclient.Job{
		Action:    workerclient.ActionProvision,
		Base:      d.ID(),
		Params:    params,
		StackName: stack,
		EnvDir:    entry.Dir,
	}); err != nil {
		entry.Meta.Status = envstore.StatusError
		_ = store.UpdateMeta(entry)
		return err
	}

	if err := readFakeintakeOutput(entry, cfg.FakeIntakeEnabled(), &entry.Meta); err != nil {
		entry.Meta.Status = envstore.StatusError
		_ = store.UpdateMeta(entry)
		return err
	}

	entry.Meta.Status = envstore.StatusReady
	return store.UpdateMeta(entry)
}

// Stop implements driver.Driver.
func (d *Driver) Stop(entry envstore.Entry, store *envstore.Store) error {
	cfg, err := entry.LoadConfig()
	if err != nil {
		return err
	}
	params, err := executorParams(cfg)
	if err != nil {
		return err
	}
	fmt.Println("destroying EC2 host and its fakeintake (Pulumi executor)...")
	if err := workerclient.Run(entry.Dir, workerclient.Job{
		Action:    workerclient.ActionDestroy,
		Base:      d.ID(),
		Params:    params,
		StackName: stackName(entry.Name),
		EnvDir:    entry.Dir,
	}); err != nil {
		return err
	}
	return store.Delete(entry.Name)
}

// executorParams forwards the common fakeintake option as part of this
// scenario's parameters. The generic executor job does not gain an EC2 field,
// and environment.ec2-host remains limited to host-specific settings.
func executorParams(cfg *config.File) (string, error) {
	var params struct {
		Section    `yaml:",inline"`
		FakeIntake bool `yaml:"fakeintake"`
	}
	if err := config.StrictDecode(cfg.Environment.Section, &params.Section); err != nil {
		return "", fmt.Errorf("environment.ec2-host: %w", err)
	}
	params.FakeIntake = cfg.FakeIntakeEnabled()
	data, err := yaml.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("encoding EC2 scenario parameters: %w", err)
	}
	return string(data), nil
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

// stackName derives the Pulumi stack name deterministically from the
// environment name: nothing to store, rebuildable at stop time.
func stackName(name string) string {
	return "e2ectl-" + stackNameRegexp.ReplaceAllString(name, "-")
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func join(list []string) string {
	out := ""
	for i, v := range list {
		if i > 0 {
			out += ", "
		}
		out += v
	}
	return out
}
