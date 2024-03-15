// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance/dbconfig"
	"github.com/stretchr/testify/require"
)

func TestComplianceModuleNoProcess(t *testing.T) {
	{
		url := "/dbconfig"
		statusCode, _, respBody := doDBConfigRequest(t, url)
		require.Contains(t, string(respBody), "pid query parameter is not an integer")
		require.Equal(t, http.StatusBadRequest, statusCode)
	}

	{
		url := "/dbconfig?pid=0"
		statusCode, _, respBody := doDBConfigRequest(t, url)
		require.Contains(t, "resource not found for pid=0", string(respBody))
		require.Equal(t, http.StatusNotFound, statusCode)
	}
}

func TestComplianceCheckModuleWithProcess(t *testing.T) {
	tmp := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pid := launchFakeProcess(ctx, t, tmp, "postgres")
	url := fmt.Sprintf("/dbconfig?pid=%d", pid)
	statusCode, headers, respBody := doDBConfigRequest(t, url)
	require.Equal(t, http.StatusOK, statusCode)
	require.Equal(t, "application/json", headers.Get("Content-Type"))

	var resource *dbconfig.DBResource
	if err := json.Unmarshal(respBody, &resource); err != nil {
		t.Fatal(err)
	}
	require.Equal(t, "db_postgresql", resource.Type)
	require.Equal(t, "postgres", resource.Config.ProcessName)
	require.NotEmpty(t, resource.Config.ProcessUser)
	require.Equal(t, filepath.Join(tmp, "postgresql.conf"), resource.Config.ConfigFilePath)
	require.NotEmpty(t, resource.Config.ConfigFileUser)
	require.NotEmpty(t, resource.Config.ConfigFileGroup)
	require.Equal(t, uint32(0600), resource.Config.ConfigFileMode)
	require.Equal(t, map[string]interface{}{"foo": "bar"}, resource.Config.ConfigData)
}

func launchFakeProcess(ctx context.Context, t *testing.T, tmp, procname string) int {
	fakePgBinPath := filepath.Join(tmp, "postgres")
	fakePgConfPath := filepath.Join(tmp, "postgresql.conf")

	if err := os.WriteFile(fakePgBinPath, []byte("#!/bin/bash\nsleep 10"), 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(fakePgConfPath, []byte(`foo = 'bar'`), 0600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(ctx, fakePgBinPath, fmt.Sprintf("--config-file=%s", fakePgConfPath))
	if err := cmd.Start(); err != nil {
		t.Fatalf("could not start fake process %q: %v", procname, err)
	}

	return cmd.Process.Pid
}

func doDBConfigRequest(t *testing.T, url string) (int, http.Header, []byte) {
	rec := httptest.NewRecorder()

	m := &complianceModule{}
	m.handleScanDBConfig(rec, httptest.NewRequest(http.MethodGet, url, nil))

	response := rec.Result()

	defer response.Body.Close()
	resBytes, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, response.Header, resBytes
}
