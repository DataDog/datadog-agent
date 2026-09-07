// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioner

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
)

func TestWriteSnapshotFileForEnvRoundtrip(t *testing.T) {
	type base struct {
		RemoteHost *outputs.HostOutput
	}
	type extended struct {
		base
		Peer       *outputs.HostOutput `import:"explicit-peer"`
		FakeIntake *outputs.FakeintakeOutput
	}
	source := extended{base: base{RemoteHost: &outputs.HostOutput{}}, Peer: &outputs.HostOutput{}}
	source.RemoteHost.SetKey("dd-Host-aws-vm")
	source.Peer.SetKey("ignored-in-favor-of-import-tag")
	resources := RawResources{
		"dd-Host-aws-vm": []byte(`{"address":"first"}`),
		"explicit-peer":  []byte(`{"address":"second"}`),
	}
	metadata := map[string]any{"_source": "test", "_stack": "existing-stack"}
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := WriteSnapshotFileForEnv(path, &source, resources, metadata); err != nil {
		t.Fatal(err)
	}
	if _, ok := metadata[SnapshotBindingsKey]; ok {
		t.Fatal("exporter modified the caller's metadata")
	}
	_, meta, err := ReadSnapshotFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var bindings map[string]string
	if err := json.Unmarshal(meta[SnapshotBindingsKey], &bindings); err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 2 || bindings["remoteHost"] != "dd-Host-aws-vm" || bindings["explicit-peer"] != "explicit-peer" {
		t.Fatalf("incorrect bindings: %v", bindings)
	}

	var dest extended
	got, err := NewStaticStackProvisioner[extended]("", path).ProvisionEnv(context.Background(), "attach", io.Discard, &dest)
	if err != nil {
		t.Fatal(err)
	}
	if dest.RemoteHost == nil || dest.Peer == nil || dest.FakeIntake != nil {
		t.Fatal("binding capture and attachment disagree about embedded/tagged/optional fields")
	}
	if err := dest.RemoteHost.Import(got[dest.RemoteHost.Key()], dest.RemoteHost); err != nil {
		t.Fatal(err)
	}
	if dest.RemoteHost.Address != "first" {
		t.Fatalf("unexpected host address: %q", dest.RemoteHost.Address)
	}
	var host outputs.HostOutput
	if err := ReadSnapshotResource(path, "remoteHost", &host); err != nil {
		t.Fatal(err)
	}
	if host.Address != "first" {
		t.Fatal("direct resource reads must resolve the same binding as static attachment")
	}
}

func TestWriteSnapshotFileForEnvRejectsInvalidEnvironment(t *testing.T) {
	for name, env := range map[string]any{
		"nil":              (*snapshotHostEnv)(nil),
		"non pointer":      snapshotHostEnv{},
		"non struct":       new(string),
		"missing key":      &snapshotHostEnv{RemoteHost: &outputs.HostOutput{}},
		"missing resource": hostWithKey("missing"),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "snapshot.json")
			if err := WriteSnapshotFileForEnv(path, env, RawResources{}, nil); err == nil {
				t.Fatal("expected an error rather than an unusable snapshot")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("invalid snapshot should not be written: %v", err)
			}
		})
	}
}

func hostWithKey(key string) *snapshotHostEnv {
	env := &snapshotHostEnv{RemoteHost: &outputs.HostOutput{}}
	env.RemoteHost.SetKey(key)
	return env
}

func TestUpdateSnapshotResourceUpdatesBinding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := WriteSnapshotFileForEnv(path, hostWithKey("dd-Host-original"), RawResources{
		"dd-Host-original": []byte(`{"address":"old"}`),
	}, map[string]any{"_stack": "keep-me"}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateSnapshotResource(path, "remoteHost", []byte(`{"address":"new"}`)); err != nil {
		t.Fatal(err)
	}
	var env snapshotHostEnv
	resources, err := NewStaticStackProvisioner[snapshotHostEnv]("", path).ProvisionEnv(context.Background(), "attach", io.Discard, &env)
	if err != nil {
		t.Fatal(err)
	}
	if env.RemoteHost == nil || env.RemoteHost.Key() != "remoteHost" {
		t.Fatal("binding still points at the old provisioned resource")
	}
	if err := env.RemoteHost.Import(resources[env.RemoteHost.Key()], env.RemoteHost); err != nil {
		t.Fatal(err)
	}
	if env.RemoteHost.Address != "new" || resources["dd-Host-original"] == nil {
		t.Fatal("replacement must update the binding and preserve unrelated resources")
	}
	_, meta, err := ReadSnapshotFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(meta["_stack"]) != `"keep-me"` {
		t.Fatal("snapshot metadata was lost")
	}
}

func TestWriteSnapshotFilePrivateReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits are not supported on Windows")
	}
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteSnapshotFile(path, RawResources{"remoteHost": []byte(`{}`)}, nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot is not private: %v", info.Mode())
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateSnapshotResource(path, "remoteHost", []byte(`invalid`)); err == nil {
		t.Fatal("invalid replacement must be rejected")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("invalid replacement corrupted the existing snapshot")
	}
}
