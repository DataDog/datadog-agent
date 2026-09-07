// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2host

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
	ec2config "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/ec2host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/fixtures"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

func TestTypedExecutorRoundtrip(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      bool
	}{
		{name: "default", want: true},
		{name: "enabled", raw: "fakeintake: true", want: true},
		{name: "disabled", raw: "fakeintake: false", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, normalized, err := ec2config.Schema.Decode([]byte("os: ubuntu-22.04\ninstance-type: t3.medium"), "input")
			if err != nil {
				t.Fatal(err)
			}
			fixtureConfig, _, err := fixtures.Schema.Decode([]byte(tc.raw), "fixtures")
			if err != nil {
				t.Fatal(err)
			}
			cfg := &config.File{Environment: config.Environment{Section: normalized, Fixtures: fixtureConfig}}
			job := executorJob(workerclient.ActionProvision, cfg, envstore.Entry{Name: "test", Dir: t.TempDir()})
			data, err := json.Marshal(job)
			if err != nil {
				t.Fatal(err)
			}
			var received workerclient.Job
			if err := json.Unmarshal(data, &received); err != nil {
				t.Fatal(err)
			}
			got, _, err := ec2config.Schema.Decode([]byte(received.Params), "executor")
			if err != nil || got != p {
				t.Fatalf("typed handoff changed parameters: %+v %v", got, err)
			}
			if got.Arch != "amd64" {
				t.Fatal("declared default was not materialized")
			}
			if received.ProtocolVersion != workerclient.ProtocolVersion || received.Fixtures == nil || received.Fixtures.FakeIntake != tc.want {
				t.Fatalf("fixture/default/version lost in transport: %+v", received)
			}
		})
	}
}

func TestFixtureOptionIsNotAnEC2Field(t *testing.T) {
	if _, _, err := ec2config.Schema.Decode([]byte("os: ubuntu-22.04\nfakeintake: false"), "input"); err == nil {
		t.Fatal("fixture settings belong to their shared schema, not the EC2 type")
	}
}

func TestReadPulumiFakeintakeOutput(t *testing.T) {
	entry := envstore.Entry{Name: "ec2", Dir: t.TempDir()}
	if err := provisioner.WriteSnapshotFile(entry.SnapshotPath(), provisioner.RawResources{
		"dd-Fakeintake-aws-vm": []byte(`{"host":"fakeintake.example.invalid","scheme":"http","port":80,"url":"http://fakeintake.example.invalid:80"}`),
	}, map[string]any{
		"_bindings": map[string]string{"fakeIntake": "dd-Fakeintake-aws-vm"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := readFakeintakeOutput(entry, true, &entry.Meta); err != nil {
		t.Fatal(err)
	}
	if entry.Meta.FakeIntakeURL != "http://fakeintake.example.invalid:80" || entry.Meta.FakeIntakePort != 80 {
		t.Fatalf("metadata does not use the provisioner's endpoint: %+v", entry.Meta)
	}
}

func TestReadFakeintakeDisabled(t *testing.T) {
	entry := envstore.Entry{Dir: t.TempDir(), Meta: envstore.Meta{FakeIntakeURL: "old", FakeIntakePort: 80}}
	// No file exists. Disabled fakeintake needs no snapshot lookup or network access.
	if err := readFakeintakeOutput(entry, false, &entry.Meta); err != nil {
		t.Fatal(err)
	}
	if entry.Meta.FakeIntakeURL != "" || entry.Meta.FakeIntakePort != 0 {
		t.Fatal("disabled fakeintake retained a stale endpoint")
	}
}

func TestReadFakeintakeMissingOrIncomplete(t *testing.T) {
	for name, resources := range map[string]provisioner.RawResources{
		"missing":    {"remoteHost": []byte(`{}`)},
		"incomplete": {"fakeIntake": []byte(`{"port":80}`)},
	} {
		t.Run(name, func(t *testing.T) {
			entry := envstore.Entry{Dir: t.TempDir()}
			if err := provisioner.WriteSnapshotFile(entry.SnapshotPath(), resources, nil); err != nil {
				t.Fatal(err)
			}
			if err := readFakeintakeOutput(entry, true, &entry.Meta); err == nil || !strings.Contains(strings.ToLower(err.Error()), "fakeintake") {
				t.Fatalf("expected an actionable fakeintake error, got: %v", err)
			}
			if entry.Meta.FakeIntakeURL != "" {
				t.Fatal("a failed read must not invent an endpoint")
			}
		})
	}
}
