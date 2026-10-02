// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && (darwin || windows)

package integration

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"os/user"
	"runtime"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/impl"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// Operator-managed artifacts remain outside the repository. Run on the device
// used for capture so native identities can be checked without persisting them.
func TestNativeCaptureBundle(t *testing.T) {
	directory := os.Getenv("EUDM_CAPTURE_BUNDLE")
	if directory == "" {
		t.Skip("set EUDM_CAPTURE_BUNDLE to a complete capture from this device")
	}
	loaded, err := bundle.Load(directory, version.FullCommit)
	if err != nil {
		t.Fatal(err)
	}
	platform := runtime.GOOS
	if platform == "darwin" {
		platform = "macos"
	}
	if loaded.Manifest.Profile.OS != platform {
		t.Fatal("native smoke must run on the captured platform")
	}
	var identities []string
	host, err := os.Hostname()
	if err != nil {
		t.Fatal("cannot obtain hostname for privacy check")
	}
	identities = append(identities, host)
	current, err := user.Current()
	if err != nil {
		t.Fatal("cannot obtain user for privacy check")
	}
	identities = append(identities, current.Username, current.HomeDir)
	system, err := checks.CollectSystemInfo()
	if err != nil {
		t.Fatal("cannot obtain native system identity for privacy check")
	}
	identities = append(identities, system.Uuid)
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatal("cannot obtain interfaces for privacy check")
	}
	for _, iface := range interfaces {
		identities = append(identities, iface.HardwareAddr.String())
		addresses, err := iface.Addrs()
		if err != nil {
			t.Fatal("cannot obtain interface addresses for privacy check")
		}
		for _, addr := range addresses {
			ip, _, err := net.ParseCIDR(addr.String())
			if err == nil && !ip.IsUnspecified() {
				identities = append(identities, ip.String())
			}
		}
	}
	check := func(data []byte) {
		t.Helper()
		for _, identity := range identities {
			escaped, _ := json.Marshal(identity)
			if identity != "" && (bytes.Contains(bytes.ToLower(data), bytes.ToLower([]byte(identity))) || bytes.Contains(bytes.ToLower(data), bytes.ToLower(escaped[1:len(escaped)-1]))) {
				// Never print the offending raw identity or body, including on failure.
				t.Fatal("native identity found in persisted capture content")
			}
		}
	}
	manifest, err := json.Marshal(loaded.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	check(manifest)
	for _, sample := range loaded.Manifest.Samples {
		check(loaded.Files[sample.File])
		for _, file := range sample.WireFiles {
			var ref bundle.WireReference
			if err := bundle.DecodeJSON(loaded.Files[file], &ref); err != nil {
				t.Fatal(err)
			}
			headers, err := json.Marshal(ref.Headers)
			if err != nil {
				t.Fatal(err)
			}
			check(headers)
			var body []byte
			if sample.Stream == schema.Processes || sample.Stream == schema.Connections {
				decoded, err := model.DecodeMessage(ref.Body)
				if err != nil {
					t.Fatal("cannot decode recorded Agent process message")
				}
				body, err = json.Marshal(decoded.Body)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				encoding := ref.Headers.Get("Content-Encoding")
				if encoding == "" || encoding == "identity" {
					body = ref.Body
				} else {
					kind, ok := map[string]string{"deflate": "zlib", "gzip": "gzip", "zstd": "zstd"}[encoding]
					if !ok {
						t.Fatal("unknown recorded compression format")
					}
					compressor := logscompression.NewComponent().NewCompressor(kind, 1)
					if compressor.ContentEncoding() != encoding {
						t.Fatal("required capture decompressor is unavailable")
					}
					body, err = compressor.Decompress(ref.Body)
					if err != nil {
						t.Fatal("cannot decompress recorded Agent body")
					}
				}
			}
			check(body)
		}
	}
}
