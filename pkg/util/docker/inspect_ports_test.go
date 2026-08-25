// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dcontainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// customerInspectJSON mirrors CONS-8441: a port RANGE key ("1061-1070", no
// protocol) in Config.ExposedPorts alongside explicit entries. Older Docker
// daemons return payloads like this, which moby's decoder rejects outright.
const customerInspectJSON = `{
  "Id": "85dfebd748501ca258af4f935d9d977be582aadedc2861dcfcdc1fecdc0dc250",
  "Config": {
    "ExposedPorts": {
      "1061-1070": {},
      "1061/udp": {},
      "9095/tcp": {}
    }
  }
}`

func mustPort(t *testing.T, s string) network.Port {
	t.Helper()
	p, err := network.ParsePort(s)
	require.NoErrorf(t, err, "bad test port %q", s)
	return p
}

func portSetKeys(m network.PortSet) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k.String())
	}
	return out
}

// Test_moby_rejects_port_range documents the underlying bug: the raw payload
// cannot be decoded into the moby struct because of the range key.
func Test_moby_rejects_port_range(t *testing.T) {
	var c dcontainer.InspectResponse
	err := json.Unmarshal([]byte(customerInspectJSON), &c)
	require.Error(t, err, "expected moby decode to reject the range key")
	assert.Contains(t, err.Error(), "invalid port '1061-1070'")
}

func Test_sanitizeInspectPortRanges(t *testing.T) {
	sanitized, changed := sanitizeInspectPortRanges([]byte(customerInspectJSON))
	require.True(t, changed, "expected the payload to be rewritten")

	// The sanitized payload must now decode cleanly into the moby struct.
	var c dcontainer.InspectResponse
	require.NoError(t, json.Unmarshal(sanitized, &c))
	assert.Equal(t, "85dfebd748501ca258af4f935d9d977be582aadedc2861dcfcdc1fecdc0dc250", c.ID)

	// The "1061-1070" range (no proto -> tcp) is expanded, while explicit
	// entries are preserved.
	require.NotNil(t, c.Config)
	ep := c.Config.ExposedPorts
	for p := 1061; p <= 1070; p++ {
		assert.Containsf(t, ep, mustPort(t, fmt.Sprintf("%d/tcp", p)),
			"expected expanded tcp port %d; have %v", p, portSetKeys(ep))
	}
	assert.Contains(t, ep, mustPort(t, "1061/udp"))
	assert.Contains(t, ep, mustPort(t, "9095/tcp"))
}

// Test_sanitizeInspectPortRanges_backCompat: a payload with only individual port
// keys is left untouched (no rewrite).
func Test_sanitizeInspectPortRanges_backCompat(t *testing.T) {
	in := `{"Id":"abc","Config":{"ExposedPorts":{"80/tcp":{},"443/tcp":{}}}}`
	_, changed := sanitizeInspectPortRanges([]byte(in))
	assert.False(t, changed, "individual ports must not trigger a rewrite")
}

// Test_sanitizeInspectPortRanges_dropMalformed: an unparseable key is dropped
// instead of failing the whole container.
func Test_sanitizeInspectPortRanges_dropMalformed(t *testing.T) {
	in := `{"Id":"abc","Config":{"ExposedPorts":{"notaport":{},"80/tcp":{}}}}`
	out, changed := sanitizeInspectPortRanges([]byte(in))
	require.True(t, changed)

	var c dcontainer.InspectResponse
	require.NoError(t, json.Unmarshal(out, &c))
	require.NotNil(t, c.Config)
	assert.Len(t, c.Config.ExposedPorts, 1)
	assert.Contains(t, c.Config.ExposedPorts, mustPort(t, "80/tcp"))
}

func Test_isInvalidPortKeyError(t *testing.T) {
	assert.True(t, isInvalidPortKeyError(errors.New("invalid port '1061-1070': invalid syntax")))
	assert.False(t, isInvalidPortKeyError(nil))
	assert.False(t, isInvalidPortKeyError(errors.New("connection refused")))
	// Must not match net/url dial errors on a misconfigured DOCKER_HOST.
	assert.False(t, isInvalidPortKeyError(errors.New(`dial tcp: address h:notaport: invalid port "notaport"`)))
}

// fakeInspectDaemon stands in for the Docker daemon, answering version
// negotiation and returning the given inspect body for any /json request.
func fakeInspectDaemon(t *testing.T, inspectBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/_ping":
			w.Header().Set("API-Version", "1.54")
			w.Header().Set("OSType", "linux")
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/json"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, inspectBody)
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// Test_InspectNoCache_recoversPortRange is the end-to-end reproduction: a real
// moby client talks to a fake daemon returning the CONS-8441 payload. The direct
// moby call fails, but DockerUtil.InspectNoCache recovers via sanitization.
func Test_InspectNoCache_recoversPortRange(t *testing.T) {
	srv := fakeInspectDaemon(t, customerInspectJSON)
	defer srv.Close()

	cli := newTestDockerClient(t, srv.URL)
	du := &DockerUtil{cli: cli, queryTimeout: 30 * time.Second}

	// The unrecovered path fails (documents the bug end-to-end).
	_, rawErr := cli.ContainerInspect(context.Background(), "85dfebd74850", client.ContainerInspectOptions{})
	require.Error(t, rawErr)
	assert.Contains(t, rawErr.Error(), "invalid port")

	// The recovered path succeeds.
	c, err := du.InspectNoCache(context.Background(), "85dfebd74850", false)
	require.NoError(t, err, "InspectNoCache should recover the container")
	assert.Equal(t, "85dfebd748501ca258af4f935d9d977be582aadedc2861dcfcdc1fecdc0dc250", c.ID)
	require.NotNil(t, c.Config)
	assert.Contains(t, c.Config.ExposedPorts, mustPort(t, "1065/tcp"))
}

// Test_InspectNoCache_surfacesOriginalError checks the recovery path cannot mask
// a genuine failure: a daemon error must be returned, not swallowed.
func Test_InspectNoCache_surfacesOriginalError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_ping" {
			w.Header().Set("API-Version", "1.54")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message":"boom"}`)
	}))
	defer srv.Close()

	du := &DockerUtil{cli: newTestDockerClient(t, srv.URL), queryTimeout: 30 * time.Second}
	_, err := du.InspectNoCache(context.Background(), "abc", false)
	require.Error(t, err, "daemon failure must not be masked by the recovery path")
	assert.Contains(t, err.Error(), "boom")
}

// Test_InspectNoCache_rejectsMismatchedContainer guards against returning a
// different container than the one requested: the container may be replaced
// between the failed inspect and the recovery refetch.
func Test_InspectNoCache_rejectsMismatchedContainer(t *testing.T) {
	body := `{"Id":"1111111111111111","Name":"/other","Config":{"ExposedPorts":{"1061-1070":{}}}}`
	srv := fakeInspectDaemon(t, body)
	defer srv.Close()

	du := &DockerUtil{cli: newTestDockerClient(t, srv.URL), queryTimeout: 30 * time.Second}
	_, err := du.InspectNoCache(context.Background(), "deadbeefdead", false)
	require.Error(t, err, "must not return a container that does not match the request")
}

// Test_sanitizeInspectPortRanges_dropsHugeRange: a pathological range is dropped
// rather than expanded into tens of thousands of ports.
func Test_sanitizeInspectPortRanges_dropsHugeRange(t *testing.T) {
	in := `{"Id":"abc","Config":{"ExposedPorts":{"1-65535":{},"80/tcp":{}}}}`
	out, changed := sanitizeInspectPortRanges([]byte(in))
	require.True(t, changed)

	var c dcontainer.InspectResponse
	require.NoError(t, json.Unmarshal(out, &c))
	require.NotNil(t, c.Config)
	assert.Len(t, c.Config.ExposedPorts, 1, "wide range must be dropped, explicit port kept")
	assert.Contains(t, c.Config.ExposedPorts, mustPort(t, "80/tcp"))
}

// Test_sanitizeInspectPortRanges_budgetIsAggregate: the expansion budget is
// shared across the whole object, so many individually-acceptable ranges cannot
// multiply into a huge port set.
func Test_sanitizeInspectPortRanges_budgetIsAggregate(t *testing.T) {
	// 40 ranges of 1000 ports each: every one is under the per-range width, but
	// together they would be 40k ports without an aggregate budget.
	keys := make([]string, 0, 40)
	for i := range 40 {
		start := 1000 + i*1000
		keys = append(keys, fmt.Sprintf(`"%d-%d/tcp":{}`, start, start+999))
	}
	in := `{"Id":"abc","Config":{"ExposedPorts":{` + strings.Join(keys, ",") + `}}}`

	out, changed := sanitizeInspectPortRanges([]byte(in))
	require.True(t, changed)

	var c dcontainer.InspectResponse
	require.NoError(t, json.Unmarshal(out, &c))
	require.NotNil(t, c.Config)
	assert.LessOrEqual(t, len(c.Config.ExposedPorts), maxExpandedPorts,
		"expansion must stay within the aggregate budget, got %d", len(c.Config.ExposedPorts))
}

// Test_sanitizeInspectPortRanges_deterministic: which ranges fit the budget must
// not depend on Go's randomised map iteration, or a container's reported ports
// would flap between inspects.
func Test_sanitizeInspectPortRanges_deterministic(t *testing.T) {
	in := `{"Id":"abc","Config":{"ExposedPorts":{` +
		`"2000-2799/tcp":{},"3000-3799/tcp":{},"4000-4799/tcp":{}}}}`
	first, changed := sanitizeInspectPortRanges([]byte(in))
	require.True(t, changed)
	for range 10 {
		next, _ := sanitizeInspectPortRanges([]byte(in))
		assert.JSONEq(t, string(first), string(next), "sanitizer output must be stable")
	}
}

// Test_InspectNoCache_recoveryRequestsSize: with withSize set, the recovery
// refetch must ask for size too, or the recovered container loses its size data.
func Test_InspectNoCache_recoveryRequestsSize(t *testing.T) {
	var gotQuery []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_ping" {
			w.Header().Set("API-Version", "1.54")
			w.WriteHeader(http.StatusOK)
			return
		}
		gotQuery = append(gotQuery, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"Id":"deadbeefdead","SizeRw":4096,`+
			`"Config":{"ExposedPorts":{"1061-1070":{}}}}`)
	}))
	defer srv.Close()

	du := &DockerUtil{cli: newTestDockerClient(t, srv.URL), queryTimeout: 30 * time.Second}
	c, err := du.InspectNoCache(context.Background(), "deadbeefdead", true)
	require.NoError(t, err)
	require.Len(t, gotQuery, 2, "expected the initial inspect plus the recovery refetch")
	assert.Equal(t, "size=1", gotQuery[1], "recovery refetch must carry size=1")
	require.NotNil(t, c.SizeRw)
	assert.Equal(t, int64(4096), *c.SizeRw)
}

// Test_InspectNoCache_healthyUnaffected confirms the fallback never engages for
// a well-formed payload (back-compat with modern daemons).
func Test_InspectNoCache_healthyUnaffected(t *testing.T) {
	body := `{"Id":"deadbeef","Config":{"ExposedPorts":{"80/tcp":{}}}}`
	srv := fakeInspectDaemon(t, body)
	defer srv.Close()

	du := &DockerUtil{cli: newTestDockerClient(t, srv.URL), queryTimeout: 30 * time.Second}
	c, err := du.InspectNoCache(context.Background(), "deadbeef", false)
	require.NoError(t, err)
	assert.Equal(t, "deadbeef", c.ID)
	require.NotNil(t, c.Config)
	assert.Contains(t, c.Config.ExposedPorts, mustPort(t, "80/tcp"))
}
