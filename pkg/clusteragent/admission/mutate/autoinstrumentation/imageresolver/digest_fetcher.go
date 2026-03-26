// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	requestTimeout = 15 * time.Second
)

// httpDigestFetcher fetches image digests from container registries.
// This implementation is designed for Datadog public registries only
// (Docker Hub, ECR, GCR, ACR) and does not support authentication.
type httpDigestFetcher struct {
	client *http.Client
}

func (h *httpDigestFetcher) buildManifestRequest(ref string) (*http.Request, error) {
	parts := strings.Split(ref, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid image reference: %s", ref)
	}

	registry := parts[0]
	repoAndTag := strings.Join(parts[1:], "/")

	// DEV: Split from the right to handle registries with ports (e.g., registry.io:5000/repo:tag)
	lastColon := strings.LastIndex(repoAndTag, ":")
	if lastColon == -1 {
		return nil, fmt.Errorf("invalid image reference, missing tag: %s", ref)
	}
	repository := repoAndTag[:lastColon]
	tag := repoAndTag[lastColon+1:]

	// DEV: Docker Hub uses registry-1.docker.io for API calls
	if registry == "docker.io" {
		registry = "registry-1.docker.io"
	}

	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, tag)

	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// DEV: Accept both Docker and OCI manifest formats for multi-arch images.
	// All Datadog lib injection images are published as multi-arch, so we don't
	// need single-arch fallback.
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.index.v1+json",
	}, ", "))
	req.Header.Set("User-Agent", "datadog-cluster-agent")

	return req, nil
}

func (h *httpDigestFetcher) digest(ref string) (string, error) {
	req, err := h.buildManifestRequest(ref)
	if err != nil {
		return "", err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "", fmt.Errorf("registry requires authentication (not supported for public images): %s (status %d)", ref, resp.StatusCode)
		case http.StatusNotFound:
			return "", fmt.Errorf("image not found: %s", ref)
		case http.StatusTooManyRequests:
			return "", fmt.Errorf("rate limited by registry: %s", ref)
		default:
			return "", fmt.Errorf("unexpected status %d for %s", resp.StatusCode, ref)
		}
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("no digest header found for %s", ref)
	}
	if !isValidDigest(digest) {
		return "", fmt.Errorf("invalid digest format: %s", digest)
	}
	return digest, nil
}

func newHTTPDigestFetcher(rt http.RoundTripper) *httpDigestFetcher {
	if rt == nil {
		rt = http.DefaultTransport.(*http.Transport).Clone()
	}
	return &httpDigestFetcher{
		client: &http.Client{
			Timeout:   requestTimeout,
			Transport: rt,
		},
	}
}
