// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

const (
	// ConfigTrustSite is a controlled HTTPS destination resolved only on the test VM.
	ConfigTrustSite = "config-trust.test:18443"
	// ConfigTrustAPIKey is synthetic: these tests must never submit an organization API key.
	ConfigTrustAPIKey = "00000000000000000000000000000000"
)

//go:embed fixtures/config-trust-receiver.ps1
var configTrustReceiverScript string

//go:embed fixtures/config-trust.ps1
var configTrustScript string

// ConfigTrustReceiver records installer requests on a local, machine-trusted HTTPS endpoint.
// It requires an otherwise clean Windows host with no installed Agent.
type ConfigTrustReceiver struct {
	host      *components.RemoteHost
	directory string
}

// ConfigTrustRequest is a request acknowledged by the receiver after being persisted.
type ConfigTrustRequest struct {
	Path   string `json:"path"`
	APIKey string `json:"apiKey"`
	Body   string `json:"body"`
}

// StartConfigTrustReceiver installs a temporary certificate and hosts entries and starts the
// receiver. Cleanup restores hosts, removes the certificate/binding/user, and saves diagnostics.
func StartConfigTrustReceiver(t *testing.T, host *components.RemoteHost, artifactDir string) *ConfigTrustReceiver {
	t.Helper()
	r := &ConfigTrustReceiver{host: host, directory: `C:\Windows\Temp\config-trust-` + uuid.NewString()}
	require.NoError(t, host.MkdirAll(r.directory))
	t.Cleanup(func() {
		_, err := r.run("Stop")
		assert.NoError(t, err, "restore certificate, hosts and receiver process")
		for _, name := range []string{"requests.jsonl", "stdout", "stderr"} {
			if contents, err := host.ReadFile(r.directory + `\` + name); err == nil {
				assert.NoError(t, os.WriteFile(filepath.Join(artifactDir, "config-trust-"+name), contents, 0600))
			}
		}
		assert.NoError(t, host.RemoveAll(r.directory))
	})
	_, err := host.WriteFile(r.directory+`\receiver.ps1`, []byte(configTrustReceiverScript))
	require.NoError(t, err)
	_, err = host.WriteFile(r.directory+`\test.ps1`, []byte(configTrustScript))
	require.NoError(t, err)
	_, err = r.run("Start")
	require.NoError(t, err)
	// Keep the receiver's SSH session open: Windows terminates child processes
	// when the session that launched them ends, even with Start-Process.
	session, _, _, err := host.Start(fmt.Sprintf(`& '%s\receiver.ps1' -Directory '%s' > '%s\stdout' 2> '%s\stderr'`,
		r.directory, r.directory, r.directory, r.directory))
	if session != nil {
		// Registered after receiver cleanup so the SSH session closes before Stop
		// releases the receiver's redirected output files.
		t.Cleanup(func() { _ = session.Close() })
	}
	require.NoError(t, err, "start HTTPS receiver in a persistent SSH session")
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := host.Lstat(r.directory + `\ready`)
		require.NoError(c, err, "HTTPS receiver must start")
	}, time.Minute, time.Second)
	r.control(t)
	return r
}

func (r *ConfigTrustReceiver) run(mode string) (string, error) {
	return r.host.Execute(fmt.Sprintf(`& '%s\test.ps1' -Mode '%s' -Directory '%s'`, r.directory, mode, r.directory))
}

func (r *ConfigTrustReceiver) control(t *testing.T) {
	t.Helper()
	_, err := r.run("Control")
	require.NoError(t, err, "HTTPS must remain reachable with normal certificate validation")
}

// PlantConfigRoot creates the directory and YAML with an untrusted BUILTIN\Users directory owner.
// Cleanup reclaims and removes the planted directory.
func (r *ConfigTrustReceiver) PlantConfigRoot(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		_, err := r.run("ClearConfig")
		assert.NoError(t, err, "remove planted configuration")
	})
	_, err := r.run("Plant")
	require.NoError(t, err)
}

// ClearConfigRoot removes configuration created by an installer positive control.
func (r *ConfigTrustReceiver) ClearConfigRoot(t *testing.T) {
	t.Helper()
	_, err := r.run("ClearConfig")
	require.NoError(t, err)
}

// Reset begins the negative observation phase after the positive-control process exits.
func (r *ConfigTrustReceiver) Reset(t *testing.T) {
	t.Helper()
	_, err := r.host.Execute(fmt.Sprintf(`Clear-Content '%s\requests.jsonl'`, r.directory))
	require.NoError(t, err)
}

func (r *ConfigTrustReceiver) requests() ([]ConfigTrustRequest, error) {
	contents, err := r.host.ReadFile(r.directory + `\requests.jsonl`)
	if err != nil {
		return nil, err
	}
	var requests []ConfigTrustRequest
	for _, line := range strings.Split(strings.TrimPrefix(string(contents), "\ufeff"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var request ConfigTrustRequest
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			return nil, err
		}
		if request.Path == "/control" {
			continue
		}
		body, err := base64.StdEncoding.DecodeString(request.Body)
		if err != nil {
			return nil, err
		}
		request.Body = string(body)
		requests = append(requests, request)
	}
	return requests, nil
}

// RequireTelemetry proves the real installer submitted the synthetic key to this receiver.
// For MSI controls, event also checks that the expected rollback event was received.
func (r *ConfigTrustReceiver) RequireTelemetry(t *testing.T, event string) {
	t.Helper()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		requests, err := r.requests()
		require.NoError(c, err)
		for _, request := range requests {
			if request.Path == "/api/v2/apmtelemetry" && request.APIKey == ConfigTrustAPIKey && strings.Contains(request.Body, event) {
				return
			}
		}
		assert.Fail(c, "installer positive control must submit telemetry with the synthetic API key")
	}, time.Minute, time.Second)
}

// AssertNoSubmissions must be called after the installer exits, including shutdown flushing.
// A final acknowledged HTTPS control proves the receiver remained live during observation.
func (r *ConfigTrustReceiver) AssertNoSubmissions(t *testing.T) {
	t.Helper()
	r.control(t)
	requests, err := r.requests()
	require.NoError(t, err)
	assert.Empty(t, requests, "planted configuration must direct no installer submissions")
}
