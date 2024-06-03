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
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/stretchr/testify/assert"
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
	h.remote.MustExecute("sudo mkdir -p /opt/fixtures")
	h.remote.MustExecute("sudo chmod 777 /opt/fixtures")
	err = h.remote.CopyFolder(tmpDir, "/opt/fixtures")
	for _, fixture := range fixtures {
		if filepath.Ext(fixture.Name()) == ".sh" {
			fixturePath := filepath.Join("/opt/fixtures", fixture.Name())
			h.remote.MustExecute(fmt.Sprintf("chmod +x %s", fixturePath))
		}
	}

	require.NoError(h.t, err)
}

// StartExamplePythonApp starts an example Python app
func (h *Host) StartExamplePythonApp() {
	env := map[string]string{
		"DD_SERVICE": "example-python-app",
		"DD_ENV":     "e2e-installer",
		"DD_VERSION": "1.0",
	}
	h.remote.MustExecute(`sudo -E /opt/fixtures/run_http_server.sh`, client.WithEnvVariables(env))
}

// StopExamplePythonApp stops the example Python app
func (h *Host) StopExamplePythonApp() {
	h.remote.MustExecute("sudo pkill -f http_server.py")
}

// CallExamplePythonApp calls the example Python app
func (h *Host) CallExamplePythonApp(traceID string) {
	h.remote.MustExecute(fmt.Sprintf(`curl -X GET "http://localhost:8080/" \
		-H "X-Datadog-Trace-Id: %s" \
		-H "X-Datadog-Parent-Id: %s" \
		-H "X-Datadog-Sampling-Priority: 2"`,
		traceID, traceID))
}

// StartExamplePythonAppInDocker starts the example Python app in Docker
func (h *Host) StartExamplePythonAppInDocker() {
	h.remote.MustExecute(`sudo docker run --name python-app -d -p 8081:8080 -v /opt/fixtures/http_server.py:/usr/src/app/http_server.py public.ecr.aws/docker/library/python:3.8-slim python /usr/src/app/http_server.py`)
}

// StopExamplePythonAppInDocker stops the example Python app in Docker
func (h *Host) StopExamplePythonAppInDocker() {
	h.remote.MustExecute("sudo docker rm -f python-app")
}

// CallExamplePythonAppInDocker calls the example Python app in Docker
func (h *Host) CallExamplePythonAppInDocker(traceID string) {
	success := assert.Eventually(h.t, func() bool {
		_, err := h.remote.Execute(fmt.Sprintf(`curl -X GET "http://localhost:8081/" \
		-H "X-Datadog-Trace-Id: %s" \
		-H "X-Datadog-Parent-Id: %s" \
		-H "X-Datadog-Sampling-Priority: 2"`,
			traceID, traceID))
		return err == nil
	}, time.Second*3, time.Second*1)
	if !success {
		h.t.Log("Error calling example Python app in Docker")
	}
}

// SetBrokenDockerConfig injects a broken JSON in the Docker daemon configuration
func (h *Host) SetBrokenDockerConfig() {
	h.remote.MustExecute("echo 'broken' | sudo tee /etc/docker/daemon.json")
}

// SetBrokenDockerConfigAdditionalFields injects additional fields in the Docker daemon configuration
// these fields are not supported
func (h *Host) SetBrokenDockerConfigAdditionalFields() {
	h.remote.MustExecute(`echo '{"tomato": "potato"}' | sudo tee /etc/docker/daemon.json`)
}

// RemoveBrokenDockerConfig removes the broken configuration from the Docker daemon
func (h *Host) RemoveBrokenDockerConfig() {
	h.remote.MustExecute("sudo rm /etc/docker/daemon.json")
}

// SetupFakeAgentExp sets up a fake Agent experiment with configurable options.
func (h *Host) SetupFakeAgentExp() FakeAgent {
	vBroken := "/opt/datadog-packages/datadog-agent/vbroken"
	h.remote.MustExecute(fmt.Sprintf("sudo mkdir -p %s/embedded/bin", vBroken))
	h.remote.MustExecute(fmt.Sprintf("sudo mkdir -p %s/bin/agent", vBroken))

	h.remote.MustExecute(fmt.Sprintf("sudo ln -sf %s /opt/datadog-packages/datadog-agent/experiment", vBroken))
	f := FakeAgent{
		h:    h,
		path: vBroken,
	}

	// default with sigterm
	f.SetStopWithSigterm("trace-agent")
	f.SetStopWithSigterm("core-agent")
	f.SetStopWithSigterm("process-agent")

	return f
}

// FakeAgent represents a fake Agent.
type FakeAgent struct {
	h    *Host
	path string
}

func (f FakeAgent) setBinary(fixtureBinary, agent string) {
	pathToFixture := filepath.Join("/opt/fixtures", fixtureBinary)
	var pathToAgent string
	switch agent {
	case "trace-agent":
		pathToAgent = filepath.Join(f.path, "embedded/bin/trace-agent")
	case "process-agent":
		pathToAgent = filepath.Join(f.path, "embedded/bin/process-agent")
	case "core-agent":
		pathToAgent = filepath.Join(f.path, "bin/agent/agent")
	default:
		panic("unimplemented agent")
	}
	f.h.remote.MustExecute(fmt.Sprintf("sudo ln -sf %s %s", pathToFixture, pathToAgent))
}

// SetExit0 sets the fake Agent to exit with code 0.
func (f FakeAgent) SetExit0(agent string) FakeAgent {
	f.setBinary("exit0.sh", agent)
	return f
}

// SetExit1 sets the fake Agent to exit with code 1.
func (f FakeAgent) SetExit1(agent string) FakeAgent {
	f.setBinary("exit1.sh", agent)
	return f
}

// SetStopWithSigkill sets the fake Agent to stop with SIGKILL.
func (f FakeAgent) SetStopWithSigkill(agent string) FakeAgent {
	f.setBinary("stop_with_sigkill.sh", agent)
	return f
}

// SetStopWithSigterm sets the fake Agent to stop with SIGTERM.
func (f FakeAgent) SetStopWithSigterm(agent string) FakeAgent {
	f.setBinary("stop_with_sigterm.sh", agent)
	return f
}
