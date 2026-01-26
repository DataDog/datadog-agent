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

type DigestFetcher interface {
	Digest(ref string) (string, error)
}

type httpDigestFetcher struct {
	client *http.Client
}

func (h *httpDigestFetcher) Digest(ref string) (string, error) {
	parts := strings.Split(ref, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid image reference: %s", ref)
	}

	registry := parts[0]
	repoAndTag := strings.Join(parts[1:], "/")

	repoParts := strings.Split(repoAndTag, ":")
	if len(repoParts) != 2 {
		return "", fmt.Errorf("invalid image reference, missing tag: %s", ref)
	}
	repository := repoParts[0]
	tag := repoParts[1]

	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, tag)

	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")

	resp, err := h.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code %d for %s", resp.StatusCode, ref)
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("no digest header found for %s", ref)
	}

	return digest, nil
}

func newHTTPDigestFetcher() *httpDigestFetcher {
	return &httpDigestFetcher{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}
