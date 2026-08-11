// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installerimpl implements the installer metadata providers interface
package installerimpl

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	installerstatus "github.com/DataDog/datadog-agent/pkg/fleet/daemon/status"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

func setupFetcher(t *testing.T, response *installerstatus.Response, err error) {
	original := fetchInstallerStatus
	t.Cleanup(func() { fetchInstallerStatus = original })

	fetchInstallerStatus = func(_ context.Context) (*installerstatus.Response, error) {
		return response, err
	}
}

func getInstallerComp(t *testing.T) *inst {
	r := Requires{
		Log:        logmock.New(t),
		Config:     config.NewMock(t),
		Serializer: serializermock.NewMetricSerializer(t),
		Hostname:   hostnameimpl.NewHostnameService(),
	}

	comp := NewComponent(r).Comp
	i := comp.(*inst)
	i.hostname = "test hostname"
	return i
}

func TestGetPayload(t *testing.T) {
	diskSpace := uint64(12884901888)
	setupFetcher(t, &installerstatus.Response{
		InstallerVersion:   "7.76.0",
		AvailableDiskSpace: &diskSpace,
	}, nil)

	p := getInstallerComp(t).getPayload().(*Payload)

	assert.Equal(t, "test hostname", p.Hostname)
	assert.True(t, p.Timestamp <= time.Now().UnixNano())
	assert.NotEmpty(t, p.UUID)
	assert.Equal(t,
		map[string]interface{}{
			"installer_reachable":  true,
			"installer_version":    "7.76.0",
			"available_disk_space": uint64(12884901888),
		},
		p.Metadata)
}

// An unreachable installer is the normal case on a host without remote updates. The
// payload must still be produced: returning nil would make the inventory runner skip
// the submission entirely, which loses the absence signal.
func TestGetPayloadInstallerUnreachable(t *testing.T) {
	setupFetcher(t, nil, errors.New("dial unix: no such file or directory"))

	p := getInstallerComp(t).getPayload()
	require.NotNil(t, p)

	assert.Equal(t,
		map[string]interface{}{
			"installer_reachable": false,
		},
		p.(*Payload).Metadata)
}

// The daemon leaves available_disk_space unset when it cannot determine it. Reporting
// it as 0 would read as "disk full", which is a different fact entirely.
func TestGetPayloadUnknownDiskSpace(t *testing.T) {
	setupFetcher(t, &installerstatus.Response{InstallerVersion: "7.76.0"}, nil)

	p := getInstallerComp(t).getPayload().(*Payload)

	assert.Equal(t,
		map[string]interface{}{
			"installer_reachable": true,
			"installer_version":   "7.76.0",
		},
		p.Metadata)
}

// The backend routes /api/v1/metadata on the top-level JSON key, so renaming it
// silently drops the payload.
func TestPayloadTopLevelKey(t *testing.T) {
	setupFetcher(t, &installerstatus.Response{InstallerVersion: "7.76.0"}, nil)

	raw, err := json.Marshal(getInstallerComp(t).getPayload())
	require.NoError(t, err)

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Contains(t, decoded, "installer_metadata")
}

func TestWritePayload(t *testing.T) {
	diskSpace := uint64(12884901888)
	setupFetcher(t, &installerstatus.Response{
		InstallerVersion:   "7.76.0",
		AvailableDiskSpace: &diskSpace,
	}, nil)

	req := httptest.NewRequest("GET", "http://fake_url.com", nil)
	w := httptest.NewRecorder()

	getInstallerComp(t).writePayloadAsJSON(w, req)

	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)

	p := Payload{}
	require.NoError(t, json.Unmarshal(body, &p))

	assert.Equal(t, "test hostname", p.Hostname)
	assert.Equal(t, true, p.Metadata["installer_reachable"])
	assert.Equal(t, "7.76.0", p.Metadata["installer_version"])
	// JSON numbers decode to float64.
	assert.Equal(t, float64(12884901888), p.Metadata["available_disk_space"])
}
