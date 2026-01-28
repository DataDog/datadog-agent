// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHttpDigestFetcher_buildManifestRequest_Success(t *testing.T) {
	f := newHTTPDigestFetcher()
	tests := []struct {
		name               string
		imageRef           string
		expectedRegistry   string
		expectedRepository string
		expectedTag        string
	}{
		{
			name:               "simple",
			imageRef:           "gcr.io/datadoghq/agent:7.50.0",
			expectedRegistry:   "gcr.io",
			expectedRepository: "datadoghq/agent",
			expectedTag:        "7.50.0",
		},
		{
			name:               "multi-level repository",
			imageRef:           "gcr.io/datadoghq/team/agent:latest",
			expectedRegistry:   "gcr.io",
			expectedRepository: "datadoghq/team/agent",
			expectedTag:        "latest",
		},
		{
			name:               "registry with port",
			imageRef:           "registry.io:5000/myrepo/image:1.0",
			expectedRegistry:   "registry.io:5000",
			expectedRepository: "myrepo/image",
			expectedTag:        "1.0",
		},
		{
			name:               "gradual rollout tag",
			imageRef:           "gcr.io/datadoghq/agent:v1-0",
			expectedRegistry:   "gcr.io",
			expectedRepository: "datadoghq/agent",
			expectedTag:        "v1-0",
		},
		{
			name:               "docker.io converts to registry-1.docker.io",
			imageRef:           "docker.io/datadog/agent:7.50.0",
			expectedRegistry:   "registry-1.docker.io",
			expectedRepository: "datadog/agent",
			expectedTag:        "7.50.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := f.buildManifestRequest(tt.imageRef)
			assert.NoError(t, err)
			assert.NotNil(t, req)

			expectedURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s",
				tt.expectedRegistry, tt.expectedRepository, tt.expectedTag)
			assert.Equal(t, expectedURL, req.URL.String())

			assert.Contains(t, req.Header.Get("Accept"), "application/vnd.docker.distribution.manifest.list.v2+json")
			assert.Contains(t, req.Header.Get("Accept"), "application/vnd.oci.image.index.v1+json")
			assert.Equal(t, "datadog-cluster-agent", req.Header.Get("User-Agent"))
			assert.Equal(t, "HEAD", req.Method)
		})
	}
}

func TestHttpDigestFetcher_buildManifestRequest_Error(t *testing.T) {
	f := newHTTPDigestFetcher()
	tests := []struct {
		name     string
		imageRef string
		errorMsg string
	}{
		{
			name:     "missing tag",
			imageRef: "gcr.io/datadoghq/agent",
			errorMsg: "missing tag",
		},
		{
			name:     "missing repository",
			imageRef: "gcr.io",
			errorMsg: "invalid image reference",
		},
		{
			name:     "empty string",
			imageRef: "",
			errorMsg: "invalid image reference",
		},
		{
			name:     "no separator",
			imageRef: "invalidreference",
			errorMsg: "invalid image reference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := f.buildManifestRequest(tt.imageRef)
			assert.Error(t, err)
			assert.Nil(t, req)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

func TestHttpDigestFetcher_digest_Success(t *testing.T) {
	f := newHTTPDigestFetcher()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Docker-Content-Digest", "sha256:abc123def456")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	testRef := server.URL[7:] + "/datadoghq/agent:v1"

	digest, err := f.digest(testRef)
	assert.NoError(t, err)
	assert.Equal(t, "sha256:abc123def456", digest)
}

func TestHttpDigestFetcher_digest_ErrorStatusCodes(t *testing.T) {
	f := newHTTPDigestFetcher()
	tests := []struct {
		name       string
		statusCode int
		errorMsg   string
	}{
		{
			name:       "404 Not Found",
			statusCode: http.StatusNotFound,
			errorMsg:   "image not found",
		},
		{
			name:       "401 Unauthorized",
			statusCode: http.StatusUnauthorized,
			errorMsg:   "requires authentication",
		},
		{
			name:       "403 Forbidden",
			statusCode: http.StatusForbidden,
			errorMsg:   "requires authentication",
		},
		{
			name:       "429 Rate Limited",
			statusCode: http.StatusTooManyRequests,
			errorMsg:   "rate limited",
		},
		{
			name:       "500 Internal Server Error",
			statusCode: http.StatusInternalServerError,
			errorMsg:   "unexpected status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			testRef := server.URL[7:] + "/datadoghq/agent:v1"

			digest, err := f.digest(testRef)
			assert.Error(t, err)
			assert.Empty(t, digest)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

func TestHttpDigestFetcher_digest_MissingDigestHeader(t *testing.T) {
	f := newHTTPDigestFetcher()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// DEV: Return 200 but no Docker-Content-Digest header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	testRef := server.URL[7:] + "/datadoghq/agent:v1"

	digest, err := f.digest(testRef)
	assert.Error(t, err)
	assert.Empty(t, digest)
	assert.Contains(t, err.Error(), "no digest header found")
}

func TestHttpDigestFetcher_digest_ValidDigestFormat(t *testing.T) {
	f := newHTTPDigestFetcher()
	digestValue := "sha256:abc123def456789012345678901234567890123456789012345678901234"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Docker-Content-Digest", digestValue)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	testRef := server.URL[7:] + "/datadoghq/agent:v1"
	digest, err := f.digest(testRef)

	assert.NoError(t, err)
	assert.Equal(t, digestValue, digest)
}

func TestHttpDigestFetcher_digest_InvalidDigestFormat(t *testing.T) {
	f := newHTTPDigestFetcher()
	tests := []struct {
		name        string
		digestValue string
	}{
		{
			name:        "no algorithm prefix",
			digestValue: "abc123def456",
		},
		{
			name:        "unsupported algorithm",
			digestValue: "md5:abc123",
		},
		{
			name:        "malformed digest",
			digestValue: "sha256:",
		},
		{
			name:        "invalid characters",
			digestValue: "sha256:zzz!!!",
		},
		{
			name:        "sha512 digest",
			digestValue: "sha512:abc123def456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Docker-Content-Digest", tt.digestValue)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			testRef := server.URL[7:] + "/datadoghq/agent:v1"
			digest, err := f.digest(testRef)

			assert.Error(t, err)
			assert.Empty(t, digest)
			assert.Contains(t, err.Error(), "invalid digest format")
		})
	}
}
