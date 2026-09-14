// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	binaryconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/binary"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

func binaryConfig(t *testing.T, section string) *config.File {
	t.Helper()
	data := "schema: 1\nenvironment:\n  base: local\nagent:\n  install: binary\n"
	if section != "" {
		data += "  binary:\n" + section
	}
	f, errs := config.Parse([]byte(data))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	return f
}

func TestBinaryValidateRejectsForeignFields(t *testing.T) {
	b := &Binary{}
	errs := b.Validate(binaryConfig(t, "    version: \"7.69.0\"\n"))
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "agent.binary.version") {
		t.Fatalf("version must not exist in a binary section, got: %v", errs)
	}
	if errs := b.Validate(binaryConfig(t, "")); len(errs) > 0 {
		t.Fatalf("an empty binary section is valid, got: %v", errs)
	}
}

func TestBinaryArtifactIsEmpty(t *testing.T) {
	version, image, err := (&Binary{}).Artifact(binaryConfig(t, ""))
	if err != nil || version != "" || image != "" {
		t.Fatalf("binary installs have no version or image, got %q/%q %v", version, image, err)
	}
}

func TestBinaryAgentExampleIsValid(t *testing.T) {
	node, err := (&Binary{}).AgentExample()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeBinarySection(&config.File{
		Agent: config.Agent{Install: "binary", SectionNode: node},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAgentRunArgs(t *testing.T) {
	entry := envstore.Entry{Name: "dev", Dir: "/store/envs/dev"}
	args := agentRunArgs(entry, "registry.datadoghq.com/agent:7.83.0")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--name dev-agent",
		"--network dev-net",
		"--hostname dev-agent",
		"/store/envs/dev/agent-binary:/opt/datadog-agent/bin/agent/agent:ro",
		"/store/envs/dev/agent.yaml:/etc/datadog-agent/datadog.yaml:ro",
		"/store/envs/dev/conf.d:/etc/datadog-agent/conf.d",
		"-e LD_LIBRARY_PATH=/opt/datadog-agent/dev-lib:/opt/datadog-agent/embedded/lib",
		"--entrypoint /opt/datadog-agent/bin/agent/agent",
		"registry.datadoghq.com/agent:7.83.0 run -c /etc/datadog-agent/datadog.yaml",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("docker run args missing %q: %s", want, joined)
		}
	}
}

func TestWriteAgentFilesGeneratesWiredConfig(t *testing.T) {
	entry := envstore.Entry{Name: "dev", Dir: t.TempDir()}
	// A snapshot with a fakeintake, like the local driver's Start writes.
	fiKey := []byte(`{"host":"127.0.0.1","scheme":"http","port":18080,"url":"http://127.0.0.1:18080"}`)
	if err := provisioner.WriteSnapshotFile(entry.SnapshotPath(), provisioner.RawResources{"fakeIntake": fiKey}, nil); err != nil {
		t.Fatal(err)
	}
	section, _, err := binaryconfig.Schema.Decode([]byte("runtime-image: registry.datadoghq.com/agent:7.83.0\nconfig: |\n  log_level: debug\nintegrations:\n  custom_logs.d: |\n    logs:\n      - type: file\n"), "agent.binary")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAgentFiles(entry, section, "test-api-key"); err != nil {
		t.Fatal(err)
	}
	agentYAML, err := os.ReadFile(filepath.Join(entry.Dir, "agent.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(agentYAML)
	for _, want := range []string{
		"hostname: dev-agent",
		"dd_url: http://dev-fakeintake:80", // container-network address, not 127.0.0.1
		"logs_config.logs_dd_url: dev-fakeintake:80",
		"log_level: debug",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("agent.yaml missing %q:\n%s", want, content)
		}
	}
	if !strings.Contains(content, "api_key: test-api-key") {
		t.Fatalf("api key not wired:\n%s", content)
	}
	conf, err := os.ReadFile(filepath.Join(entry.Dir, "conf.d", "custom_logs.d", "conf.yaml"))
	if err != nil || !strings.Contains(string(conf), "type: file") {
		t.Fatalf("integration folder not written: %v", err)
	}
	// Default core checks are seeded so system metrics work out of the box.
	for _, check := range []string{"cpu", "memory", "disk", "network", "uptime", "load", "io", "file_handle"} {
		if _, err := os.Stat(filepath.Join(entry.Dir, "conf.d", check+".d", "conf.yaml")); err != nil {
			t.Fatalf("default core check %s.d not written: %v", check, err)
		}
	}
	info, err := os.Stat(filepath.Join(entry.Dir, "agent.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("agent.yaml contains the api key and must be private, got %v", info.Mode())
	}
}
