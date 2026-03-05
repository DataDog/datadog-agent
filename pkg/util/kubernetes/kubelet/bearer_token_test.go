// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBearerAuthRoundTripper_InjectsToken(t *testing.T) {
	var capturedReq *http.Request
	inner := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: 200}, nil
	})

	rt := &bearerAuthRoundTripper{bearer: "my-token", rt: inner}
	req, _ := http.NewRequest("GET", "https://example.com", nil)

	_, err := rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, "Bearer my-token", capturedReq.Header.Get("Authorization"))
}

func TestBearerAuthRoundTripper_DoesNotOverrideExistingAuth(t *testing.T) {
	var capturedReq *http.Request
	inner := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: 200}, nil
	})

	rt := &bearerAuthRoundTripper{bearer: "my-token", rt: inner}
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	req.Header.Set("Authorization", "Basic existing")

	_, err := rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, "Basic existing", capturedReq.Header.Get("Authorization"))
}

func TestBearerAuthRoundTripper_DoesNotModifyOriginalRequest(t *testing.T) {
	inner := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200}, nil
	})

	rt := &bearerAuthRoundTripper{bearer: "my-token", rt: inner}
	req, _ := http.NewRequest("GET", "https://example.com", nil)

	_, err := rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestBearerAuthRoundTripper_UsesRefreshedToken(t *testing.T) {
	var capturedReq *http.Request
	inner := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: 200}, nil
	})

	source := &fakeTokenSource{token: "refreshed-token"}
	rt := &bearerAuthRoundTripper{bearer: "initial-token", source: source, rt: inner}
	req, _ := http.NewRequest("GET", "https://example.com", nil)

	_, err := rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, "Bearer refreshed-token", capturedReq.Header.Get("Authorization"))
}

func TestBearerAuthRoundTripper_FallsBackOnSourceError(t *testing.T) {
	var capturedReq *http.Request
	inner := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return &http.Response{StatusCode: 200}, nil
	})

	source := &fakeTokenSource{err: assert.AnError}
	rt := &bearerAuthRoundTripper{bearer: "fallback-token", source: source, rt: inner}
	req, _ := http.NewRequest("GET", "https://example.com", nil)

	_, err := rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, "Bearer fallback-token", capturedReq.Header.Get("Authorization"))
}

func TestCachedFileTokenSource_ReadsAndCaches(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("token-v1"), 0644))

	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ts := newCachedFileTokenSource(tokenFile)
	ts.now = func() time.Time { return now }

	// First read
	tok, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "token-v1", tok)

	// Update file, but cached version should be returned
	require.NoError(t, os.WriteFile(tokenFile, []byte("token-v2"), 0644))
	tok, err = ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "token-v1", tok)

	// Advance time past expiry (period=1m, leeway=10s => expires at 50s)
	now = now.Add(51 * time.Second)
	tok, err = ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "token-v2", tok)
}

func TestCachedFileTokenSource_EmptyTokenFile(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("  \n  "), 0644))

	ts := newCachedFileTokenSource(tokenFile)
	_, err := ts.Token()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read empty token")
}

func TestCachedFileTokenSource_MissingFile(t *testing.T) {
	ts := newCachedFileTokenSource("/nonexistent/path/token")
	_, err := ts.Token()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read token file")
}

func TestCachedFileTokenSource_ReturnsStaleOnReadError(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("good-token"), 0644))

	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ts := newCachedFileTokenSource(tokenFile)
	ts.now = func() time.Time { return now }

	tok, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "good-token", tok)

	// Remove the file and advance past expiry
	require.NoError(t, os.Remove(tokenFile))
	now = now.Add(2 * time.Minute)

	// Should return stale token
	tok, err = ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "good-token", tok)
}

func TestCachedFileTokenSource_TrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("  my-token  \n"), 0644))

	ts := newCachedFileTokenSource(tokenFile)
	tok, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "my-token", tok)
}

func TestNewBearerAuthWithRefreshRoundTripper_NoFile(t *testing.T) {
	inner := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200}, nil
	})

	rt, err := newBearerAuthWithRefreshRoundTripper("static-token", "", inner)
	require.NoError(t, err)
	require.NotNil(t, rt)

	brt := rt.(*bearerAuthRoundTripper)
	assert.Equal(t, "static-token", brt.bearer)
	assert.Nil(t, brt.source)
}

func TestNewBearerAuthWithRefreshRoundTripper_WithFile(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("file-token"), 0644))

	inner := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200}, nil
	})

	rt, err := newBearerAuthWithRefreshRoundTripper("initial", tokenFile, inner)
	require.NoError(t, err)
	require.NotNil(t, rt)

	brt := rt.(*bearerAuthRoundTripper)
	assert.Equal(t, "initial", brt.bearer)
	assert.NotNil(t, brt.source)
}

func TestNewBearerAuthWithRefreshRoundTripper_EmptyBearerReadsFile(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("from-file"), 0644))

	inner := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200}, nil
	})

	rt, err := newBearerAuthWithRefreshRoundTripper("", tokenFile, inner)
	require.NoError(t, err)
	require.NotNil(t, rt)

	brt := rt.(*bearerAuthRoundTripper)
	assert.Equal(t, "from-file", brt.bearer)
}

func TestNewBearerAuthWithRefreshRoundTripper_MissingFileReturnsError(t *testing.T) {
	inner := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200}, nil
	})

	_, err := newBearerAuthWithRefreshRoundTripper("", "/nonexistent/token", inner)
	assert.Error(t, err)
}

// roundTripperFunc is a helper to create http.RoundTripper from a function.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// fakeTokenSource is a test helper implementing tokenSource.
type fakeTokenSource struct {
	token string
	err   error
}

func (f *fakeTokenSource) Token() (string, error) {
	return f.token, f.err
}
