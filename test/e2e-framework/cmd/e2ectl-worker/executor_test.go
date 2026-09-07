// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
)

type exportedHostEnv struct {
	RemoteHost *outputs.HostOutput
	FakeIntake *outputs.FakeintakeOutput
	Agent      *outputs.HostAgentOutput
}

// Simulates the real Pulumi exporter: it names resources independently of the
// environment fields, and binds the allocated fields with SetKey. It never
// calls AWS or initializes an SSH client.
type hostExporter struct{ fakeintake bool }

func (hostExporter) ID() string                                       { return "test-host-exporter" }
func (hostExporter) Destroy(context.Context, string, io.Writer) error { return nil }
func (p hostExporter) ProvisionEnv(_ context.Context, _ string, _ io.Writer, env *exportedHostEnv) (provisioner.RawResources, error) {
	env.RemoteHost.SetKey("dd-Host-aws-vm")
	env.Agent = nil // agent installation always happens after provisioning
	resources := provisioner.RawResources{
		"dd-Host-aws-vm": []byte(`{"address":"example.invalid","port":22,"username":"ubuntu"}`),
	}
	if p.fakeintake {
		env.FakeIntake.SetKey("dd-Fakeintake-aws-vm")
		resources["dd-Fakeintake-aws-vm"] = []byte(`{"host":"intake.invalid","scheme":"http","port":80,"url":"http://intake.invalid:80"}`)
	} else {
		env.FakeIntake = nil
	}
	return resources, nil
}

func TestExecutorSnapshotCanBeAttached(t *testing.T) {
	for _, tc := range []struct {
		name       string
		fakeintake bool
	}{
		{name: "VM only"},
		{name: "VM and Pulumi fakeintake", fakeintake: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "snapshot.json")
			ctx := standalone.NewContext(dir)
			if err := fromTyped[exportedHostEnv](hostExporter{fakeintake: tc.fakeintake}).Provision(ctx, "test-stack", path); err != nil {
				t.Fatal(err)
			}

			// A new process has no keys from the original environment: both host
			// and fakeintake bindings must survive through the snapshot alone.
			env, err := standalone.Provision[exportedHostEnv](ctx, "attach",
				provisioner.NewStaticStackProvisioner[exportedHostEnv]("", path))
			if err != nil {
				t.Fatal(err)
			}
			if env.RemoteHost == nil {
				t.Fatal("executor snapshot lost its remoteHost binding")
			}
			if env.RemoteHost.Address != "example.invalid" || env.RemoteHost.Port != 22 {
				t.Fatalf("connection details did not survive attachment: %+v", env.RemoteHost)
			}
			if (env.FakeIntake != nil) != tc.fakeintake {
				t.Fatalf("fakeintake presence does not match the scenario: %+v", env.FakeIntake)
			}
			if env.FakeIntake != nil && env.FakeIntake.URL != "http://intake.invalid:80" {
				t.Fatal("fakeintake endpoint did not survive attachment")
			}
			if env.Agent != nil {
				t.Fatal("the infrastructure handoff must not invent an installed Agent")
			}
		})
	}
}
