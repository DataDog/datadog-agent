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
		require.Equal(t, http.StatusBadRequest, statusCode)
		require.Len(t, respBody, 0)
	}

	{
		url := "/dbconfig?pid=0"
		statusCode, _, respBody := doDBConfigRequest(t, url)
		require.Equal(t, http.StatusNotFound, statusCode)
		require.Len(t, respBody, 0)
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
	require.Nil(t, resource)
}

func launchFakeProcess(ctx context.Context, t *testing.T, tmp, procname string) int {
	// creates a symlink to /usr/bin/sleep to be able to create a fake
	// postgres process.
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("could not find sleep util")
	}
	fakePgPath := filepath.Join(tmp, procname)
	if err := os.Symlink(sleepPath, fakePgPath); err != nil {
		t.Fatalf("could not create fake process symlink: %v", err)
	}
	if err := os.Chmod(fakePgPath, 0700); err != nil {
		t.Fatalf("could not chmod fake process symlink: %v", err)
	}

	cmd := exec.CommandContext(ctx, fakePgPath, "5")
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
