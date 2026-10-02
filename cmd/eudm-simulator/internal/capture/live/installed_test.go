// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package live

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestInstalledConfigurationSelectsOnlyAPILocations(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "datadog.yaml")
	contents := "cmd_host: 127.0.0.1\ncmd_port: 8123\nipc_address: localhost\nauth_token_file_path: /fixture/auth\nipc_cert_file_path: /fixture/cert\nprocess_config: {cmd_port: 8124, process_collection: {enabled: false}}\napi_key: credential-sentinel\ndd_url: https://native-backend.invalid\nnetwork_config: {direct_send: true}\nsoftware_inventory: {enabled: true, interval: 99}\nsystem_probe_config: {sysprobe_socket: /fixture/old.socket}\n"
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "system-probe.yaml"), []byte("system_probe_config: {sysprobe_socket: /fixture/current.socket}\nnetwork_config: {direct_send: true}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DD_API_KEY", "ambient-credential-sentinel")
	t.Setenv("DD_CMD_PORT", "9999")
	for _, input := range []string{path, directory} {
		cfg, err := installedConfig(input, true)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ConfigFileUsed() != path || cfg.GetInt("cmd_port") != 8123 || cfg.GetInt("process_config.cmd_port") != 8124 || cfg.GetString("ipc_address") != "localhost" || cfg.GetString("system_probe_config.sysprobe_socket") != "/fixture/current.socket" || cfg.GetString("auth_token_file_path") != "/fixture/auth" || cfg.GetString("ipc_cert_file_path") != "/fixture/cert" {
			t.Fatal("installed API locations were not preserved")
		}
		for _, key := range []string{"api_key", "dd_url", "network_config.direct_send", "software_inventory.interval", "process_config.process_collection.enabled"} {
			if cfg.IsConfigured(key) {
				t.Fatalf("capture imported an unrelated installed setting: %s", key)
			}
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != contents {
		t.Fatal("reading API locations changed installed configuration")
	}
}

func TestInstalledConfigurationFailuresDoNotEchoInput(t *testing.T) {
	for _, body := range []string{"cmd_port: native-secret", "cmd_host: [native-secret", "cmd_port: 5001\n---\napi_key: native-secret"} {
		path := filepath.Join(t.TempDir(), "datadog.yaml")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := installedConfig(path, false); err == nil || strings.Contains(err.Error(), "native-secret") {
			t.Fatal("unsafe configuration error")
		}
	}
}

func TestCaptureReadOnlyIPCDoesNotCreateMissingArtifacts(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("installed capture platform restriction")
	}
	previous := version.FullCommit
	version.FullCommit = strings.Repeat("a", 40)
	t.Cleanup(func() { version.FullCommit = previous })
	directory := t.TempDir()
	path := filepath.Join(directory, "datadog.yaml")
	if err := os.WriteFile(path, []byte("cmd_host: localhost\ncmd_port: 5001\n"), 0600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "capture")
	var progress bytes.Buffer
	if err := RunInstalled(context.Background(), output, path, time.Minute, &progress); err == nil || !strings.Contains(err.Error(), "existing Agent IPC authentication artifacts") {
		t.Fatalf("missing artifacts did not fail closed: %v", err)
	}
	if !strings.Contains(progress.String(), "initializing local authentication") || !strings.Contains(progress.String(), "failed") || strings.Contains(progress.String(), "bundle="+output) {
		t.Fatal("startup failure did not produce safe progress and failure output")
	}
	files, err := os.ReadDir(directory)
	if err != nil || len(files) != 1 || files[0].Name() != "datadog.yaml" {
		t.Fatal("capture created authentication artifacts or output before readiness")
	}
}

func TestCaptureRejectsExistingOutputBeforeReadingConfiguration(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("installed capture platform restriction")
	}
	previous := version.FullCommit
	version.FullCommit = strings.Repeat("a", 40)
	t.Cleanup(func() { version.FullCommit = previous })
	for _, kind := range []string{"completed bundle", "file", "dangling symlink"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			output := filepath.Join(directory, "capture")
			preserved := output
			if kind == "dangling symlink" {
				if err := os.Symlink(filepath.Join(directory, "missing"), output); err != nil {
					t.Skipf("cannot create symlink: %v", err)
				}
			} else {
				if kind == "completed bundle" {
					if err := os.Mkdir(output, 0700); err != nil {
						t.Fatal(err)
					}
					preserved = filepath.Join(output, "COMPLETE")
				}
				if err := os.WriteFile(preserved, []byte("existing-content"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var progress bytes.Buffer
			err := RunInstalled(context.Background(), output, filepath.Join(directory, "missing.yaml"), time.Minute, &progress)
			if err == nil || !strings.Contains(err.Error(), "already exists; choose a new --output directory") {
				t.Fatalf("existing output was not rejected before configuration: %v", err)
			}
			if strings.Contains(progress.String(), "waiting for producers") {
				t.Fatal("existing output reached producer discovery")
			}
			if kind == "dangling symlink" {
				if target, err := os.Readlink(output); err != nil || target != filepath.Join(directory, "missing") {
					t.Fatal("capture changed the existing symlink")
				}
			} else if contents, err := os.ReadFile(preserved); err != nil || string(contents) != "existing-content" {
				t.Fatal("capture changed existing output")
			}
		})
	}
}
