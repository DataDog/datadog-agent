// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// bearerAuthRoundTripper is an http.RoundTripper that injects a bearer token
// into the Authorization header of each request. If a tokenSource is provided,
// the token is refreshed from it on each request.
type bearerAuthRoundTripper struct {
	bearer string
	source tokenSource
	rt     http.RoundTripper
}

// tokenSource provides a token string.
type tokenSource interface {
	Token() (string, error)
}

// newBearerAuthWithRefreshRoundTripper creates an http.RoundTripper that adds
// bearer token authentication. If tokenFile is non-empty, the token is
// periodically re-read from the file (with 1-minute caching, matching upstream
// client-go behavior for Bound Service Account Token Volume support).
//
// This is a local replacement for k8s.io/client-go/transport.NewBearerAuthWithRefreshRoundTripper.
func newBearerAuthWithRefreshRoundTripper(bearer string, tokenFile string, rt http.RoundTripper) (http.RoundTripper, error) {
	if len(tokenFile) == 0 {
		return &bearerAuthRoundTripper{bearer: bearer, rt: rt}, nil
	}
	source := newCachedFileTokenSource(tokenFile)
	if len(bearer) == 0 {
		token, err := source.Token()
		if err != nil {
			return nil, err
		}
		bearer = token
	}
	return &bearerAuthRoundTripper{bearer: bearer, source: source, rt: rt}, nil
}

func (rt *bearerAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(req.Header.Get("Authorization")) != 0 {
		return rt.rt.RoundTrip(req)
	}

	req = cloneRequest(req)
	token := rt.bearer
	if rt.source != nil {
		if refreshedToken, err := rt.source.Token(); err == nil {
			token = refreshedToken
		}
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	return rt.rt.RoundTrip(req)
}

// cloneRequest creates a shallow copy of the request along with a deep copy
// of the Header map.
func cloneRequest(req *http.Request) *http.Request {
	r := new(http.Request)
	*r = *req
	r.Header = make(http.Header, len(req.Header))
	for k, s := range req.Header {
		r.Header[k] = append([]string(nil), s...)
	}
	return r
}

// cachedFileTokenSource reads a token from a file and caches it for a period.
// This mirrors the behavior of k8s.io/client-go/transport.cachingTokenSource
// with a fileTokenSource underneath.
type cachedFileTokenSource struct {
	path   string
	period time.Duration
	leeway time.Duration

	mu  sync.RWMutex
	tok string
	exp time.Time

	// for testing
	now func() time.Time
}

func newCachedFileTokenSource(path string) *cachedFileTokenSource {
	return &cachedFileTokenSource{
		path: path,
		// This period was picked because it is half of the duration between when
		// the kubelet refreshes a projected service account token and when the
		// original token expires. Default token lifetime is 10 minutes, and the
		// kubelet starts refreshing at 80% of lifetime.
		period: time.Minute,
		leeway: 10 * time.Second,
		now:    time.Now,
	}
}

// Token returns the cached token, or reads a fresh one from disk if expired.
func (ts *cachedFileTokenSource) Token() (string, error) {
	now := ts.now()

	// fast path
	ts.mu.RLock()
	tok, exp := ts.tok, ts.exp
	ts.mu.RUnlock()

	if tok != "" && exp.Add(-ts.leeway).After(now) {
		return tok, nil
	}

	// slow path
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.tok != "" && ts.exp.Add(-ts.leeway).After(now) {
		return ts.tok, nil
	}

	newTok, err := readTokenFile(ts.path)
	if err != nil {
		if ts.tok != "" {
			// Return stale token on read failure, matching upstream behavior.
			return ts.tok, nil
		}
		return "", err
	}

	ts.tok = newTok
	ts.exp = now.Add(ts.period)
	return ts.tok, nil
}

func readTokenFile(path string) (string, error) {
	tokb, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read token file %q: %v", path, err)
	}
	tok := strings.TrimSpace(string(tokb))
	if len(tok) == 0 {
		return "", fmt.Errorf("read empty token from file %q", path)
	}
	return tok, nil
}
