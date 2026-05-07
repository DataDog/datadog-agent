// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.
package main

import (
	"archive/tar"
	"bytes"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// startRegistry spins up an in-process OCI registry and returns its host address.
func startRegistry(t *testing.T) string {
	t.Helper()
	s := httptest.NewServer(registry.New())
	t.Cleanup(s.Close)
	return s.Listener.Addr().String()
}

// makeAgentIndex creates a minimal source agent OCI index containing a ddot
// extension layer with a fake otel-agent binary, and pushes it to host.
func makeAgentIndex(t *testing.T, host string, tag string, platform *v1.Platform, binaryContent []byte) string {
	t.Helper()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	hdr := &tar.Header{Name: "embedded/bin/otel-agent", Mode: 0755, Size: int64(len(binaryContent))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(binaryContent); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	layer := static.NewLayer(tarBuf.Bytes(), ddotExtensionMediaType)
	base := mutate.MediaType(empty.Image, types.OCIManifestSchema1)
	base = mutate.ConfigMediaType(base, agentPackageMediaType)
	img, err := mutate.Append(base, mutate.Addendum{
		Layer:       layer,
		Annotations: map[string]string{extensionAnnotationKey: ddotExtensionName},
	})
	if err != nil {
		t.Fatal(err)
	}

	idx := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{
		Add:        img,
		Descriptor: v1.Descriptor{Platform: platform},
	})
	ref, err := name.ParseReference(fmt.Sprintf("%s/%s", host, tag))
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.WriteIndex(ref, idx, remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
		t.Fatal(err)
	}
	return ref.String()
}

// makeELFBinary returns a minimal ELF64 LE header that passes validateBinary for the given arch.
// arch must be "amd64" or "arm64".
func makeELFBinary(t *testing.T, arch string) []byte {
	t.Helper()
	b := make([]byte, 64)
	copy(b[0:4], []byte{0x7f, 'E', 'L', 'F'})
	b[4] = 2  // ELFCLASS64
	b[5] = 1  // ELFDATA2LSB
	b[6] = 1  // EI_VERSION
	b[16] = 2 // ET_EXEC
	b[20] = 1 // e_version
	switch arch {
	case "amd64":
		b[18] = 0x3e // EM_X86_64
	case "arm64":
		b[18] = 0xb7 // EM_AARCH64
	default:
		t.Fatalf("unsupported arch %q", arch)
	}
	return b
}

// writeTempBinary writes data to a temporary file and returns its path.
func writeTempBinary(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "otel-agent")
	if err := os.WriteFile(p, data, 0755); err != nil {
		t.Fatal(err)
	}
	return p
}

// platformsInIndex returns the "os/arch" pairs present in the output index.
func platformsInIndex(t *testing.T, ref string) map[string]bool {
	t.Helper()
	r, err := name.ParseReference(ref)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := remote.Index(r, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		t.Fatalf("pulling output index: %v", err)
	}
	m, err := idx.IndexManifest()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, d := range m.Manifests {
		if d.Platform != nil {
			out[d.Platform.OS+"/"+d.Platform.Architecture] = true
		}
	}
	return out
}

// TestFirstPushCreatesIndex verifies that when the output tag does not yet exist
// the tool pushes a fresh single-platform index.
func TestFirstPushCreatesIndex(t *testing.T) {
	host := startRegistry(t)
	platform := &v1.Platform{OS: "linux", Architecture: "amd64"}
	srcRef := makeAgentIndex(t, host, "agent/src:latest", platform, makeELFBinary(t, "amd64"))
	outputRef := fmt.Sprintf("%s/output/ddot:latest", host)

	if err := run(srcRef, writeTempBinary(t, makeELFBinary(t, "amd64")), outputRef, "linux", "amd64", authn.DefaultKeychain); err != nil {
		t.Fatalf("run: %v", err)
	}

	got := platformsInIndex(t, outputRef)
	if !got["linux/amd64"] {
		t.Fatalf("expected linux/amd64 in output index, got %v", got)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 platform, got %d: %v", len(got), got)
	}
}

// TestSecondPushAccumulatesPlatforms verifies that pushing a second platform
// appends to the existing index instead of overwriting it.
func TestSecondPushAccumulatesPlatforms(t *testing.T) {
	host := startRegistry(t)
	outputRef := fmt.Sprintf("%s/output/ddot:latest", host)

	platforms := []struct{ os, arch string }{
		{"linux", "arm64"},
		{"linux", "amd64"},
	}
	for _, p := range platforms {
		platform := &v1.Platform{OS: p.os, Architecture: p.arch}
		srcRef := makeAgentIndex(t, host, fmt.Sprintf("agent/src-%s-%s:latest", p.os, p.arch), platform, makeELFBinary(t, p.arch))
		if err := run(srcRef, writeTempBinary(t, makeELFBinary(t, p.arch)), outputRef, p.os, p.arch, authn.DefaultKeychain); err != nil {
			t.Fatalf("run(%s/%s): %v", p.os, p.arch, err)
		}
	}

	got := platformsInIndex(t, outputRef)
	for _, want := range []string{"linux/arm64", "linux/amd64"} {
		if !got[want] {
			t.Errorf("platform %s missing from output index; got %v", want, got)
		}
	}
}

// TestRepushSamePlatformDoesNotDuplicate verifies that re-running the tool for
// the same platform replaces the existing entry rather than adding a duplicate.
func TestRepushSamePlatformDoesNotDuplicate(t *testing.T) {
	host := startRegistry(t)
	outputRef := fmt.Sprintf("%s/output/ddot:latest", host)
	platform := &v1.Platform{OS: "linux", Architecture: "amd64"}

	for i := range 2 {
		srcRef := makeAgentIndex(t, host, fmt.Sprintf("agent/src-%d:latest", i), platform, makeELFBinary(t, "amd64"))
		if err := run(srcRef, writeTempBinary(t, makeELFBinary(t, "amd64")), outputRef, "linux", "amd64", authn.DefaultKeychain); err != nil {
			t.Fatalf("run iteration %d: %v", i, err)
		}
	}

	got := platformsInIndex(t, outputRef)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 platform after re-push, got %d: %v", len(got), got)
	}
}
