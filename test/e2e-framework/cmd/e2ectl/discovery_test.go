// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/driver"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
)

func offlineDiscovery(t *testing.T) string {
	t.Helper()
	store := filepath.Join(t.TempDir(), "must-not-be-created")
	t.Setenv("E2ECTL_HOME", store)
	t.Setenv("E2ECTL_WORKER", "/nonexistent-executor")
	t.Setenv("PATH", "") // no kind, Docker, Pulumi or credential helper can be executed
	t.Setenv("E2E_API_KEY", "")
	t.Setenv("E2E_APP_KEY", "")
	return store
}

func assertNoStore(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("discovery must not create an environment store: %v", err)
	}
}

func TestEnvironmentTypeDiscovery(t *testing.T) {
	store := offlineDiscovery(t)
	var stdout, stderr bytes.Buffer
	if err := listEnvironmentTypes([]string{"--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var descriptions []environmentDescription
	if err := json.Unmarshal(stdout.Bytes(), &descriptions); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, desc := range descriptions {
		ids = append(ids, desc.Base)
		d, err := driver.Get(desc.Base)
		if err != nil {
			t.Fatal(err)
		}
		if desc.Description != d.Description() || len(desc.Installers) != len(d.Installers()) {
			t.Fatalf("discovery must come from the registered driver: %+v", desc)
		}
		for _, info := range desc.Installers {
			inst, err := driver.InstallerFor(d, info.ID)
			if err != nil {
				t.Fatal(err)
			}
			_, wantUpdate := inst.(installer.Updatable)
			if info.Updatable != wantUpdate {
				t.Fatalf("incorrect update capability: %+v", info)
			}
		}
	}
	if !reflect.DeepEqual(ids, driver.IDs()) {
		t.Fatalf("discovery must list every driver in deterministic order: %v", ids)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected diagnostics: %s", stderr.String())
	}
	stdout.Reset()
	if err := listEnvironmentTypes(nil, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"BASE", "INSTALLERS", "DESCRIPTION", "kind", "ec2-host", "helm (update)", "script"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("human-readable discovery missing %q: %s", want, stdout.String())
		}
	}
	assertNoStore(t, store)
}

func TestInitToStdout(t *testing.T) {
	store := offlineDiscovery(t)
	for _, id := range driver.IDs() {
		t.Run(id, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := generateStarterConfig([]string{"--base", id}, &stdout, &stderr); err != nil {
				t.Fatal(err)
			}
			cfg, errs := config.Parse(stdout.Bytes())
			if len(errs) != 0 {
				t.Fatalf("stdout must contain only valid config YAML: %v", errs)
			}
			if cfg.Environment.Base != id || cfg.Agent.SectionNode == nil {
				t.Fatalf("generated config for %q must show the installer's agent section", id)
			}
			if strings.Contains(stdout.String(), "api-key") {
				t.Fatalf("generated config for %q must not contain credentials", id)
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected diagnostics: %s", stderr.String())
			}
		})
	}
	assertNoStore(t, store)
}

func TestInitToNewFile(t *testing.T) {
	store := offlineDiscovery(t)
	path := filepath.Join(t.TempDir(), "my-kind.yaml")
	var stdout, stderr bytes.Buffer
	args := []string{"--base", "kind", "--output", path}
	if err := generateStarterConfig(args, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := driver.StarterConfig("kind")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, want) || stdout.Len() != 0 || !strings.Contains(stderr.String(), path) {
		t.Fatal("file output should preserve the template and report its path only on stderr")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("generated config must be private: %v", info.Mode())
		}
	}
	if err := generateStarterConfig(args, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("expected overwrite refusal, got: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("existing config was changed")
	}
	assertNoStore(t, store)
}

func TestInitRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target, link := filepath.Join(dir, "existing.yaml"), filepath.Join(dir, "link.yaml")
	if err := os.WriteFile(target, []byte("keep this"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := generateStarterConfig([]string{"--base", "kind", "--output", link}, io.Discard, io.Discard); err == nil {
		t.Fatal("must not follow an existing symlink when generating a config")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "keep this" {
		t.Fatal("symlink target was changed")
	}
}

func TestInitErrorsDoNotCreateFiles(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"--base", "unknown"},
		{"--base", "kind", "unexpected-argument"},
		{"--base", "kind", "--invalid-flag"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "must-not-exist.yaml")
			var stdout bytes.Buffer
			// Put --output before positional arguments so flags are always parsed.
			withOutput := append([]string{"--output", path}, args...)
			if err := generateStarterConfig(withOutput, &stdout, io.Discard); err == nil {
				t.Fatal("expected an argument error")
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("invalid arguments must not create the output file")
			}
			if stdout.Len() != 0 {
				t.Fatal("invalid arguments must not emit a partial config")
			}
		})
	}
}

func TestDiscoveryHelp(t *testing.T) {
	for name, command := range map[string]func([]string, io.Writer, io.Writer) error{
		"environments": listEnvironmentTypes,
		"init":         generateStarterConfig,
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := command([]string{"--help"}, &stdout, &stderr); !errors.Is(err, flag.ErrHelp) {
				t.Fatalf("expected help result, got: %v", err)
			}
			if stdout.Len() != 0 || stderr.Len() == 0 {
				t.Fatal("help must not emit a config or listing")
			}
		})
	}
}

var errOutput = errors.New("output unavailable")

type failingOutput struct{}

func (failingOutput) Write([]byte) (int, error) { return 0, errOutput }

func TestDiscoveryOutputErrors(t *testing.T) {
	for name, run := range map[string]func() error{
		"table": func() error { return listEnvironmentTypes(nil, failingOutput{}, io.Discard) },
		"json":  func() error { return listEnvironmentTypes([]string{"--json"}, failingOutput{}, io.Discard) },
		"yaml":  func() error { return generateStarterConfig([]string{"--base", "kind"}, failingOutput{}, io.Discard) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(); !errors.Is(err, errOutput) {
				t.Fatalf("expected output error, got: %v", err)
			}
		})
	}
}
