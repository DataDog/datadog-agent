// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	pc "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/localpackage"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os/types"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/container/packagecore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"go.yaml.in/yaml/v3"
)

func packageConfig(t *testing.T) *config.File {
	t.Helper()
	cfg, errs := config.Parse([]byte(`schema: 1
environment:
  base: local
  fakeintake: true
agent:
  pipeline: 138372337
  config: |
    tags: [purpose:health]
  receiver:
    type: fakeintake
    fakeintake:
      remote-config: disabled
`))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	return cfg
}

// hostPackageEntry attaches an SSH host output to a local-base entry: the
// snapshot shape an SSH environment has after start.
func hostPackageEntry(t *testing.T) (envstore.Entry, *config.File) {
	t.Helper()
	entry, _ := routingEntry(t)
	raw := "schema: 1\nenvironment: {base: local, fakeintake: false}\nagent:\n  pipeline: 138372337\n  receiver:\n    type: blackhole\n    blackhole: {url: 'http://sink:8080'}\n"
	if err := os.WriteFile(entry.ConfigPath(), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, errs := config.Parse([]byte(raw))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	ssh, err := json.Marshal(outputs.HostOutput{Transport: "ssh", Address: "10.0.0.1", OSFamily: types.LinuxFamily})
	if err != nil {
		t.Fatal(err)
	}
	if err := provisioner.UpdateSnapshotResources(entry.SnapshotPath(), provisioner.RawResources{"remoteHost": ssh}, nil); err != nil {
		t.Fatal(err)
	}
	return entry, cfg
}

func TestPackageExampleDecodes(t *testing.T) {
	node, err := (&Package{}).AgentExample()
	if err != nil {
		t.Fatal(err)
	}
	section, err := yaml.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeAgentSection(pc.Schema, &config.File{Agent: config.Agent{Install: "package", Section: section}}, "package"); err != nil {
		t.Fatal(err)
	}
}

func TestPackageContainerSelectionIsExplicitAndBounded(t *testing.T) {
	p := &Package{}
	cfg := packageConfig(t)
	if errs := p.validateContainer(cfg); len(errs) > 0 {
		t.Fatal(errs)
	}
	cfg.Agent.Build.Provider = "omnibus-repackage"
	if errs := p.validateContainer(cfg); len(errs) == 0 {
		t.Fatal("source build accepted")
	}
	cfg = packageConfig(t)
	cfg.Agent.Receiver = nil
	if errs := p.validateContainer(cfg); len(errs) == 0 {
		t.Fatal("legacy fallback accepted")
	}
	cfg = packageConfig(t)
	cfg.Agent.Section = []byte("allow-unsigned: true\nsha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nintegrations:\n  cpu.d: x\n")
	if errs := p.validateContainer(cfg); len(errs) == 0 {
		t.Fatal("container integrations accepted")
	}
}

func TestPackageSectionPermissionAndDerivedDigest(t *testing.T) {
	cfg := packageConfig(t)
	if errs := (&Package{}).Validate(cfg); len(errs) > 0 {
		t.Fatal(errs)
	}
	// The derived pipeline shape carries no user-typed digest: the download
	// provider pins the downloaded file in its receipt instead.
	cfg.Agent.Section = []byte("allow-unsigned: true\n")
	if errs := (&Package{}).Validate(cfg); len(errs) > 0 {
		t.Fatal("derived pipeline section must validate without a user-typed digest", errs)
	}
	cfg.Agent.Section = []byte("allow-unsigned: false\nsha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n")
	if errs := (&Package{}).Validate(cfg); len(errs) == 0 {
		t.Fatal("unsigned permission not required")
	}
	cfg.Agent.Section = []byte("allow-unsigned: true\nsha256: not-a-digest\n")
	if errs := (&Package{}).Validate(cfg); len(errs) == 0 {
		t.Fatal("malformed digest accepted")
	}
}

func TestPackageContainerRuntimeHasNoHostPackageMounts(t *testing.T) {
	entry := envstore.Entry{Name: "core", Dir: "/private/env/core"}
	all := strings.Join(packageContainerRunArgs(entry, packagecore.Runtime{ImageID: "sha256:" + strings.Repeat("a", 64)}), " ")
	for _, want := range []string{"type=volume,source=", "volume-nocopy", "package-core-conf.d:/etc/datadog-agent/conf.d:ro", "exec " + packagecore.AgentBinPath + " run", "--pull=never"} {
		if !strings.Contains(all, want) {
			t.Fatal(want, all)
		}
	}
	for _, bad := range []string{"--privileged", "docker.sock", "-v /opt", "systemctl", "agent-binary", "dev-lib", "agent-run:"} {
		if strings.Contains(all, bad) {
			t.Fatal(bad, all)
		}
	}
	updates, err := localHostOutputs(entry, "24.04")
	if err != nil {
		t.Fatal(err)
	}
	var out outputs.HostAgentOutput
	if err = json.Unmarshal(updates["agent"], &out); err != nil {
		t.Fatal(err)
	}
	if out.AgentBinPath != packagecore.AgentBinPath || out.Host.Transport != "docker" || out.Host.Address != "core-agent" {
		t.Fatal(out)
	}
}

func TestPackageRoutingStatusNamesLimitedScope(t *testing.T) {
	entry, cfg := routingEntry(t)
	cfg.Agent.Install = "package"
	if err := WithRoutingState(&Package{}, cfg, entry, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	state, err := RoutingStatus(entry)
	if err != nil {
		t.Fatal(err)
	}
	if state.Scope != packagecore.Scope || state.Delivery != "unverified" || !strings.Contains(strings.Join(state.Plan.Warnings, " "), "no package producer attestation") {
		t.Fatal(state)
	}
}

func TestPackageApplyCannotChangeSource(t *testing.T) {
	entry, cfg := routingEntry(t)
	cfg.Agent.Install = "package"
	if err := (&Package{}).ApplyRouting(cfg, entry); err == nil || !strings.Contains(err.Error(), "cannot change installer") {
		t.Fatal(err)
	}
}

func TestPackageTargetComesFromTheAttachedHost(t *testing.T) {
	entry, _ := routingEntry(t) // no remoteHost: the local base before install
	if !packageTarget(entry) {
		t.Fatal("missing host output must select the container target")
	}
	host, err := json.Marshal(outputs.HostOutput{Transport: "docker", Address: "agent", OSFamily: types.LinuxFamily})
	if err != nil {
		t.Fatal(err)
	}
	if err := provisioner.UpdateSnapshotResources(entry.SnapshotPath(), provisioner.RawResources{"remoteHost": host}, nil); err != nil {
		t.Fatal(err)
	}
	if !packageTarget(entry) {
		t.Fatal("docker transport must select the container target")
	}
	if err := provisioner.UpdateSnapshotResources(entry.SnapshotPath(), provisioner.RawResources{"remoteHost": []byte(`{"transport":"ssh","address":"10.0.0.1","os_family":"linux"}`)}, nil); err != nil {
		t.Fatal(err)
	}
	if packageTarget(entry) {
		t.Fatal("ssh transport must select the host target")
	}
}

func TestPackageHostTargetKeepsLegacyPlanAndFailsClosedOnApply(t *testing.T) {
	entry, cfg := hostPackageEntry(t)
	if packageTarget(entry) {
		t.Fatal("ssh host snapshot must select the host target")
	}
	if (&Package{}).RoutingScope(entry) != "" {
		t.Fatal("host target must not carry the container scope")
	}
	// The legacy plan fallback stays available on the host target.
	legacy := *cfg
	legacy.Agent.Receiver = nil
	plan, err := (&Package{}).PrepareRouting(&legacy, entry)
	if err != nil {
		t.Fatal(err)
	}
	if plan != nil {
		t.Fatal("legacy host fallback must produce no plan")
	}
	// No-build receiver apply is not implemented for host targets; no fallback.
	if err := (&Package{}).ApplyRouting(cfg, entry); err == nil || !strings.Contains(err.Error(), "not supported for agent.package on SSH host targets") {
		t.Fatal(err)
	}
}
