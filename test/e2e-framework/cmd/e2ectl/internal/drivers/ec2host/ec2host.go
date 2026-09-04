// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

// Package ec2host is the EC2 host driver. The core side is thin and Pulumi-
// free: provisioning runs in the pulumi-executor (the worker) with the
// GENERIC job {action, base, params}; per the infra-only contract the
// executor hands back an empty, connectable VM, and everything afterwards —
// fakeintake deployment, agent install, iteration — happens in the core,
// exactly like every other environment.
package ec2host

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
)

// Driver is the EC2 host driver.
type Driver struct{}

// ID implements driver.Driver.
func (d *Driver) ID() string { return workerclient.BaseEC2Host }

// Section is the driver-owned config section (`environment.ec2-host`).
// It is ALSO the scenario params handed to the executor (one schema: the
// config section IS the run function's params).
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

// Start implements driver.Driver: the executor provisions the VM, then the
// core deploys the fakeintake on it (infra-only executor, §12 of the plan).
func (d *Driver) Start(cfg *config.File, entry envstore.Entry, store *envstore.Store) error {
	meta := entry.Meta
	stack := stackName(entry.Name) // deterministic: recomputed at stop time, never stored

	fmt.Println("provisioning EC2 host (Pulumi executor), this takes a few minutes...")
	if err := workerclient.Run(entry.Dir, workerclient.Job{
		Action:    workerclient.ActionProvision,
		Base:      d.ID(),
		Params:    string(cfg.Environment.Section),
		StackName: stack,
		EnvDir:    entry.Dir,
	}); err != nil {
		entry.Meta.Status = envstore.StatusError
		_ = store.UpdateMeta(entry)
		return err
	}

	if cfg.FakeIntakeEnabled() {
		if err := deployFakeintakeOnHost(entry, &meta); err != nil {
			return fmt.Errorf("deploying the fakeintake on the VM: %w", err)
		}
	}

	meta.Status = envstore.StatusReady
	entry.Meta = meta
	return store.UpdateMeta(entry)
}

// Stop implements driver.Driver.
func (d *Driver) Stop(entry envstore.Entry, store *envstore.Store) error {
	cfg, err := entry.LoadConfig()
	if err != nil {
		return err
	}
	fmt.Println("destroying EC2 host (Pulumi executor)...")
	if err := workerclient.Run(entry.Dir, workerclient.Job{
		Action:    workerclient.ActionDestroy,
		Base:      d.ID(),
		Params:    string(cfg.Environment.Section),
		StackName: stackName(entry.Name),
		EnvDir:    entry.Dir,
	}); err != nil {
		return err
	}
	return store.Delete(entry.Name)
}

// deployFakeintakeOnHost runs the fakeintake container on the VM over the ssh
// connection the snapshot just handed over, and records it in the snapshot
// and meta — the same component, the same lifecycle, as on every environment.
func deployFakeintakeOnHost(entry envstore.Entry, meta *envstore.Meta) error {
	p := provisioner.NewStaticStackProvisioner[environments.Host]("", entry.SnapshotPath())
	ctx := standalone.NewContext(entry.Dir)
	env, _, err := standalone.ProvisionE[environments.Host](ctx, "attach", p)
	if err != nil {
		return fmt.Errorf("attaching to the VM from its snapshot: %w", err)
	}

	const fiPort = 8080 // the fakeintake's container port, mapped on the VM
	cmd := fmt.Sprintf(
		"sudo docker run -d --name e2ectl-fakeintake --restart unless-stopped -p %d:80 %s --rc-key-data=%s",
		fiPort, fakeintakeImage, fakeintakeSeed)
	if _, err := env.RemoteHost.Execute(cmd); err != nil {
		return fmt.Errorf("running the fakeintake container (is docker installed on the VM?): %w", err)
	}

	fiKey, err := json.Marshal(map[string]any{
		"host":   env.RemoteHost.Address,
		"scheme": "http",
		"port":   fiPort,
		"url":    fmt.Sprintf("http://%s:%d", env.RemoteHost.Address, fiPort),
	})
	if err != nil {
		return err
	}
	resources, snapMeta, err := provisioner.ReadSnapshotFile(entry.SnapshotPath())
	if err != nil {
		return err
	}
	resources["fakeIntake"] = fiKey
	anyMeta := make(map[string]any, len(snapMeta))
	for k, v := range snapMeta {
		anyMeta[k] = v
	}
	if err := provisioner.WriteSnapshotFile(entry.SnapshotPath(), resources, anyMeta); err != nil {
		return err
	}
	meta.FakeIntakePort = fiPort
	meta.FakeIntakeURL = fmt.Sprintf("http://%s:%d", env.RemoteHost.Address, fiPort)
	return nil
}

// The image and the RC seed mirror components/outputs (kept in sync; the core
// stays Pulumi-free).
const (
	fakeintakeImage = "public.ecr.aws/datadog/fakeintake:latest"
	fakeintakeSeed  = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
)

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
