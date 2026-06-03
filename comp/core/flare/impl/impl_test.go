// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flareimpl

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flarebuilder "github.com/DataDog/datadog-agent/comp/core/flare/builder"
	flare "github.com/DataDog/datadog-agent/comp/core/flare/def"
	"github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type rcSettings struct {
	duration         time.Duration
	blockRate        int
	mutexFrac        int
	enableStreamLogs bool
}

func getFlare(t *testing.T, overrides map[string]interface{}, providers ...*types.FlareFiller) *flareImpl {
	return getFlareWithParams(t, flare.Params{}, overrides, providers...)
}

func getFlareWithParams(t *testing.T, params flare.Params, overrides map[string]interface{}, providers ...*types.FlareFiller) *flareImpl {
	return NewComponent(Requires{
		Log:       logmock.New(t),
		Config:    config.NewMockWithOverrides(t, overrides),
		Params:    params,
		Providers: providers,
		WMeta:     option.None[workloadmeta.Component](),
		IPC:       ipcmock.New(t),
	}).Comp.(*flareImpl)
}

// getFlareComponent returns the flare Component (public API) for tests that should only use the interface.
func getFlareComponent(t *testing.T, params flare.Params, overrides map[string]interface{}, providers ...*types.FlareFiller) flare.Component {
	return NewComponent(Requires{
		Log:       logmock.New(t),
		Config:    config.NewMockWithOverrides(t, overrides),
		Params:    params,
		Providers: providers,
		WMeta:     option.None[workloadmeta.Component](),
		IPC:       ipcmock.New(t),
	}).Comp
}

// CreateFlareBuilderMockFactory generates a FlareBuilderFactory that will output mocked builders when called.
func setupMockBuilder(t *testing.T) func() {
	fbFactory = func(localFlare bool, flareArgs types.FlareArgs) (types.FlareBuilder, error) {
		return NewFlareBuilderMockWithArgs(t, localFlare, flareArgs), nil
	}

	return func() {
		fbFactory = NewFlareBuilder
	}
}
func TestFlareCreation(t *testing.T) {
	realProvider := types.NewFiller(func(_ context.Context, _ types.FlareBuilder) error { return nil })

	flareComp := getFlare(
		t,
		map[string]interface{}{},
		nil,          // nil FlareFiller
		realProvider, // real FlareFiller
	)

	assert.GreaterOrEqual(t, len(flareComp.providers), 1)
	assert.NotContains(t, flareComp.providers, nil)
}

func TestRunProviders(t *testing.T) {
	firstStarted := make(chan struct{}, 1)
	var secondDone atomic.Bool

	flareComp := getFlare(t, nil)
	// We override the providers list as the default implementation adds ExtraFlareProviders and more. Those
	// extra providers would continue after the timeout and could access config after test cleanup.
	// Note: the callback goroutine may still run past the timeout (e.g. time.Sleep); subprocesses
	// started with exec.CommandContext(providerCtx, ...) are cancelled when the provider context times out.
	flareComp.providers = []*types.FlareFiller{
		types.NewFiller(func(_ context.Context, _ types.FlareBuilder) error {
			firstStarted <- struct{}{}
			return nil
		}),
		types.NewFiller(func(_ context.Context, _ types.FlareBuilder) error {
			time.Sleep(10 * time.Second)
			secondDone.Store(true)
			return nil
		}),
	}

	cliProviderTimeout := time.Nanosecond

	fb, err := NewFlareBuilder(false, flarebuilder.FlareArgs{})
	require.NoError(t, err)

	start := time.Now()
	flareComp.runProviders(fb, cliProviderTimeout)
	// ensure that providers are actually started
	<-firstStarted
	elapsed := time.Since(start)

	// ensure that we're not blocking for the slow provider
	assert.Less(t, elapsed, 5*time.Second)
	assert.False(t, secondDone.Load())
}

func TestRunProviders_CancelsSubprocessOnTimeout(t *testing.T) {
	errCh := make(chan error, 1)
	flareComp := getFlare(t, nil)
	flareComp.providers = []*types.FlareFiller{
		types.NewFiller(func(ctx context.Context, _ types.FlareBuilder) error {
			var cmd *exec.Cmd
			if runtime.GOOS == "windows" {
				cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", "Start-Sleep -Seconds 120")
			} else {
				cmd = exec.CommandContext(ctx, "sleep", "120")
			}
			errCh <- cmd.Run()
			return nil
		}),
	}

	fb, err := NewFlareBuilder(false, flarebuilder.FlareArgs{})
	require.NoError(t, err)

	flareComp.runProviders(fb, time.Nanosecond)

	select {
	case err := <-errCh:
		require.Error(t, err)
		ok := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
		if !ok {
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() != 0 {
				ok = true
			}
		}
		assert.True(t, ok, "subprocess should stop when provider ctx is canceled, got: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for subprocess to exit after provider context canceled")
	}
}

func TestAgentTaskFlareProfilingArgs(t *testing.T) {
	defer setupMockBuilder(t)()

	enabledDefaults := rcSettings{
		duration:  40 * time.Second,
		blockRate: 123,
		mutexFrac: 456,
	}

	disabledDefaults := rcSettings{}

	testCfg := map[string]interface{}{
		"site":                                "localhost", // Provide extra guarantees we don't try to send the flare off the box
		"flare.rc_profiling.profile_duration": enabledDefaults.duration,
		"flare.rc_profiling.blocking_rate":    enabledDefaults.blockRate,
		"flare.rc_profiling.mutex_fraction":   enabledDefaults.mutexFrac,
	}

	scenarios := []struct {
		name        string
		task        string
		expSettings rcSettings
	}{
		{
			name:        "Test profiling enabled",
			task:        "{\"args\":{\"case_id\":\"22420\",\"enable_profiling\":\"true\",\"user_handle\":\"no-reply@datadoghq.com\"},\"task_type\":\"flare\",\"uuid\":\"a_uuid\"}",
			expSettings: enabledDefaults,
		},
		{
			name:        "Test profiling disabled",
			task:        "{\"args\":{\"case_id\":\"22420\",\"enable_profiling\":\"false\",\"user_handle\":\"no-reply@datadoghq.com\"},\"task_type\":\"flare\",\"uuid\":\"a_uuid\"}",
			expSettings: disabledDefaults,
		},
		{
			name:        "Test profiling invalid",
			task:        "{\"args\":{\"case_id\":\"22420\",\"enable_profiling\":\"1\",\"user_handle\":\"no-reply@datadoghq.com\"},\"task_type\":\"flare\",\"uuid\":\"a_uuid\"}",
			expSettings: disabledDefaults,
		},
		{
			name:        "Test profiling not present",
			task:        "{\"args\":{\"case_id\":\"22420\",\"user_handle\":\"no-reply@datadoghq.com\"},\"task_type\":\"flare\",\"uuid\":\"a_uuid\"}",
			expSettings: disabledDefaults,
		},
	}

	runFlareTestScenarios(t, testCfg, scenarios, func(fb types.FlareBuilder, expSettings rcSettings) {
		assert.Equal(t, expSettings.duration, fb.GetFlareArgs().ProfileDuration)
		assert.Equal(t, expSettings.blockRate, fb.GetFlareArgs().ProfileBlockingRate)
		assert.Equal(t, expSettings.mutexFrac, fb.GetFlareArgs().ProfileMutexFraction)
	})
}

func TestAgentTaskFlareStreamLogsArgs(t *testing.T) {
	defer setupMockBuilder(t)()

	enabledDefaults := rcSettings{
		enableStreamLogs: true,
		duration:         60 * time.Second,
	}

	disabledDefaults := rcSettings{
		enableStreamLogs: false,
	}

	testCfg := map[string]interface{}{
		"site":                         "localhost", // Provide extra guarantees we don't try to send the flare off the box
		"flare.rc_streamlogs.duration": enabledDefaults.duration,
	}

	scenarios := []struct {
		name        string
		task        string
		expSettings rcSettings
	}{
		{
			name:        "Test stream logs enabled",
			task:        "{\"args\":{\"case_id\":\"22420\",\"enable_streamlogs\":\"true\",\"user_handle\":\"no-reply@datadoghq.com\"},\"task_type\":\"flare\",\"uuid\":\"a_uuid\"}",
			expSettings: enabledDefaults,
		},
		{
			name:        "Test stream logs disabled",
			task:        "{\"args\":{\"case_id\":\"22420\",\"enable_streamlogs\":\"false\",\"user_handle\":\"no-reply@datadoghq.com\"},\"task_type\":\"flare\",\"uuid\":\"a_uuid\"}",
			expSettings: disabledDefaults,
		},
		{
			name:        "Test stream logs invalid",
			task:        "{\"args\":{\"case_id\":\"22420\",\"enable_streamlogs\":\"1\",\"user_handle\":\"no-reply@datadoghq.com\"},\"task_type\":\"flare\",\"uuid\":\"a_uuid\"}",
			expSettings: disabledDefaults,
		},
		{
			name:        "Test stream logs not present",
			task:        "{\"args\":{\"case_id\":\"22420\",\"user_handle\":\"no-reply@datadoghq.com\"},\"task_type\":\"flare\",\"uuid\":\"a_uuid\"}",
			expSettings: disabledDefaults,
		},
	}

	runFlareTestScenarios(t, testCfg, scenarios, func(fb types.FlareBuilder, expSettings rcSettings) {
		assert.Equal(t, expSettings.duration, fb.GetFlareArgs().StreamLogsDuration)
	})
}

func TestSendRemovesArchiveAfterSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "flare.zip")
	require.NoError(t, os.WriteFile(archivePath, []byte("fake flare"), 0600))

	origSendTo := sendToFunc
	defer func() { sendToFunc = origSendTo }()
	sendToFunc = func(_ model.Reader, _ string, _ string, _ string, _ string, _ string, _ types.FlareSource) (string, error) {
		return "success", nil
	}

	comp := getFlareComponent(t, flare.Params{}, nil)
	_, err := comp.Send(archivePath, "case1", "test@example.com", types.NewLocalFlareSource())
	require.NoError(t, err)

	_, statErr := os.Stat(archivePath)
	assert.True(t, os.IsNotExist(statErr), "flare archive should be removed after successful send")
}

func TestSendKeepsArchiveWhenKeepArchiveAfterSend(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "flare.zip")
	require.NoError(t, os.WriteFile(archivePath, []byte("fake flare"), 0600))

	origSendTo := sendToFunc
	defer func() { sendToFunc = origSendTo }()
	sendToFunc = func(_ model.Reader, _ string, _ string, _ string, _ string, _ string, _ types.FlareSource) (string, error) {
		return "success", nil
	}

	comp := getFlareComponent(t, flare.Params{KeepArchiveAfterSend: true}, nil)
	_, err := comp.Send(archivePath, "case1", "test@example.com", types.NewLocalFlareSource())
	require.NoError(t, err)

	_, statErr := os.Stat(archivePath)
	assert.NoError(t, statErr, "flare archive should be kept when KeepArchiveAfterSend is true")
}

func runFlareTestScenarios(t *testing.T, testCfg map[string]interface{}, scenarios []struct {
	name        string
	task        string
	expSettings rcSettings
}, assertFunc func(fb types.FlareBuilder, expSettings rcSettings)) {
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			flareComp := getFlare(t, testCfg)

			flareComp.providers = []*types.FlareFiller{
				types.NewFiller(func(_ context.Context, fb types.FlareBuilder) error {
					assertFunc(fb, s.expSettings)
					return nil
				}),
			}
			atc, err := rcclienttypes.ParseConfigAgentTask([]byte(s.task), state.Metadata{})
			assert.NoError(t, err)

			flareComp.onAgentTaskEvent(rcclienttypes.TaskFlare, atc)
		})
	}
}

func TestLocalFlareFileContent(t *testing.T) {
	var mockBuilder *FlareBuilderMock
	fbFactory = func(localFlare bool, flareArgs types.FlareArgs) (types.FlareBuilder, error) {
		mockBuilder = NewFlareBuilderMockWithArgs(t, localFlare, flareArgs)
		return mockBuilder, nil
	}
	defer func() { fbFactory = NewFlareBuilder }()

	errIpc := errors.New("connection refused")
	flareComp := getFlareWithParams(t, flare.NewLocalParams("", "", "", "", "", ""), nil)
	// Override providers to prevent them from running past the timeout and writing into
	// t.TempDir()-backed flare directories during test cleanup.
	// This also avoids side-effects caused by default providers.
	// The "local" file under test is written before providers run, so this is safe to nil out.
	flareComp.providers = nil
	// Save() is a no-op in the mock and returns an error; the local file is still written.
	_, _ = flareComp.Create(types.ProfileData{}, 0, errIpc, []byte{})

	require.NotNil(t, mockBuilder)
	mockBuilder.AssertFileContentMatch(`unable to contact the agent to retrieve flare: connection refused`, "local")
	mockBuilder.AssertFileContentMatch(`Flare creation time: \d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`, "local")
	mockBuilder.AssertFileContentMatch(`Go version: go\d+\.\d+`, "local")
}
