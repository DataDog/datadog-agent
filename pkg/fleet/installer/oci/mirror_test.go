// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oci

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
	"github.com/stretchr/testify/require"
)

type testMirrorServer struct {
	s *httptest.Server
	u string

	contentTypeRemap map[string]string
}

func newTestMirrorServer(upstream string) *testMirrorServer {
	s := &testMirrorServer{
		u:                upstream,
		contentTypeRemap: map[string]string{},
	}
	s.s = httptest.NewServer(s)
	return s
}

func (t *testMirrorServer) Close() {
	t.s.Close()
}

func (t *testMirrorServer) URL() string {
	return t.s.URL
}

func (t *testMirrorServer) SetContentTypeRemap(from string, to string) {
	t.contentTypeRemap[from] = to
}

func (t *testMirrorServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/mirror") || r.Method != http.MethodGet {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	url := t.u + strings.TrimPrefix(r.URL.Path, "/mirror")
	resp, err := http.Get(url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	contentType := resp.Header.Get("Content-Type")
	if remap, ok := t.contentTypeRemap[contentType]; ok {
		contentType = remap
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (t *testMirrorServer) WrapTransport(transport http.RoundTripper) http.RoundTripper {
	tr, err := newMirrorTransport(transport, t.s.URL+"/mirror")
	if err != nil {
		panic(err)
	}
	return tr
}

func TestMirrorTransport(t *testing.T) {
	upstream := fixtures.NewServer(t)
	defer upstream.Close()
	client := &http.Client{
		Transport: http.DefaultTransport,
	}

	mirror := newTestMirrorServer(upstream.URL())
	defer mirror.Close()
	client.Transport = mirror.WrapTransport(client.Transport)

	resp, err := client.Get(upstream.URL() + "/v2/")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMirrorTransportContentTypeRemap(t *testing.T) {
	upstream := fixtures.NewServer(t)
	defer upstream.Close()
	client := &http.Client{
		Transport: http.DefaultTransport,
	}

	mirror := newTestMirrorServer(upstream.URL())
	defer mirror.Close()
	client.Transport = mirror.WrapTransport(client.Transport)
	mirror.SetContentTypeRemap("application/vnd.oci.image.index.v1+json", "application/broken")

	resp, err := client.Get(upstream.URL() + "/v2/" + fixtures.FixtureSimpleV1.Package + "/manifests/latest")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/vnd.oci.image.index.v1+json", resp.Header.Get("Content-Type"))
}

func TestMirrorManifestPathWithError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer upstream.Close()
	client := &http.Client{
		Transport: http.DefaultTransport,
	}

	mirror := newTestMirrorServer(upstream.URL)
	defer mirror.Close()
	client.Transport = mirror.WrapTransport(client.Transport)

	resp, err := client.Get(upstream.URL + "/v2/agent-package/manifests/invalid-404")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}
