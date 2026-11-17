// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package run

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	healthprobe "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	healthprobefx "github.com/DataDog/datadog-agent/comp/core/healthprobe/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	pidimpl "github.com/DataDog/datadog-agent/comp/core/pid/impl"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"run"},
		run,
		func(_ pidimpl.Params, _ core.BundleParams, readiness *gpu.Readiness) {
			require.Nil(t, readiness, "GPU readiness should be disabled by default")
		})
}

func TestCommandPidfile(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"run", "--pidfile", "/pid/file"},
		run,
		func(pidParams pidimpl.Params, _ core.BundleParams) {
			require.Equal(t, "/pid/file", pidParams.PIDfilePath)
		})
}

func newGlobalParamsTest(t *testing.T) *command.GlobalParams {
	// Because getSharedFxOption uses fx.Invoke, demultiplexer component is built
	// which lead to build:
	//   - config.Component which requires a valid datadog.yaml
	//   - hostname.Component which requires a valid hostname
	configPath := path.Join(t.TempDir(), "datadog.yaml")
	err := os.WriteFile(configPath, []byte("hostname: test"), 0644)
	require.NoError(t, err)

	return &command.GlobalParams{
		ConfFilePath: configPath,
	}
}

func TestValidateContainerCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, container, apiKey, orgUUID string
		wantError                        bool
	}{
		{name: "native Agent"},
		{name: "container without credentials", container: "yes", wantError: true},
		{name: "resolved API key", container: "yes", apiKey: "test-key"},
		{name: "delegated authentication", container: "yes", orgUUID: "test-org"},
		{name: "both", container: "yes", apiKey: "test-key", orgUUID: "test-org"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DOCKER_DD_AGENT", tc.container)
			cfg := configmock.New(t)
			cfg.Set("api_key", tc.apiKey, pkgconfigmodel.SourceCLI)
			cfg.Set("delegated_auth.org_uuid", tc.orgUUID, pkgconfigmodel.SourceCLI)
			err := validateContainerCredentials(cfg)
			if tc.wantError {
				require.ErrorContains(t, err, "requires api_key or delegated_auth.org_uuid")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Pause the actual credential reader while Fx builds the probe's dependencies.
type delayedCredentialConfig struct {
	config.Component
	entered chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (c *delayedCredentialConfig) GetString(key string) string {
	if key == "api_key" {
		c.once.Do(func() { close(c.entered); <-c.release })
	}
	return c.Component.GetString(key)
}

func TestCredentialsBeforeReadinessProbe(t *testing.T) {
	// Other command tests create global health registrations. Use a fresh process
	// so this test can verify the real probe's successful HTTP response too.
	if os.Getenv("TEST_CREDENTIAL_READINESS_PROCESS") != "1" {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestCredentialsBeforeReadinessProbe$", "-test.v")
		cmd.Env = append(os.Environ(), "TEST_CREDENTIAL_READINESS_PROCESS=1")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
		return
	}
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid credentials", true: "missing credentials"}[fail], func(t *testing.T) {
			t.Setenv("DOCKER_DD_AGENT", "yes")
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			port := listener.Addr().(*net.TCPAddr).Port
			require.NoError(t, listener.Close())
			key := "test-key"
			if fail {
				key = ""
			}
			cfg := config.NewMockWithOverrides(t, map[string]interface{}{
				"health_port": port, "api_key": key, "delegated_auth.org_uuid": "",
			})
			release := make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			delayed := &delayedCredentialConfig{Component: cfg, entered: make(chan struct{}), release: release}
			logger := logmock.New(t)
			var probe healthprobe.Component
			apps := make(chan *fx.App, 1)
			go func() {
				apps <- fx.New(fx.NopLogger, fxutil.FxLifecycleAdapter(),
					fx.Provide(newContainerCredentials, healthProbeOptions,
						func() config.Component { return delayed },
						func() log.Component { return logger }),
					healthprobefx.Module(), fx.Populate(&probe))
			}()
			select {
			case <-delayed.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("credential validation did not begin")
			}
			client := &http.Client{Timeout: time.Second}
			url := fmt.Sprintf("http://127.0.0.1:%d/ready", port)
			// The actual health probe cannot listen while validation is paused.
			for range 3 {
				response, err := client.Get(url)
				if response != nil {
					response.Body.Close()
				}
				require.Error(t, err)
			}
			unblock()
			var app *fx.App
			select {
			case app = <-apps:
			case <-time.After(5 * time.Second):
				t.Fatal("credential validation did not return")
			}
			if fail {
				require.ErrorContains(t, app.Err(), "requires api_key or delegated_auth.org_uuid")
				response, err := client.Get(url)
				if response != nil {
					response.Body.Close()
				}
				require.Error(t, err, "failed validation must not open the probe")
				return
			}
			require.NoError(t, app.Err())
			require.NoError(t, app.Start(t.Context()))
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				require.NoError(t, app.Stop(ctx))
			})
			response, err := client.Get(url)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusOK, response.StatusCode)
		})
	}
}
