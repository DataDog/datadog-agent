// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package com_datadoghq_script

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/require"

	fleetoci "github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	authoredscriptsoci "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts/oci"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func registryScriptImage(t *testing.T, version, command string) v1.Image {
	t.Helper()
	var data bytes.Buffer
	w := tar.NewWriter(&data)
	metadata := fmt.Sprintf(`{"schema-version":"v1","version":%q,"fqn":"com.datadoghq.authoredscripts.echo","command":{"entrypoint":"run.sh"},"parameterSchema":{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]},"parameterEnvMapping":{"message":"SCRIPT_MESSAGE"}}`, version)
	for _, file := range []struct{ name, contents string }{
		{"metadata.json", metadata},
		{"run.sh", "#!/bin/sh\n" + command + "\n"},
	} {
		require.NoError(t, w.WriteHeader(&tar.Header{Name: file.name, Mode: 0o755, Size: int64(len(file.contents))}))
		_, err := io.WriteString(w, file.contents)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	layer, err := tarball.LayerFromReader(bytes.NewReader(data.Bytes()))
	require.NoError(t, err)
	image, err := mutate.Append(empty.Image, mutate.Addendum{Layer: layer, MediaType: fleetoci.DatadogPackageLayerMediaType})
	require.NoError(t, err)
	return mutate.Annotations(image, map[string]string{fleetoci.AnnotationPackage: "test-script", fleetoci.AnnotationVersion: version}).(v1.Image)
}

func registryScriptTask(uri string) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		BundleID: "com.datadoghq.script", Name: "runRegistryScript",
		Inputs: map[string]interface{}{"uri": uri, "parameters": map[string]interface{}{"message": "hello"}},
	}
	return task
}

func registryCredentials(entries ...string) *privateconnection.PrivateCredentials {
	configuration := "schemaId: script-credentials-v1\nallowedRegistries:\n"
	for _, entry := range entries {
		configuration += "  - " + entry + "\n"
	}
	return &privateconnection.PrivateCredentials{Tokens: []privateconnection.PrivateCredentialsToken{{Name: "configFileLocation", Value: configuration}}}
}

func TestRunRegistryScript(t *testing.T) {
	var requests, blobReads atomic.Int32
	registryHandler := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") {
			blobReads.Add(1)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("unexpected registry credentials")
		}
		registryHandler.ServeHTTP(w, r)
	}))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	ref, err := name.NewTag(host+"/team/script:latest", name.StrictValidation)
	require.NoError(t, err)
	first := registryScriptImage(t, "1", `printf '%s' "$SCRIPT_MESSAGE"`)
	index := mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{Add: first, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "unsupported", Architecture: runtime.GOARCH}}},
		mutate.IndexAddendum{Add: first, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: runtime.GOOS, Architecture: runtime.GOARCH}}},
	)
	require.NoError(t, remote.WriteIndex(ref, index, remote.WithAuth(authn.Anonymous)))
	digest, err := first.Digest()
	require.NoError(t, err)
	resolver, err := authoredscriptsoci.NewRegistryResolver(t.TempDir(), server.Client())
	require.NoError(t, err)
	handler := &RunRegistryScriptHandler{resolver: resolver}
	credentials := registryCredentials(host + "/team")
	task := registryScriptTask("oci://" + ref.Name())

	// Installer overrides and local Docker credentials must not affect this action.
	t.Setenv("DD_INSTALLER_REGISTRY_URL", "unreachable.invalid")
	dockerConfig := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerConfig)
	require.NoError(t, os.WriteFile(filepath.Join(dockerConfig, "config.json"), []byte("invalid config: must not be read"), 0o600))

	output, err := handler.Run(context.Background(), task, credentials)
	require.NoError(t, err)
	require.Equal(t, "hello", output.(*RunRegistryScriptOutputs).Stdout)
	reads := blobReads.Load()
	require.Positive(t, reads)

	output, err = handler.Run(context.Background(), task, credentials)
	require.NoError(t, err)
	require.Equal(t, "hello", output.(*RunRegistryScriptOutputs).Stdout)
	require.Equal(t, reads, blobReads.Load(), "cache hit must not fetch layers")

	beforeDenied := requests.Load()
	_, err = handler.Run(context.Background(), task, registryCredentials())
	require.ErrorContains(t, err, "not permitted")
	_, err = handler.Run(context.Background(), task, registryCredentials(host+"/other"))
	require.ErrorContains(t, err, "not permitted")
	require.Equal(t, beforeDenied, requests.Load(), "deny before network or cached execution")

	second := registryScriptImage(t, "2", `printf 'updated:%s' "$SCRIPT_MESSAGE"`)
	require.NoError(t, remote.Write(ref, second, remote.WithAuth(authn.Anonymous)))
	output, err = handler.Run(context.Background(), task, credentials)
	require.NoError(t, err)
	require.Equal(t, "updated:hello", output.(*RunRegistryScriptOutputs).Stdout)

	output, err = handler.Run(context.Background(), registryScriptTask("oci://"+ref.Context().Digest(digest.String()).Name()), credentials)
	require.NoError(t, err)
	require.Equal(t, "hello", output.(*RunRegistryScriptOutputs).Stdout)

	task.Data.Attributes.Inputs = map[string]interface{}{"uri": "oci://" + ref.Name(), "parameters": map[string]interface{}{"message": 123}}
	_, err = handler.Run(context.Background(), task, credentials)
	require.Error(t, err, "manifest parameter schema must be enforced")

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = handler.Run(canceled, registryScriptTask("oci://"+ref.Name()), credentials)
	require.ErrorIs(t, err, context.Canceled)
}

func TestRunRegistryScriptRejectsInvalidInputs(t *testing.T) {
	handler := &RunRegistryScriptHandler{}
	_, err := handler.Run(context.Background(), nil, nil)
	require.ErrorContains(t, err, "task is required")
	_, err = handler.Run(context.Background(), registryScriptTask("oci://registry.test/script:v1"), nil)
	require.ErrorContains(t, err, "credentials are required")
	task := registryScriptTask("oci://registry.test/script:v1")
	task.Data.Attributes.Inputs = map[string]interface{}{"uri": "oci://registry.test/script:v1", "parameters": []string{"wrong"}}
	_, err = handler.Run(context.Background(), task, registryCredentials("registry.test"))
	require.ErrorContains(t, err, "unmarshaling inputs")
}
