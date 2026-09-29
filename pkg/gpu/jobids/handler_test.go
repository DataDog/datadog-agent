// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package jobids

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
)

var envJobs = gpuconfig.JobsConfig{
	Run:   gpuconfig.IdentifierConfig{Key: "JOB_ID", Type: gpuconfig.IdentifierTypeEnv},
	Group: gpuconfig.IdentifierConfig{Key: "JOB_GROUP", Type: gpuconfig.IdentifierTypeEnv},
}

// writeEnviron creates <procRoot>/<pid>/environ with the given variables, as NUL separated entries.
func writeEnviron(t *testing.T, procRoot string, pid int, vars ...string) {
	t.Helper()
	dir := filepath.Join(procRoot, strconv.Itoa(pid))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "environ"), []byte(strings.Join(vars, "\x00")+"\x00"), 0o600))
}

func doRequest(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/process-job-ids", strings.NewReader(body)))
	return rec
}

func pidRequest(t *testing.T, pid int) string {
	t.Helper()
	body, err := json.Marshal(model.ProcessJobIDsRequest{PID: pid})
	require.NoError(t, err)
	return string(body)
}

func decodeIDs(t *testing.T, rec *httptest.ResponseRecorder) model.ProcessJobIDs {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code)
	var ids model.ProcessJobIDs
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ids))
	return ids
}

func TestServeJobIDs(t *testing.T) {
	procRoot := t.TempDir()
	writeEnviron(t, procRoot, 1234, "PATH=/usr/bin", "JOB_ID=run-1", "JOB_GROUP=group-1", "SECRET=hunter2")
	h := NewHandler(envJobs, procRoot)

	rec := doRequest(t, h, pidRequest(t, 1234))

	assert.Equal(t, model.ProcessJobIDs{Run: "run-1", Group: "group-1"}, decodeIDs(t, rec))
}

func TestServeOnlyJobIDs(t *testing.T) {
	procRoot := t.TempDir()
	writeEnviron(t, procRoot, 1234, "JOB_ID=run-1", "SECRET=hunter2", "JOB_GROUP=group-1")
	h := NewHandler(envJobs, procRoot)

	rec := doRequest(t, h, pidRequest(t, 1234))

	assert.NotContains(t, rec.Body.String(), "hunter2", "nothing but the job identifiers may leave the process environment")
	assert.NotContains(t, rec.Body.String(), "PATH")
	assert.JSONEq(t, `{"run":"run-1","group":"group-1"}`, rec.Body.String())
}

func TestServeIgnoresRequestedVariables(t *testing.T) {
	procRoot := t.TempDir()
	writeEnviron(t, procRoot, 1234, "JOB_ID=run-1", "SECRET=hunter2")
	h := NewHandler(envJobs, procRoot)

	// The caller cannot ask for other variables: the request only has a pid and unknown fields are ignored.
	rec := doRequest(t, h, `{"pid":1234,"keys":["SECRET"],"key":"SECRET"}`)

	assert.NotContains(t, rec.Body.String(), "hunter2")
	assert.Equal(t, model.ProcessJobIDs{Run: "run-1"}, decodeIDs(t, rec))
}

func TestServeOnlyEnvIdentifiers(t *testing.T) {
	procRoot := t.TempDir()
	writeEnviron(t, procRoot, 1234, "JOB_ID=run-1", "JOB_GROUP=group-1")
	jobs := gpuconfig.JobsConfig{
		Run:   gpuconfig.IdentifierConfig{Key: "JOB_ID", Type: gpuconfig.IdentifierTypeLabel}, // not an env identifier
		Group: gpuconfig.IdentifierConfig{Key: "JOB_GROUP", Type: gpuconfig.IdentifierTypeEnv},
	}
	h := NewHandler(jobs, procRoot)

	rec := doRequest(t, h, pidRequest(t, 1234))

	assert.Equal(t, model.ProcessJobIDs{Group: "group-1"}, decodeIDs(t, rec), "label and annotation identifiers are not read from the process")
}

func TestServeMissingVariable(t *testing.T) {
	procRoot := t.TempDir()
	writeEnviron(t, procRoot, 1234, "PATH=/usr/bin")
	h := NewHandler(envJobs, procRoot)

	rec := doRequest(t, h, pidRequest(t, 1234))

	assert.Equal(t, model.ProcessJobIDs{}, decodeIDs(t, rec))
}

func TestServeProcessNotFound(t *testing.T) {
	h := NewHandler(envJobs, t.TempDir())

	rec := doRequest(t, h, pidRequest(t, 4242))

	assert.Equal(t, model.ProcessJobIDs{}, decodeIDs(t, rec), "a process that exited has no environment, which is not an error")
}

func TestServeInvalidRequests(t *testing.T) {
	h := NewHandler(envJobs, t.TempDir())

	assert.Equal(t, http.StatusBadRequest, doRequest(t, h, "not json").Code)
	assert.Equal(t, http.StatusBadRequest, doRequest(t, h, pidRequest(t, 0)).Code)
	assert.Equal(t, http.StatusBadRequest, doRequest(t, h, pidRequest(t, -1)).Code)
}
