// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packagecore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/host/localpackage"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"go.yaml.in/yaml/v3"
)

func fixture(t *testing.T) agentbuild.Result {
	t.Helper()
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("package-core installation requires native Linux amd64/arm64")
	}
	path := filepath.Join(t.TempDir(), "agent.deb")
	if err := os.WriteFile(path, []byte("package fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := agentbuild.DescribeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return agentbuild.Result{Schema: 1, Target: agentbuild.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}, Provenance: agentbuild.Provenance{Producer: "existing-package"}, Package: &agentbuild.Package{File: f, Name: "datadog-agent", Format: "deb", Version: "1:7.85.0-test", Roles: []string{"agent"}}}
}
func plan(t *testing.T) receivers.Plan {
	t.Helper()
	p, err := receivers.Capture("blackhole", "http://sink:8080", "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func effective(t *testing.T, p receivers.Plan) string {
	t.Helper()
	m := p.Settings()
	for k, v := range disabledSettings() {
		m[k] = v
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestPackageCoreNeverAttestsGeneralPackageRouting(t *testing.T) {
	r := fixture(t)
	if err := Validate(r, r.Package.File.SHA256); err != nil {
		t.Fatal(err)
	}
	p := plan(t)
	if err := localpackage.Validate(localpackage.Params{Artifact: r, AllowUnsigned: true, Routing: &p}, r.Target); err == nil {
		t.Fatal("unprofiled SSH package routing accepted")
	}
	r.Profile = &receivers.ProducerProfile{RouteContract: receivers.RouteContract, Roles: []receivers.ProducerRole{receivers.CoreAgent}}
	if err := Validate(r, r.Package.File.SHA256); err == nil {
		t.Fatal("forged package profile accepted")
	}
	r.Profile.RouteContract = "unknown"
	if err := Validate(r, r.Package.File.SHA256); err == nil {
		t.Fatal("unknown contract accepted")
	}
}
func TestPackageCorePreflightRejectsTargetsAndChangedBytes(t *testing.T) {
	r := fixture(t)
	if err := Validate(r, strings.Repeat("0", 64)); err == nil {
		t.Fatal("wrong selected digest accepted")
	}
	r.Target.Arch = "amd64"
	if runtime.GOARCH == "amd64" {
		r.Target.Arch = "arm64"
	}
	if err := Validate(r, r.Package.File.SHA256); err == nil {
		t.Fatal("non-native package target accepted")
	}
	r.Target.Arch = "unsupported"
	if err := Validate(r, r.Package.File.SHA256); err == nil {
		t.Fatal("wrong target accepted")
	}
	r = fixture(t)
	if err := os.WriteFile(r.Package.File.Path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Validate(r, r.Package.File.SHA256); err == nil {
		t.Fatal("changed artifact accepted")
	}
}
func TestPackageCoreConfigUsesStandardReceiverRules(t *testing.T) {
	// Receiver-owned destinations/credentials, unsupported backend sections and
	// attempts to re-enable the core-only pins are rejected; arbitrary other
	// configuration is accepted and merged into datadog.yaml.
	for _, raw := range []string{"apm_config.enabled: true", "logs_enabled: true", "process_config.process_collection.enabled: true", "dd_url: http://other", "skip_ssl_validation: true", "fips.enabled: true", "multi_region_failover.enabled: true", "logs_config.logs_dd_url: http://other", "not: [valid"} {
		if err := ValidateConfig(raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, raw := range []string{"", "tags: [purpose:health]", "hostname: package-test", "log_level: debug", "config_providers: []", "ca_certs: /etc/ssl/certs/ca-certificates.crt"} {
		if err := ValidateConfig(raw); err != nil {
			t.Fatalf("rejected %s: %v", raw, err)
		}
	}
	p := plan(t)
	config, err := Config(p, "must-not-be-used", "tags: [purpose:health]\nhostname: package-test\nlog_level: debug", "unused")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(config, "must-not-be-used") {
		t.Fatal("capture got real key")
	}
	if !strings.Contains(config, "log_level: debug") {
		t.Fatal("extra configuration not merged")
	}
	if _, err := EffectiveSettings(config, p); err != nil {
		t.Fatal(err)
	}
	if _, err := EffectiveSettings(strings.Replace(config, "logs_enabled: false", "logs_enabled: true", 1), p); err == nil {
		t.Fatal("enabled unqualified sender accepted")
	}
	if _, err := EffectiveSettings(strings.ReplaceAll(config, "http://sink:8080", "http://other:8080"), p); err == nil {
		t.Fatal("wrong effective endpoint accepted")
	}
}
func TestPackageCoreInstallationIsNotSourceBuild(t *testing.T) {
	r := fixture(t)
	id := "sha256:" + strings.Repeat("a", 64)
	checksum := strings.Repeat("b", 64)
	calls := []string{}
	i := Installer{Run: func(_ context.Context, in agentbuild.Invocation) ([]byte, error) {
		if in.Program != "docker" {
			t.Fatal("not container installation", in)
		}
		args := strings.Join(in.Args, " ")
		calls = append(calls, args)
		for _, bad := range []string{"--privileged", "docker.sock", "DOCKER_DD_AGENT", "systemctl"} {
			if strings.Contains(args, bad) {
				t.Fatal("unsafe container args", args)
			}
		}
		switch in.Args[0] {
		case "image":
			return json.Marshal([]map[string]string{{"Id": id, "Os": runtime.GOOS, "Architecture": runtime.GOARCH}})
		case "tag", "rm":
			return nil, nil
		case "build":
			dir := in.Args[len(in.Args)-1]
			df, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"FROM localhost/e2ectl-package-base:", "policy-rc.d", "exit 101", "dpkg -i /tmp/agent.deb", r.Package.File.SHA256, "dpkg-deb -x", "install ok installed"} {
				if !strings.Contains(string(df), want) {
					t.Fatal("missing install guard", want)
				}
			}
			if strings.Index(string(df), "policy-rc.d") > strings.Index(string(df), "dpkg -i") {
				t.Fatal("policy installed too late")
			}
			if !strings.Contains(string(df), "RUN --network=none echo") || !strings.Contains(string(df), "--no-install-recommends ca-certificates") {
				t.Fatal("system trust prerequisite or isolated package-script step missing")
			}
			return nil, os.WriteFile(in.Args[4], []byte(id), 0600)
		case "run":
			return []byte("ubuntu 24.04\ninstall ok installed\ndatadog-agent\n" + r.Package.Version + "\n" + r.Target.Arch + "\n" + checksum + "  " + AgentBinPath), nil
		default:
			t.Fatal(in)
			return nil, nil
		}
	}}
	got, err := i.InstallImage(context.Background(), r, r.Package.File.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if got.Scope != Scope || got.ImageID != id || got.ExecutableSHA256 != checksum || r.Profile != nil {
		t.Fatal(got)
	}
	if len(calls) != 8 {
		t.Fatal(calls)
	}
}
func TestPackageCoreProbeIsIsolatedAndCleansUpOnFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		p := plan(t)
		cleanup := false
		i := Installer{Run: func(_ context.Context, in agentbuild.Invocation) ([]byte, error) {
			if in.Args[0] == "rm" {
				cleanup = true
				return nil, nil
			}
			all := strings.Join(in.Args, " ")
			if !strings.Contains(all, "--network=none") || !strings.Contains(all, "--name e2ectl-package-probe-") || !strings.Contains(all, "check cpu") {
				t.Fatal(all)
			}
			for n, arg := range in.Args {
				if arg == "-v" && strings.HasSuffix(in.Args[n+1], ":/probe:ro") {
					dir := strings.TrimSuffix(in.Args[n+1], ":/probe:ro")
					b, err := os.ReadFile(filepath.Join(dir, "datadog.yaml"))
					if err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(string(b), receivers.DummyAPIKey) {
						t.Fatal("probe key not dummy")
					}
				}
			}
			if fail {
				return nil, errors.New("failure")
			}
			return []byte(effective(t, p)), nil
		}}
		err := i.Probe(context.Background(), Runtime{ImageID: "sha256:" + strings.Repeat("a", 64)}, p, "")
		if (err != nil) != fail || !cleanup {
			t.Fatal(err, cleanup)
		}
	}
}
func TestPackageCoreChecksRejectExtraOrChangedFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "conf.d")
	if err := PrepareChecks(dir); err != nil {
		t.Fatal(err)
	}
	if err := PrepareChecks(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extra.yaml"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := PrepareChecks(dir); err == nil {
		t.Fatal("extra check accepted")
	}
	if err := os.Remove(filepath.Join(dir, "extra.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cpu.d", "conf.yaml"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := PrepareChecks(dir); err == nil {
		t.Fatal("changed check accepted")
	}
}
func TestPackageCoreMissingSystemTrustFailsVerification(t *testing.T) {
	r := fixture(t)
	installed := Runtime{Scope: Scope, PackageSHA256: r.Package.File.SHA256, ImageID: "sha256:" + strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64)}
	i := Installer{Run: func(_ context.Context, in agentbuild.Invocation) ([]byte, error) {
		if in.Args[0] == "inspect" {
			return []byte(installed.ImageID), nil
		}
		if !strings.Contains(strings.Join(in.Args, " "), "test -s /etc/ssl/certs/ca-certificates.crt") {
			t.Fatal("system CA prerequisite not checked")
		}
		return nil, errors.New("system CA bundle missing")
	}}
	if err := i.Verify(context.Background(), r, installed, "agent"); err == nil {
		t.Fatal("missing runtime trust accepted")
	}
}

func TestPackageCoreVerifyRejectsRuntimeSubstitution(t *testing.T) {
	r := fixture(t)
	installed := Runtime{Scope: Scope, PackageSHA256: r.Package.File.SHA256, ImageID: "sha256:" + strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64)}
	i := Installer{Run: func(context.Context, agentbuild.Invocation) ([]byte, error) {
		return []byte("sha256:" + strings.Repeat("c", 64)), nil
	}}
	if err := i.Verify(context.Background(), r, installed, "agent"); err == nil {
		t.Fatal("substituted image accepted")
	}
	out := "ubuntu 24.04\ninstall ok installed\ndatadog-agent\n" + r.Package.Version + "\n" + r.Target.Arch + "\n" + strings.Repeat("c", 64) + "  " + AgentBinPath
	if _, err := checkIdentity(out, r, installed); err == nil {
		t.Fatal("substituted core bytes accepted")
	}
}
