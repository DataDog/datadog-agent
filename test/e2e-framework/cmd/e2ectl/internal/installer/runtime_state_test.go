// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/localinfra"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

func TestLegacyRuntimeStateBlocksBuildWithoutDiscardingState(t *testing.T) {
	entry, cfg := routingEntry(t)
	dir := filepath.Join(entry.Dir, "agent-run")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := rejectLegacyRuntimeState(entry); err != nil {
		t.Fatalf("empty scratch directory rejected: %v", err)
	}
	state := filepath.Join(dir, "remote-config.db")
	if err := os.WriteFile(state, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	b := &Binary{adapter: agentbuild.Adapter{Run: func(context.Context, agentbuild.Invocation) ([]byte, error) {
		t.Fatal("built/probed before legacy state gate")
		return nil, nil
	}}, docker: func(...string) (string, error) { t.Fatal("mutated legacy runtime"); return "", nil }}
	if err := b.Install(cfg, entry); err == nil || !strings.Contains(err.Error(), "migration/recreation") {
		t.Fatal(err)
	}
	data, err := os.ReadFile(state)
	if err != nil || string(data) != "keep" {
		t.Fatal("legacy state lost", err)
	}
}
func TestAgentOperationLockSerializesTeardown(t *testing.T) {
	entry, _ := routingEntry(t)
	unlock, err := LockAgentOperation(entry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LockAgentOperation(entry); err == nil {
		t.Fatal("concurrent teardown/mutation accepted")
	}
	unlock()
	again, err := LockAgentOperation(entry)
	if err != nil {
		t.Fatal(err)
	}
	again()
}

func namedRuntimeArtifact(t *testing.T) agentbuild.Result {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	file, err := agentbuild.DescribeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tree := func() agentbuild.Tree {
		dir := t.TempDir()
		path := filepath.Join(dir, "file")
		if err := os.WriteFile(path, []byte("fixture"), 0644); err != nil {
			t.Fatal(err)
		}
		f, err := agentbuild.DescribeFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f.Path = "file"
		return agentbuild.Tree{Root: dir, Files: []agentbuild.File{f}}
	}
	return agentbuild.Result{Schema: 1, Target: localTarget(), Provenance: agentbuild.Provenance{Producer: "invoke-binary", SourceSHA256: strings.Repeat("a", 64), Options: map[string]string{"runtimeLayout": "bazel-embedded-absolute-prefix"}}, Profile: &receivers.ProducerProfile{RouteContract: receivers.RouteContract, Roles: []receivers.ProducerRole{receivers.CoreAgent}}, Binary: &agentbuild.BinaryBundle{Executable: file, Runtime: tree(), Assets: tree(), RuntimePrefix: "/source/repo/dev/embedded", RuntimeImageID: "sha256:" + strings.Repeat("a", 64), PythonABI: "python3.13", PythonPath: "/opt/datadog-agent/embedded/lib/python3.13/site-packages", OSVersion: "24.04"}}
}
func TestNamedRuntimeReceiptAndNoBuildRewirePreserveExactVolume(t *testing.T) {
	entry, cfg := routingEntry(t)
	result := namedRuntimeArtifact(t)
	volume := localinfra.AgentRuntimeVolume(entry.Dir, entry.Meta.CreatedAt)
	exists := false
	labels := map[string]string{}
	commands := []string{}
	b := &Binary{ready: func(envstore.Entry) error { return nil }, docker: func(args ...string) (string, error) {
		commands = append(commands, strings.Join(args, " "))
		if args[0] == "volume" {
			switch args[1] {
			case "ls":
				if exists {
					return volume.Name, nil
				}
				return "", nil
			case "create":
				if exists {
					t.Fatal("recreated state volume")
				}
				exists = true
				pair := strings.SplitN(args[3], "=", 2)
				labels[pair[0]] = pair[1]
				return volume.Name, nil
			case "inspect":
				data, _ := json.Marshal(labels)
				return string(data), nil
			default:
				t.Fatal(args)
			}
		}
		switch args[0] {
		case "inspect":
			return result.Binary.RuntimeImageID, nil
		case "rm":
			return "", nil
		case "run":
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "type=volume,source="+volume.Name) || !strings.Contains(joined, "volume-nocopy") || strings.Contains(joined, filepath.Join(entry.Dir, "agent-run")) {
				t.Fatal("wrong runtime mount", joined)
			}
			if !strings.Contains(joined, "chmod 0700 /opt/datadog-agent/run") {
				t.Fatal("runtime not private")
			}
			return "", nil
		}
		t.Fatal(args)
		return "", nil
	}}
	if err := b.prepareRuntimeState(entry); err != nil {
		t.Fatal(err)
	}
	if err := publishArtifact(entry, result, provisioner.RawResources{}); err != nil {
		t.Fatal(err)
	}
	v, recorded, err := runtimeVolumeRecord(entry)
	if err != nil || !recorded || v != volume {
		t.Fatal(v, recorded, err)
	}
	commands = nil
	if err := b.ApplyRouting(cfg, entry); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(commands, "\n"), "volume create") {
		t.Fatal("rewire recreated runtime volume")
	}
	b.ready = func(envstore.Entry) error { return errors.New("health failed") }
	if err := WithRoutingState(b, cfg, entry, func() error { return b.ApplyRouting(cfg, entry) }); err == nil {
		t.Fatal("failed readiness accepted")
	}
	observed, err := RoutingStatus(entry)
	if err != nil || observed.Phase != "failed" {
		t.Fatal(observed, err)
	}
	exists = false
	commands = nil
	b.adapter.Run = func(context.Context, agentbuild.Invocation) ([]byte, error) {
		t.Fatal("built after runtime state loss")
		return nil, nil
	}
	if err := b.Update(cfg, entry, false); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatal(err)
	}
	if err := b.verifyRuntimeState(entry); err == nil {
		t.Fatal("missing volume silently reused")
	}
	if strings.Contains(strings.Join(commands, "\n"), "create") {
		t.Fatal("missing state recreated")
	}
}
