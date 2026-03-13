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
	"github.com/DataDog/datadog-agent/pkg/compliance/types"
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

	fakePgConfPath := filepath.Join(tmp, "postgresql.conf")
	if err := os.WriteFile(fakePgConfPath, []byte(`wal_level = 'logical'`), 0644); err != nil {
		t.Fatal(err)
	}

	pid, stop := launchFakeProcess(ctx, t, tmp, "postgres", "--config-file="+fakePgConfPath)
	defer stop()
	url := fmt.Sprintf("/dbconfig?pid=%d", pid)
	statusCode, headers, respBody := doDBConfigRequest(t, url)
	require.Equal(t, http.StatusOK, statusCode)
	require.Equal(t, "application/json", headers.Get("Content-Type"))

	var resource *dbconfig.DBResource
	if err := json.Unmarshal(respBody, &resource); err != nil {
		t.Fatal(err)
	}
	require.Equal(t, types.ResourceTypeDbPostgresql, resource.Type)
	require.Equal(t, "postgres", resource.Config.ProcessName)
	require.NotEmpty(t, resource.Config.ProcessUser)
	require.Equal(t, fakePgConfPath, resource.Config.ConfigFilePath)
	require.NotEmpty(t, resource.Config.ConfigFileUser)
	require.NotEmpty(t, resource.Config.ConfigFileGroup)
	require.Equal(t, uint32(0644), resource.Config.ConfigFileMode)
	require.Equal(t, map[string]interface{}{"wal_level": "logical"}, resource.Config.ConfigData)
}

func launchFakeProcess(ctx context.Context, t *testing.T, tmp, procname string, args ...string) (int, func()) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	copyBin := func(w io.WriteCloser, r io.ReadCloser) {
		defer w.Close()
		defer r.Close()
		_, err = io.Copy(w, r)
		if err != nil {
			t.Fatal(err)
		}
	}

	binPath := filepath.Join(tmp, procname)
	r, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	w, err := os.OpenFile(binPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
	if err != nil {
		t.Fatal(err)
	}
	copyBin(w, r)
	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pipeR.Close()
	defer pipeW.Close()

	args = append([]string{"-test.run=TestComplianceCheckModuleLaunchFakeProcess", "--"}, args...)
	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = pipeW
	cmd.Env = append([]string{}, os.Environ()...)
	cmd.Env = append(cmd.Env, "GO_TEST_COMPLIANCE_CHECK_MODULE_FAKE_PROCESS=1")

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var b [16]byte
	if _, err := pipeR.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	go io.Copy(io.Discard, pipeR)

	pid := cmd.Process.Pid

	return pid, func() {
		cmd.Process.Kill()
		cmd.Process.Wait()
	}
}

func TestComplianceCheckModuleLaunchFakeProcess(t *testing.T) {
	if os.Getenv("GO_TEST_COMPLIANCE_CHECK_MODULE_FAKE_PROCESS") != "1" {
		t.Skip()
	}
	fmt.Printf("READY") // sending output to signal to caller that the process is properly execed and ready.
	time.Sleep(1 * time.Minute)
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
