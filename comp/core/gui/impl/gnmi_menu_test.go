// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package guiimpl

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/status"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
)

// fakeGNMIStatus answers the gNMI status section with a fixed JSON payload.
type fakeGNMIStatus struct {
	status.Component
	out []byte
	err error
}

func (f fakeGNMIStatus) GetStatusBySections(sections []string, format string, _ bool) ([]byte, error) {
	if len(sections) != 1 || sections[0] != "gnmi" || format != "json" {
		return nil, errors.New("unexpected status query")
	}
	return f.out, f.err
}

func TestGNMIDevicesConfigured(t *testing.T) {
	tests := []struct {
		name   string
		status status.Component
		want   bool
	}{
		{name: "no status component", status: nil, want: false},
		{name: "status error", status: fakeGNMIStatus{err: errors.New("no such section")}, want: false},
		{name: "no devices", status: fakeGNMIStatus{out: []byte(`{"devices":[]}`)}, want: false},
		{name: "invalid json", status: fakeGNMIStatus{out: []byte(`not json`)}, want: false},
		{name: "one device", status: fakeGNMIStatus{out: []byte(`{"devices":[{"Address":"10.0.0.1"}]}`)}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &gui{status: tt.status}
			assert.Equal(t, tt.want, g.gnmiDevicesConfigured())
		})
	}
}

// The gNMI status page is only offered in the menu when a gNMI device is configured.
func TestRenderIndexPageGNMIMenu(t *testing.T) {
	render := func(s status.Component) string {
		g := &gui{status: s, sysprobeConfig: sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{})}
		rr := httptest.NewRecorder()
		http.HandlerFunc(g.renderIndexPage).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		res := rr.Result()
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, res.StatusCode)
		return string(body)
	}

	assert.NotContains(t, render(fakeGNMIStatus{out: []byte(`{"devices":[]}`)}), "loadStatus('gnmi')")
	assert.Contains(t, render(fakeGNMIStatus{out: []byte(`{"devices":[{"Address":"10.0.0.1"}]}`)}), "loadStatus('gnmi')")
}
