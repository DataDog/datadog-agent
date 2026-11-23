// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestGetExpVarFetchesTraceAgentVars(t *testing.T) {
	m := ipcmock.New(t)

	var authHeader string
	srv := m.NewMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		fmt.Fprintf(w, `{"trace_writer": {"payloads": 7}}`)
	}))

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	_, portStr, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	// GetExpVar reads the debug port from the global config
	config.NewMockWithOverrides(t, map[string]interface{}{
		"cmd_host":              "127.0.0.1",
		"apm_config.debug.port": port,
	})

	fb := flarehelpers.NewFlareBuilderMock(t, true)
	require.NoError(t, GetExpVar(context.Background(), fb, m.GetClient()))

	fb.AssertFileContentMatch("payloads: 7", filepath.Join("expvar", "trace-agent"))
	assert.Empty(t, authHeader, "no auth token should be sent to /debug/vars")
}

func TestIncludeSystemProbeConfig(t *testing.T) {
	configmock.NewFromFile(t, "./testdata/datadog-agent.yaml")
	// create system-probe.yaml file because it's in .gitignore
	_, err := os.Create("./testdata/system-probe.yaml")
	require.NoError(t, err, "couldn't create system-probe.yaml")
	defer os.Remove("./testdata/system-probe.yaml")

	mock := flarehelpers.NewFlareBuilderMock(t, false)
	GetConfigFiles(mock, map[string]string{"": "./testdata/confd"})

	mock.AssertFileExists("etc", "datadog.yaml")
	mock.AssertFileExists("etc", "system-probe.yaml")
}

func TestIncludeConfigFiles(t *testing.T) {
	configmock.New(t)

	mock := flarehelpers.NewFlareBuilderMock(t, false)
	GetConfigFiles(mock, map[string]string{"": "./testdata/confd"})

	mock.AssertFileExists("etc/confd/test.yaml")
	mock.AssertFileExists("etc/confd/test.Yml")
	mock.AssertNoFileExists("etc/confd/not_included.conf")
}

func TestIncludeConfigFilesWithPrefix(t *testing.T) {
	configmock.New(t)

	mock := flarehelpers.NewFlareBuilderMock(t, false)
	GetConfigFiles(mock, map[string]string{"prefix": "./testdata/confd"})

	mock.AssertFileExists("etc/confd/prefix/test.yaml")
	mock.AssertFileExists("etc/confd/prefix/test.Yml")
	mock.AssertNoFileExists("etc/confd/prefix/not_included.conf")
}
