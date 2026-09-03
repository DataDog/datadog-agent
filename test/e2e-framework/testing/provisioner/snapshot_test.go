// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

package provisioner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot.json")

	resources := RawResources{
		"kubernetesCluster": []byte(`{"clusterName":"qa-dev","kubeConfig":"apiVersion: v1"}`),
		"fakeIntake":        []byte(`{"host":"192.168.1.10","scheme":"http","port":30080,"url":"http://192.168.1.10:30080"}`),
	}
	meta := map[string]any{"source": "test", "created": "2026-09-03T10:00:00Z"}
	if err := WriteSnapshotFile(path, resources, meta); err != nil {
		t.Fatal(err)
	}

	gotResources, gotMeta, err := ReadSnapshotFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotResources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(gotResources))
	}
	// the writer re-indents embedded JSON payloads; only semantic equality
	// is contractual, so compare after compacting both sides
	if compactJSON(t, gotResources["kubernetesCluster"]) != compactJSON(t, resources["kubernetesCluster"]) {
		t.Errorf("kubernetesCluster payload mismatch: %s", gotResources["kubernetesCluster"])
	}
	if len(gotMeta) != 2 {
		t.Fatalf("expected 2 metadata keys, got %d: %v", len(gotMeta), gotMeta)
	}

	var cluster struct {
		ClusterName string `json:"clusterName"`
		KubeConfig  string `json:"kubeConfig"`
	}
	if err := ReadSnapshotResource(path, "kubernetesCluster", &cluster); err != nil {
		t.Fatal(err)
	}
	if cluster.ClusterName != "qa-dev" {
		t.Errorf("clusterName mismatch: %s", cluster.ClusterName)
	}
	if err := ReadSnapshotResource(path, "agent", &cluster); err == nil {
		t.Error("expected an error reading a missing resource")
	}
}

func TestWriteSnapshotFileRejectsReservedKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	err := WriteSnapshotFile(path, RawResources{"_meta": []byte(`{}`)}, nil)
	if err == nil || !strings.Contains(err.Error(), `must not start with "_"`) {
		t.Fatalf("expected reserved-key rejection, got: %v", err)
	}
}

func TestWriteSnapshotFileRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	err := WriteSnapshotFile(path, RawResources{"kubernetesCluster": []byte(`{not json`)}, nil)
	if err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("expected invalid-JSON rejection, got: %v", err)
	}
}

// TestSnapshotFormatMatchesStaticStack checks the on-disk shape against the
// format documented and read by provisioners.StaticStackProvisioner:
// a top-level JSON object, resource keys at the top level, "_"-prefixed
// metadata ignored by the reader.
func TestSnapshotFormatMatchesStaticStack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := WriteSnapshotFile(path, RawResources{"fakeIntake": []byte(`{"url":"http://x"}`)},
		map[string]any{"source": "test"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// the metadata key must have been prefixed with "_"
	if !strings.Contains(string(data), `"_source"`) {
		t.Errorf("metadata not written with _ prefix: %s", data)
	}
	for _, want := range []string{`"fakeIntake"`, `"_source"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("snapshot missing %s: %s", want, data)
		}
	}
}

// compactJSON removes all whitespace outside strings for comparison purposes.
func compactJSON(t *testing.T, data []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("re-marshaling: %v", err)
	}
	return string(out)
}
