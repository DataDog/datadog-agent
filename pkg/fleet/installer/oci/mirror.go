// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oci

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// mirrorTransport is an http.RoundTripper that forwards requests to a mirror URL.
type mirrorTransport struct {
	mirror    *url.URL
	transport http.RoundTripper
}

// newMirrorTransport creates a new mirrorTransport from a mirror URL.
func newMirrorTransport(transport http.RoundTripper, mirror string) (*mirrorTransport, error) {
	mirrorURL, err := url.Parse(mirror)
	if err != nil {
		return nil, err
	}

	return &mirrorTransport{
		mirror:    mirrorURL,
		transport: transport,
	}, nil
}

// RoundTrip modifies the request to point to the mirror URL before sending it.
func (mt *mirrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Avoid mirroring potential redirects requested by the mirror.
	if req.Response != nil {
		return mt.transport.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	clone.Host = mt.mirror.Host
	clone.URL.Scheme = mt.mirror.Scheme
	clone.URL.Host = mt.mirror.Host
	if mt.mirror.User != nil {
		password, _ := mt.mirror.User.Password()
		clone.SetBasicAuth(mt.mirror.User.Username(), password)
	}
	var err error
	if mt.mirror.Path != "" {
		clone.URL.Path = mt.mirror.JoinPath(clone.URL.Path).Path
	}

	// Some mirrors have special logic for this path. Since this path only purpose in the OCI spec
	// is to check if the registry is an OCI registry, we can safely return a 200 OK.
	if req.URL.Path == "/v2/" {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	}

	r, err := mt.transport.RoundTrip(clone)
	if err != nil {
		return nil, err
	}
	if r.StatusCode != http.StatusOK {
		return r, nil
	}
	// Unfortunately some mirrors (ex: Nexus) do not respect the Content-Type header of the original request.
	// We fix the Content-Type header for manifest requests to match the mediaType field in the manifest.
	if isManifestPath(req.URL.Path) {
		err := fixManifestContentTypes(r)
		if err != nil {
			return nil, fmt.Errorf("err fixing manifest content types: %w", err)
		}
	}
	return r, nil
}

// isManifestPath returns true if the path is of the form /v2/<repository>/manifests/<reference>.
func isManifestPath(path string) bool {
	path = strings.TrimPrefix(path, "/")
	segments := strings.Split(path, "/")
	return len(segments) >= 4 &&
		segments[0] == "v2" &&
		segments[len(segments)-2] == "manifests"
}

type mediaType struct {
	MediaType string `json:"mediaType"`
}

// fixManifestContentTypes modifies the Content-Type header of the response to match the mediaType field in the manifest.
func fixManifestContentTypes(r *http.Response) error {
	var mediaType mediaType
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(buf))
	err = json.Unmarshal(buf, &mediaType)
	if err != nil {
		return err
	}
	if mediaType.MediaType != "" {
		r.Header.Set("Content-Type", mediaType.MediaType)
	}
	return nil
}
