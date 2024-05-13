// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package host

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures/*
var fixturesFS embed.FS

func (h *Host) uploadFixtures() {
	fixtures, err := fixturesFS.ReadDir("fixtures")
	require.NoError(h.t, err)
	tmpDir := h.t.TempDir()
	for _, fixture := range fixtures {
		fixturePath := filepath.Join("fixtures", fixture.Name())
		fixtureData, err := fixturesFS.ReadFile(fixturePath)
		require.NoError(h.t, err)
		fixturePath = filepath.Join(tmpDir, fixture.Name())
		err = os.WriteFile(fixturePath, fixtureData, 0644)
		require.NoError(h.t, err)
	}
	h.remote.MustExecute("sudo mkdir -p /run/fixtures")
	h.remote.MustExecute("sudo chmod 777 /run/fixtures")
	h.remote.CopyFolder(tmpDir, "/run/fixtures")
}

// StartExamplePythonApp starts an example Python app
func (h *Host) StartExamplePythonApp() {
	env := map[string]string{
		"DD_SERVICE": "example-python-app",
		"DD_ENV":     "e2e-installer",
		"DD_VERSION": "1.0",
	}
	h.remote.MustExecute("nohup python3 /run/fixtures/http_server.py & echo $! > /tmp/example_python_app.pid", components.WithEnvVariables(env))
}

// StopExamplePythonApp stops the example Python app
func (h *Host) StopExamplePythonApp() {
	h.remote.MustExecute("kill $(cat /tmp/example_python_app.pid)")
}

// CallExamplePythonApp calls the example Python app
func (h *Host) CallExamplePythonApp(traceID string) {
	h.remote.MustExecute(fmt.Sprintf(`curl -X GET "http://localhost:8080/" \
		-H "X-Datadog-Trace-Id: %s" \
		-H "X-Datadog-Parent-Id: %s" \
		-H "X-Datadog-Sampling-Priority: 2"`,
		traceID, traceID))
}
