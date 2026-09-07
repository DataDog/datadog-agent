// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioner

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
)

// These are resolved output holders, not SSH clients: none of these tests
// provision infrastructure or make a network connection.
type snapshotHostEnv struct {
	RemoteHost *outputs.HostOutput
	Peer       *outputs.HostOutput
	FakeIntake *outputs.FakeintakeOutput
}

func TestStaticStackSnapshotBindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	resources := RawResources{
		"dd-Host-aws-vm": []byte(`{"address":"host-one","port":22,"username":"ubuntu"}`),
		"dd-Host-peer":   []byte(`{"address":"host-two","port":22,"username":"ubuntu"}`),
		"fakeIntake":     []byte(`{"url":"http://intake:80"}`),
	}
	meta := map[string]any{
		"_source": "test",
		"_bindings": map[string]string{
			"remoteHost": "dd-Host-aws-vm",
			"peer":       "dd-Host-peer",
		},
	}
	if err := WriteSnapshotFile(path, resources, meta); err != nil {
		t.Fatal(err)
	}

	var env snapshotHostEnv
	p := NewStaticStackProvisioner[snapshotHostEnv]("", path)
	got, err := p.ProvisionEnv(context.Background(), "attach", io.Discard, &env)
	if err != nil {
		t.Fatal(err)
	}
	if env.RemoteHost == nil || env.Peer == nil || env.FakeIntake == nil {
		t.Fatal("snapshot attachment lost components exported with Pulumi resource keys")
	}
	for _, host := range []*outputs.HostOutput{env.RemoteHost, env.Peer} {
		if err := host.Import(got[host.Key()], host); err != nil {
			t.Fatal(err)
		}
	}
	if env.RemoteHost.Address != "host-one" || env.Peer.Address != "host-two" {
		t.Fatalf("incorrect host bindings: %q, %q", env.RemoteHost.Address, env.Peer.Address)
	}
	if env.FakeIntake.Key() != "fakeIntake" {
		t.Fatalf("canonical resources added after provisioning must remain usable: %q", env.FakeIntake.Key())
	}
}

func TestStaticStackRejectsBrokenBindings(t *testing.T) {
	for name, binding := range map[string]any{
		"missing resource": map[string]string{"remoteHost": "missing"},
		"invalid format":   []string{"remoteHost"},
		"empty resource":   map[string]string{"remoteHost": ""},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "snapshot.json")
			if err := WriteSnapshotFile(path, RawResources{"dd-Host-aws-vm": []byte(`{}`)}, map[string]any{"_bindings": binding}); err != nil {
				t.Fatal(err)
			}
			var env snapshotHostEnv
			_, err := NewStaticStackProvisioner[snapshotHostEnv]("", path).ProvisionEnv(context.Background(), "attach", io.Discard, &env)
			if err == nil {
				t.Fatal("expected an actionable error, not an environment with missing components")
			}
		})
	}
}

func TestStaticStackLegacyCanonicalSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := WriteSnapshotFile(path, RawResources{"remoteHost": []byte(`{"address":"host"}`)}, nil); err != nil {
		t.Fatal(err)
	}
	var env snapshotHostEnv
	resources, err := NewStaticStackProvisioner[snapshotHostEnv]("", path).ProvisionEnv(context.Background(), "attach", io.Discard, &env)
	if err != nil {
		t.Fatal(err)
	}
	if env.RemoteHost == nil || env.Peer != nil || env.FakeIntake != nil {
		t.Fatal("legacy canonical snapshots must still attach and omit absent optional components")
	}
	if err := json.Unmarshal(resources[env.RemoteHost.Key()], env.RemoteHost); err != nil {
		t.Fatal(err)
	}
	if env.RemoteHost.Address != "host" {
		t.Fatalf("unexpected address: %q", env.RemoteHost.Address)
	}
}

func TestStaticStackLegacyPulumiSnapshotNeedsBindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := WriteSnapshotFile(path, RawResources{"dd-Host-aws-vm": []byte(`{}`)}, nil); err != nil {
		t.Fatal(err)
	}
	var env snapshotHostEnv
	_, err := NewStaticStackProvisioner[snapshotHostEnv]("", path).ProvisionEnv(context.Background(), "attach", io.Discard, &env)
	if err == nil || !strings.Contains(err.Error(), "_bindings") {
		t.Fatalf("expected guidance for a legacy Pulumi snapshot without bindings, got: %v", err)
	}
}
