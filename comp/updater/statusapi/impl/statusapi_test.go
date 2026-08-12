// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package statusapiimpl

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	statusapi "github.com/DataDog/datadog-agent/comp/updater/statusapi/def"
)

type testStatusProvider struct {
	status statusapi.Status
}

func (t *testStatusProvider) GetStatus() statusapi.Status {
	return t.status
}

// serveHandler exercises the real routing and middleware over TCP: the transport
// differs per platform, but what travels over it does not. The listener itself is
// tested in statusapi_nix_test.go / statusapi_windows_test.go.
func serveHandler(t *testing.T, status statusapi.Status) string {
	t.Helper()

	srv := httptest.NewServer(newServer(&testStatusProvider{status: status}, nil).handler())
	t.Cleanup(srv.Close)

	return srv.URL
}

type response struct {
	code        int
	contentType string
	body        []byte
}

func get(t *testing.T, url string, path string) response {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, url+path, nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return response{code: resp.StatusCode, contentType: resp.Header.Get("Content-Type"), body: body}
}

func TestStatusAPIServesStatus(t *testing.T) {
	diskSpace := uint64(12884901888)
	url := serveHandler(t, statusapi.Status{
		InstallerVersion:   "7.76.0",
		AvailableDiskSpace: &diskSpace,
	})

	resp := get(t, url, "/status")
	require.Equal(t, http.StatusOK, resp.code)
	assert.Equal(t, "application/json", resp.contentType)

	var status statusapi.Status
	require.NoError(t, json.Unmarshal(resp.body, &status))
	assert.Equal(t, "7.76.0", status.InstallerVersion)
	require.NotNil(t, status.AvailableDiskSpace)
	assert.Equal(t, diskSpace, *status.AvailableDiskSpace)
}

// A daemon that could not determine the free space must leave the field out rather
// than report 0, which would read as a full disk.
func TestStatusAPIOmitsUnknownDiskSpace(t *testing.T) {
	resp := get(t, serveHandler(t, statusapi.Status{InstallerVersion: "7.76.0"}), "/status")
	require.Equal(t, http.StatusOK, resp.code)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(resp.body, &raw))
	assert.NotContains(t, raw, "available_disk_space")
}

// Only GET /status is served. Anything else has to 404 rather than be handled by
// accident, since this listener is reachable by an unprivileged user.
func TestStatusAPIServesNothingElse(t *testing.T) {
	url := serveHandler(t, statusapi.Status{})

	assert.Equal(t, http.StatusNotFound, get(t, url, "/").code)
	assert.Equal(t, http.StatusNotFound, get(t, url, "/catalog").code)
}
