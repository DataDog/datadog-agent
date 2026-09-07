// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2host

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

func TestExecutorParamsForwardsFakeintake(t *testing.T) {
	enabled, disabled := true, false
	for _, tc := range []struct {
		name  string
		value *bool
		want  bool
	}{
		{name: "default", want: true},
		{name: "enabled", value: &enabled, want: true},
		{name: "disabled", value: &disabled, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			section := []byte("os: ubuntu-22.04\narch: amd64\ninstance-type: t3.medium\n")
			cfg := &config.File{Environment: config.Environment{Section: bytes.Clone(section), FakeIntake: tc.value}}
			encoded, err := executorParams(cfg)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Section    `yaml:",inline"`
				FakeIntake *bool `yaml:"fakeintake"`
			}
			if err := config.StrictDecode([]byte(encoded), &got); err != nil {
				t.Fatal(err)
			}
			if got.FakeIntake == nil || *got.FakeIntake != tc.want {
				t.Fatalf("fakeintake toggle was not explicitly forwarded: %v", got.FakeIntake)
			}
			if got.OS != "ubuntu-22.04" || got.Arch != "amd64" || got.InstanceType != "t3.medium" {
				t.Fatalf("host-specific parameters were lost: %+v", got.Section)
			}
			if !bytes.Equal(cfg.Environment.Section, section) {
				t.Fatal("encoding the executor request modified the user's section")
			}
		})
	}
}

func TestExecutorParamsRejectsFakeintakeInHostSection(t *testing.T) {
	cfg := &config.File{Environment: config.Environment{
		Section: []byte("os: ubuntu-22.04\narch: amd64\nfakeintake: false\n"),
	}}
	if _, err := executorParams(cfg); err == nil {
		t.Fatal("the host section must not override environment.fakeintake")
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
