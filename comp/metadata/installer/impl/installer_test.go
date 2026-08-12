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
	statusapi "github.com/DataDog/datadog-agent/comp/updater/statusapi/def"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

type fakeStatusClient struct {
	status statusapi.Status
	err    error
}

func (f fakeStatusClient) Status(_ context.Context) (statusapi.Status, error) {
	return f.status, f.err
}

func getInstallerComp(t *testing.T, status statusapi.Status, err error) *inst {
	r := Requires{
		Log:        logmock.New(t),
		Config:     config.NewMock(t),
		Serializer: serializermock.NewMetricSerializer(t),
		Hostname:   hostnameimpl.NewHostnameService(),
	}

	comp := NewComponent(r).Comp
	i := comp.(*inst)
	i.hostname = "test hostname"
	i.installer = fakeStatusClient{status: status, err: err}
	return i
}

func TestGetPayload(t *testing.T) {
	diskSpace := uint64(12884901888)
	i := getInstallerComp(t, statusapi.Status{
		InstallerVersion:   "7.76.0",
		AvailableDiskSpace: &diskSpace,
	}, nil)

	p := i.getPayload().(*Payload)

	assert.Equal(t, "test hostname", p.Hostname)
	assert.True(t, p.Timestamp <= time.Now().UnixNano())
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
	i := getInstallerComp(t, statusapi.Status{}, errors.New("dial unix: no such file or directory"))

	p := i.getPayload()
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
	i := getInstallerComp(t, statusapi.Status{InstallerVersion: "7.76.0"}, nil)

	p := i.getPayload().(*Payload)

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
	i := getInstallerComp(t, statusapi.Status{InstallerVersion: "7.76.0"}, nil)

	raw, err := json.Marshal(i.getPayload())
	require.NoError(t, err)

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Contains(t, decoded, "installer_metadata")
}

func TestWritePayload(t *testing.T) {
	diskSpace := uint64(12884901888)
	i := getInstallerComp(t, statusapi.Status{
		InstallerVersion:   "7.76.0",
		AvailableDiskSpace: &diskSpace,
	}, nil)

	req := httptest.NewRequest("GET", "http://fake_url.com", nil)
	w := httptest.NewRecorder()

	i.writePayloadAsJSON(w, req)

	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)

	p := Payload{}
	require.NoError(t, json.Unmarshal(body, &p))

	// The endpoint only has to serve the same payload; TestGetPayload covers its
	// contents.
	assert.Equal(t, "test hostname", p.Hostname)
	assert.Equal(t, "7.76.0", p.Metadata["installer_version"])
}
