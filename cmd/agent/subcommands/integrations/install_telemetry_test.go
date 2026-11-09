// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package integrations

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/coreos/go-semver/semver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	installertelemetry "github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
)

// fakeAgentTelemetry records the events sent through it, optionally blocking each send.
type fakeAgentTelemetry struct {
	events  map[string][][]byte
	unblock chan struct{}
}

func (f *fakeAgentTelemetry) SendEvent(eventType string, eventPayload []byte) error {
	if f.unblock != nil {
		<-f.unblock
	}
	if f.events == nil {
		f.events = map[string][][]byte{}
	}
	f.events[eventType] = append(f.events[eventType], eventPayload)
	return nil
}

func (f *fakeAgentTelemetry) SubmitErrorLog(errortracking.ErrorLog) {}

func (f *fakeAgentTelemetry) StartStartupSpan(string) (*installertelemetry.Span, context.Context) {
	return &installertelemetry.Span{}, context.Background()
}

// The payload's field names are the contract with the ddcoat dashboards and monitors.
func TestInstallEventPayload(t *testing.T) {
	ev := newInstallEvent(&cliParams{thirdParty: true})
	ev.IntegrationName, ev.IntegrationVersion = "datadog-acme", "1.2.0"
	ev.InstallType, ev.PreviousIntegrationVersion = installTypeUpgrade, "1.1.0"
	ev.recordDownload(&downloaderResult{
		DownloaderVersion: "9.5.0",
		WheelPath:         "/wheels/datadog-acme/datadog_acme-1.2.0-py3-none-any.whl",
		ErrorCategory:     categoryIntegrity,
		V2:                &repositoryAttempt{Result: "failure", ExceptionClass: "DigestMismatch", DurationMs: ptr(int64(0))},
		V1:                &repositoryAttempt{Result: "skipped"},
	}, 3*time.Second)
	ev.finish(withCategory(downloadCategory(&downloaderResult{ErrorCategory: categoryIntegrity}), errors.New("digest")), 5*time.Second)

	payload, err := json.Marshal(ev)

	require.NoError(t, err)
	assert.JSONEq(t, `{
		"schema_version": 1,
		"integration_name": "datadog-acme",
		"integration_version": "1.2.0",
		"integration_source": "extras",
		"result": "failure",
		"failed_step": "download",
		"error_category": "integrity",
		"unsafe_disable_verification": false,
		"install_type": "upgrade",
		"previous_integration_version": "1.1.0",
		"duration_ms": 5000,
		"download": {
			"downloader_version": "9.5.0",
			"wheel_path": "/wheels/datadog-acme/datadog_acme-1.2.0-py3-none-any.whl",
			"duration_ms": 3000,
			"v2": {"result": "failure", "exception_class": "DigestMismatch", "duration_ms": 0},
			"v1": {"result": "skipped"}
		}
	}`, string(payload))
}

func TestInstallEventFailure(t *testing.T) {
	tests := []struct {
		name     string
		params   cliParams
		err      error
		running  string
		step     string
		category string
		result   string
	}{
		{
			name:     "customer's mistake",
			err:      withCategory(categoryVersionBelowShipped, errors.New("too old")),
			step:     stepValidate,
			category: categoryVersionBelowShipped,
			result:   resultUserError,
		},
		{
			name:     "a local wheel that isn't a check is the customer's",
			params:   cliParams{localWheel: true},
			err:      withCategory(categoryNotAgentCheck, errors.New("not a check")),
			step:     stepCompatibility,
			category: categoryNotAgentCheck,
			result:   resultUserError,
		},
		{
			name:     "a published wheel that isn't a check is ours",
			err:      withCategory(categoryNotAgentCheck, errors.New("not a check")),
			step:     stepCompatibility,
			category: categoryNotAgentCheck,
			result:   resultFailure,
		},
		{
			name:     "an environment error while validating",
			err:      withCategory(categoryEnvironmentError, errors.New("no python")),
			running:  stepValidate,
			step:     stepValidate,
			category: categoryEnvironmentError,
			result:   resultFailure,
		},
		{
			name:     "an environment error while checking compatibility",
			err:      withCategory(categoryEnvironmentError, errors.New("no base version")),
			running:  stepCompatibility,
			step:     stepCompatibility,
			category: categoryEnvironmentError,
			result:   resultFailure,
		},
		{
			name:     "an uncategorized error fails the current step",
			err:      errors.New("unexpected"),
			running:  stepPipInstall,
			step:     stepPipInstall,
			category: categoryOther,
			result:   resultFailure,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := newInstallEvent(&tt.params)
			ev.step = tt.running
			if ev.step == "" {
				ev.step = stepPipInstall
			}

			ev.finish(tt.err, time.Second)

			assert.Equal(t, tt.result, ev.Result)
			assert.Equal(t, tt.step, ev.FailedStep)
			assert.Equal(t, tt.category, ev.ErrorCategory)
		})
	}
}

// Monitors count result:failure without exclusion lists, so only the categories that are certainly
// the customer's may be user errors.
func TestUserErrorCategories(t *testing.T) {
	userErrors := map[string]bool{
		categoryInvalidArguments:     true,
		categoryNotDatadogPackage:    true,
		categoryBasePackageRefused:   true,
		categoryUserNotAllowed:       true,
		categoryVersionBelowShipped:  true,
		categoryLocalWheelUnreadable: true,
		categoryConfigMoveFailed:     true,
	}
	for category := range categorySteps {
		t.Run(category, func(t *testing.T) {
			expected := resultFailure
			if userErrors[category] {
				expected = resultUserError
			}
			assert.Equal(t, expected, resultFor(category, sourceCore))
		})
	}
	assert.Equal(t, resultFailure, resultFor(categoryOther, sourceCore))
	assert.Equal(t, resultUserError, resultFor(categoryNotAgentCheck, sourceLocal))
}

func TestDownloadCategory(t *testing.T) {
	tests := []struct {
		name     string
		result   *downloaderResult
		expected string
	}{
		{name: "downloader category", result: &downloaderResult{ErrorCategory: "metadata"}, expected: categoryMetadata},
		{name: "downloader without a result file", result: nil, expected: categoryDownloaderNoResult},
		{name: "category of another step", result: &downloaderResult{ErrorCategory: categoryInvalidArguments}, expected: categoryOther},
		{name: "unknown category", result: &downloaderResult{ErrorCategory: "/home/user/secret"}, expected: categoryOther},
		{name: "no category", result: &downloaderResult{}, expected: categoryOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, downloadCategory(tt.result))
		})
	}
}

func TestInstallTypeFor(t *testing.T) {
	v := semver.New("2.0.0")
	tests := []struct {
		name     string
		current  *semver.Version
		found    bool
		expected string
	}{
		{name: "not installed", found: false, expected: installTypeNew},
		{name: "older installed", current: semver.New("1.9.0"), found: true, expected: installTypeUpgrade},
		{name: "newer installed", current: semver.New("2.1.0"), found: true, expected: installTypeDowngrade},
		{name: "same installed", current: semver.New("2.0.0"), found: true, expected: installTypeUnchanged},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, installTypeFor(v, tt.current, tt.found))
		})
	}
}

func TestValidateArgsCategory(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		local    bool
		expected string
	}{
		{name: "no argument", args: nil, expected: categoryInvalidArguments},
		{name: "too many arguments", args: []string{"datadog-a", "datadog-b"}, expected: categoryInvalidArguments},
		{name: "not a Datadog package", args: []string{"requests"}, expected: categoryNotDatadogPackage},
		{name: "missing local wheel", args: []string{filepath.Join(t.TempDir(), "missing.whl")}, local: true, expected: categoryLocalWheelUnreadable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, errorCategory(validateArgs(tt.args, tt.local)))
		})
	}
}

func TestBaseIncompatibilityCategory(t *testing.T) {
	tests := []struct {
		name     string
		metadata *string
		expected string
	}{
		{name: "no base dependency", metadata: ptr("Name: datadog-acme\nRequires-Dist: requests\n"), expected: categoryNotAgentCheck},
		{name: "base version mismatch", metadata: ptr("Name: datadog-acme\nRequires-Dist: datadog-checks-base>=99.0.0\n"), expected: categoryBaseVersionIncompatible},
		{name: "unreadable wheel", metadata: nil, expected: categoryWheelUnreadable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wheel := filepath.Join(t.TempDir(), "datadog_acme-1.0.0-py3-none-any.whl")
			if tt.metadata != nil {
				writeWheel(t, wheel, *tt.metadata)
			} else {
				require.NoError(t, os.WriteFile(wheel, []byte("not a zip"), 0o600))
			}
			assert.Equal(t, tt.expected, baseIncompatibilityCategory(wheel))
		})
	}
}

func TestReadDownloaderResult(t *testing.T) {
	tests := []struct {
		name     string
		content  *string
		expected *downloaderResult
	}{
		{
			name: "fallback to v1 that failed",
			content: ptr(`{"downloader_version": "9.5.0", "error_category": "not_found",
				"v2": {"result": "failure", "exception_class": "TargetNotFoundError", "duration_ms": 120},
				"v1": {"result": "failure", "exception_class": "NoSuchDatadogPackageVersion", "duration_ms": 900}}`),
			expected: &downloaderResult{
				DownloaderVersion: "9.5.0",
				ErrorCategory:     "not_found",
				V2:                &repositoryAttempt{Result: "failure", ExceptionClass: "TargetNotFoundError", DurationMs: ptr(int64(120))},
				V1:                &repositoryAttempt{Result: "failure", ExceptionClass: "NoSuchDatadogPackageVersion", DurationMs: ptr(int64(900))},
			},
		},
		{name: "downloader that doesn't write the file", content: nil},
		{name: "empty file", content: ptr("")},
		{name: "malformed file", content: ptr(`{"v2": `)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "result.json")
			if tt.content != nil {
				require.NoError(t, os.WriteFile(path, []byte(*tt.content), 0o600))
			}
			assert.Equal(t, tt.expected, readDownloaderResult(path))
		})
	}
}

// Pins the contract with the downloader: the option that requests the summary, which is read even
// though the downloader prints only the wheel path.
func TestDownloadWheelReturnsDownloaderSummary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake downloader is a shell script")
	}
	oldRootDir := rootDir
	t.Cleanup(func() { rootDir = oldRootDir })
	rootDir = t.TempDir()

	wheel := filepath.Join(rootDir, "datadog_postgres-18.0.0-py3-none-any.whl")
	require.NoError(t, os.WriteFile(wheel, nil, 0o600))
	fakePython := filepath.Join(rootDir, getRelPyPath())
	require.NoError(t, os.MkdirAll(filepath.Dir(fakePython), 0o755))
	script := `#!/bin/sh
while [ $# -gt 0 ]; do
  if [ "$1" = "--result-file" ]; then
    echo '{"downloader_version": "9.5.0", "v2": {"result": "success", "duration_ms": 80}, "v1": {"result": "skipped"}}' > "$2"
  fi
  shift
done
echo "` + wheel + `"
`
	require.NoError(t, os.WriteFile(fakePython, []byte(script), 0o755))

	path, result, err := downloadWheel(&cliParams{}, "datadog-postgres", "18.0.0", "core")

	require.NoError(t, err)
	assert.Equal(t, wheel, path)
	assert.Equal(t, &downloaderResult{
		DownloaderVersion: "9.5.0",
		V2:                &repositoryAttempt{Result: "success", DurationMs: ptr(int64(80))},
		V1:                &repositoryAttempt{Result: "skipped"},
	}, result)
}

// A local wheel can be a customer's own check, so its name must never leave the host.
func TestInstallEventOmitsLocalWheelName(t *testing.T) {
	ev := newInstallEvent(&cliParams{localWheel: true})
	ev.IntegrationName, ev.IntegrationVersion = "datadog-acme-internal", "1.0.0"

	ev.finish(nil, time.Second)

	assert.Empty(t, ev.IntegrationName)
	assert.Empty(t, ev.IntegrationVersion)
	assert.Equal(t, sourceLocal, ev.IntegrationSource)
}

// An unreachable intake must never hold the install for longer than the send budget.
func TestSendInstallEventGivesUpAfterTimeout(t *testing.T) {
	atel := &fakeAgentTelemetry{unblock: make(chan struct{})}
	defer close(atel.unblock)

	start := time.Now()
	sendInstallEvent(atel, installEvent{IntegrationName: "datadog-postgres"}, 50*time.Millisecond)

	assert.Less(t, time.Since(start), time.Second)
}

func TestInstallSendsOneEventWhenTheInstallFails(t *testing.T) {
	atel := &fakeAgentTelemetry{}
	params := &cliParams{args: []string{"datadog-postgres==18.0.0"}}

	err := install(params, nil, atel)

	// Outside an Agent installation, the command can't find the Agent's Python.
	require.Error(t, err)
	require.Len(t, atel.events[installEventName], 1)
	var ev map[string]any
	require.NoError(t, json.Unmarshal(atel.events[installEventName][0], &ev))
	assert.Equal(t, "failure", ev["result"])
	assert.Equal(t, "validate", ev["failed_step"])
	assert.Equal(t, "environment_error", ev["error_category"])
	assert.Equal(t, "core", ev["integration_source"])
}

func writeWheel(t *testing.T, path, metadata string) {
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	w := zip.NewWriter(f)
	m, err := w.Create("datadog_acme-1.0.0.dist-info/METADATA")
	require.NoError(t, err)
	_, err = m.Write([]byte(metadata))
	require.NoError(t, err)
	require.NoError(t, w.Close())
}

func ptr[T any](v T) *T {
	return &v
}
