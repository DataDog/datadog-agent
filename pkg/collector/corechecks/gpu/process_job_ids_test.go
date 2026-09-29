// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && linux && nvml

package gpu

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/jobids"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server/testutil"
)

// startJobIDsServer starts a system-probe test server serving the /process-job-ids endpoint of the
// GPU module for the given procfs, and returns a JobIDReader connected to it.
func startJobIDsServer(t *testing.T, jobs gpuconfig.JobsConfig, procRoot string) JobIDReader {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/stats", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("{}")) })
	mux.Handle("/gpu"+spProcessJobIDsEndpoint, jobids.NewHandler(jobs, procRoot))

	socketPath := testutil.SystemProbeSocketPath(t, "gpu")
	server, err := testutil.NewSystemProbeTestServer(mux, socketPath)
	require.NoError(t, err)
	server.Start()
	t.Cleanup(server.Close)

	return newSystemProbeJobIDReader(sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(socketPath)))
}

func writeProcEnviron(t *testing.T, procRoot string, pid string, environ string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(procRoot, pid), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(procRoot, pid, "environ"), []byte(environ), 0o600))
}

func TestSystemProbeJobIDReader(t *testing.T) {
	procRoot := t.TempDir()
	writeProcEnviron(t, procRoot, "1234", "PATH=/usr/bin\x00JOB_ID=run-1\x00JOB_GROUP=group-1\x00SECRET=hunter2\x00")
	read := startJobIDsServer(t, testEnvJobsConfig, procRoot)

	ids, err := read(1234)
	require.NoError(t, err)
	assert.Equal(t, model.ProcessJobIDs{Run: "run-1", Group: "group-1"}, ids)

	ids, err = read(4242)
	require.NoError(t, err)
	assert.Equal(t, model.ProcessJobIDs{}, ids, "a process that does not exist has no environment")
}

func TestSystemProbeJobIDReaderUnavailable(t *testing.T) {
	// No server is listening on the socket: reading must fail instead of returning empty identifiers that look valid.
	socketPath := testutil.SystemProbeSocketPath(t, "gpu-down")
	read := newSystemProbeJobIDReader(sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(socketPath)))

	_, err := read(1234)
	assert.Error(t, err)
}
