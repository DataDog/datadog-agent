// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	ostypes "github.com/DataDog/datadog-agent/test/e2e-framework/components/os/types"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
)

func routingEntry(t *testing.T) (envstore.Entry, *config.File) {
	t.Helper()
	entry := envstore.Entry{Name: "dev", Dir: t.TempDir(), Meta: envstore.Meta{Base: "local"}}
	raw := []byte("schema: 1\nenvironment: {base: local, fakeintake: false}\nagent:\n  source: true\n  receiver:\n    type: blackhole\n    blackhole: {url: 'http://sink:8080'}\n")
	if err := os.WriteFile(entry.ConfigPath(), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, errs := config.Parse(raw)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if err := provisioner.WriteSnapshotFile(entry.SnapshotPath(), provisioner.RawResources{"fixtureOwner": []byte(`{"keep":true}`)}, map[string]any{"_bindings": map[string]string{"custom": "fixtureOwner"}, "_artifact_other": "keep"}); err != nil {
		t.Fatal(err)
	}
	return entry, cfg
}

func TestBinaryNoFixturePreparesWithoutCredentials(t *testing.T) {
	entry, cfg := routingEntry(t)
	section, err := decodeBinarySection(cfg)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := (&Binary{}).prepareConfig(cfg, entry, section)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "fakeintake") || !strings.Contains(raw, receivers.DummyAPIKey) {
		t.Fatal("no-fixture sink not rendered")
	}
	// Destination/credential conflicts are rejected before Docker/container removal.
	cfg.Agent.Section = []byte("config: 'api_key: secret'")
	if _, err := (&Binary{}).prepareConfig(cfg, entry, section); err == nil {
		t.Fatal("raw conflict was accepted")
	}
}

func TestBinaryApplyRoutingRejectsLegacyPinsWithoutCapabilityEvidence(t *testing.T) {
	entry, cfg := routingEntry(t)
	if err := os.WriteFile(filepath.Join(entry.Dir, "agent-binary"), []byte("pinned-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"dev-lib", "agent-run"} {
		if err := os.Mkdir(filepath.Join(entry.Dir, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(entry.Dir, "dev-lib", "lib.so"), []byte("pinned-lib"), 0o600); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(entry.Dir, "agent-run", "remote-config.db")
	if err := os.WriteFile(stateFile, []byte("keep-runtime"), 0o600); err != nil {
		t.Fatal(err)
	}
	image := "sha256:" + strings.Repeat("1", 64)
	var commands []string
	b := &Binary{docker: func(args ...string) (string, error) {
		commands = append(commands, strings.Join(args, " "))
		switch args[0] {
		case "inspect":
			return image, nil
		case "rm", "run":
			return "", nil
		default:
			t.Fatalf("unexpected command %v", args)
			return "", nil
		}
	}, ready: func(envstore.Entry) error { return nil }}
	if err := b.recordPins(entry); err != nil {
		t.Fatal(err)
	}
	before, _, err := provisioner.ReadSnapshotFile(entry.SnapshotPath())
	if err != nil {
		t.Fatal(err)
	}
	commands = nil
	if err := b.ApplyRouting(cfg, entry); err == nil || !strings.Contains(err.Error(), "core-source routing capability evidence") {
		t.Fatal(err)
	}
	if len(commands) != 0 {
		t.Fatal("unprofiled legacy pins reached activation", commands)
	}
	after, meta, err := provisioner.ReadSnapshotFile(entry.SnapshotPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(before["fixtureOwner"]) != string(after["fixtureOwner"]) || string(meta["_artifact_other"]) != `"keep"` || meta["_bindings"] == nil || meta["_agent_binary"] == nil {
		t.Fatal("unrelated facts lost")
	}
	if data, err := os.ReadFile(stateFile); err != nil || string(data) != "keep-runtime" {
		t.Fatal("runtime state lost")
	}

}

func TestMissingExpectedFixtureAndRCTransitionsFailBeforeMutation(t *testing.T) {
	entry, cfg := routingEntry(t)
	stored := strings.ReplaceAll(string(cfg.Source()), "fakeintake: false", "fakeintake: true")
	if err := os.WriteFile(entry.ConfigPath(), []byte(stored), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, errs := config.Parse([]byte(stored))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if _, err := PrepareRouting(expected, entry, true); err == nil {
		t.Fatal("missing expected fixture fell back")
	}
	if err := os.WriteFile(entry.ConfigPath(), cfg.Source(), 0o600); err != nil {
		t.Fatal(err)
	}
	native, _ := receivers.Datadog("datadoghq.eu", "runner/api_key")
	if err := provisioner.UpdateSnapshotResources(entry.SnapshotPath(), nil, map[string]any{"_agent_routing": RoutingState{Phase: "applied", Plan: &native}}); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := WithRoutingState(&Binary{}, cfg, entry, func() error { called = true; return nil }); err == nil || called {
		t.Fatal("unsupported RC transition reached mutation")
	}
}

func TestSetAgentTagPreservesDecodedYAMLTags(t *testing.T) {
	values := map[string]interface{}{"datadog": map[string]interface{}{"tags": []interface{}{"original:tag"}}}
	setAgentTag(values, "stackid:dev")
	data, _ := json.Marshal(values)
	if !strings.Contains(string(data), "original:tag") || !strings.Contains(string(data), "stackid:dev") {
		t.Fatal(string(data))
	}
}

func TestNativeNoFixturePlanIsOffline(t *testing.T) {
	entry, cfg := routingEntry(t)
	cfg.Agent.Receiver = &config.ReceiverSelection{Type: "datadog", Section: []byte("site: datadoghq.eu\napi-key-ref: runner/api_key")}
	p, err := (&Binary{}).PrepareRouting(cfg, entry)
	if err != nil {
		t.Fatal(err)
	}
	if p.Endpoint != "" || p.Site != "datadoghq.eu" || p.RemoteConfig != "native" {
		t.Fatal(p)
	}
}

func TestRepairAttachmentDoesNotInitializeOldAgent(t *testing.T) {
	entry, _ := routingEntry(t)
	host, _ := json.Marshal(outputs.HostOutput{Transport: "docker", Address: "no-such-container", OSFamily: ostypes.LinuxFamily})
	if err := provisioner.UpdateSnapshotResources(entry.SnapshotPath(), provisioner.RawResources{
		"remoteHost": host,
		"agent":      []byte(`{"host":"invalid old Agent output must not be initialized"}`),
	}, nil); err != nil {
		t.Fatal(err)
	}
	// Docker is intentionally absent. Repair may initialize the host transport,
	// but never an old Agent client/readiness command.
	t.Setenv("PATH", t.TempDir())
	env, err := attachHostForInstall(entry)
	if err != nil {
		t.Fatal(err)
	}
	if env.Agent != nil || env.RemoteHost == nil {
		t.Fatal("repair attachment included the old Agent")
	}
	var old map[string]interface{}
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "agent", &old); err != nil {
		t.Fatal(err)
	}
	if old["host"] != "invalid old Agent output must not be initialized" {
		t.Fatal("old snapshot facts were mutated")
	}
}
