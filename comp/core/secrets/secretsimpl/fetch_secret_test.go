// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsimpl

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func build(outTarget, pkg string) {
	output, err := exec.Command("go", "build", "-o", outTarget, pkg).CombinedOutput()
	if err != nil {
		fmt.Printf("Could not compile secret backend binary: %s\n%s", err, output)
		os.Exit(1)
	}
}

func TestLimitBuffer(t *testing.T) {
	lb := limitBuffer{
		buf: &bytes.Buffer{},
		max: 5,
	}

	n, err := lb.Write([]byte("012"))
	assert.Equal(t, 3, n)
	require.NoError(t, err)
	assert.Equal(t, []byte("012"), lb.buf.Bytes())

	n, err = lb.Write([]byte("abc"))
	assert.Equal(t, 0, n)
	require.NotNil(t, err)
	assert.Equal(t, []byte("012"), lb.buf.Bytes())

	n, err = lb.Write([]byte("ab"))
	assert.Equal(t, 2, n)
	require.NoError(t, err)
	assert.Equal(t, []byte("012ab"), lb.buf.Bytes())
}

// getBackendCommandBinary compiles a binary from source, then sets the proper
// permissions on it
func getBackendCommandBinary(t *testing.T) (string, func()) {
	platform := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)
	outFile, err := os.CreateTemp("", "test_command_"+platform)
	if err != nil {
		t.Fatal(err)
	}
	targetBin := outFile.Name()
	if runtime.GOOS == "windows" {
		targetBin = targetBin + ".exe"
	}
	outFile.Close()
	cleanup := func() {
		os.Remove(targetBin)
	}

	t.Logf("compiling secret backend binary '%s'", targetBin)
	build(targetBin, "./test/src/test_command")
	setCorrectRight(targetBin)

	return targetBin, cleanup
}

func TestExecCommandError(t *testing.T) {
	inputPayload := "{\"version\": \"1.0\" , \"secrets\": [\"sec1\", \"sec2\"]}"
	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())

	// Windows-only fix for running on CI
	testCheckRightsStub()

	backendCommandBin, cleanup := getBackendCommandBinary(t)
	defer cleanup()

	t.Run("Empty secretBackendCommand", func(t *testing.T) {
		resolver := newEnabledSecretResolver(tel)
		_, err := resolver.execCommand(inputPayload)
		// Error because resolver was not configured and has no command
		require.NotNil(t, err)
	})

	t.Run("timeout", func(t *testing.T) {
		resolver := newEnabledSecretResolver(tel)
		// The "timeout" arg makes the command sleep for 2 second, it should timeout
		resolver.Configure(secrets.ConfigParams{Command: backendCommandBin, Arguments: []string{"timeout"}, Timeout: 1})
		_, err := resolver.execCommand(inputPayload)
		require.NotNil(t, err)
		require.Equal(t, "error while running '"+backendCommandBin+"': command timeout", err.Error())
	})

	t.Run("No Error", func(t *testing.T) {
		resolver := newEnabledSecretResolver(tel)
		resolver.Configure(secrets.ConfigParams{Command: backendCommandBin})
		resp, err := resolver.execCommand(inputPayload)
		require.NoError(t, err)
		require.Equal(t, "{\"sec1\":{\"value\":\"arg_password\"}}", string(resp))
	})

	t.Run("Error returned", func(t *testing.T) {
		resolver := newEnabledSecretResolver(tel)
		// This "error" arg makes the command return an erroneous exit code
		resolver.Configure(secrets.ConfigParams{Command: backendCommandBin, Arguments: []string{"error"}})
		_, err := resolver.execCommand(inputPayload)
		require.NotNil(t, err)
	})

	t.Run("buffer limit", func(t *testing.T) {
		resolver := newEnabledSecretResolver(tel)
		// This "response_too_long" arg makes the command return too long of a response
		resolver.Configure(secrets.ConfigParams{Command: backendCommandBin, Arguments: []string{"response_too_long"}, MaxSize: 20})
		_, err := resolver.execCommand(inputPayload)
		require.NotNil(t, err)
		assert.Equal(t, "error while running '"+backendCommandBin+"': command output was too long: exceeded 20 bytes", err.Error())
	})
}

func TestFetchSecretExecError(t *testing.T) {
	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())
	resolver := newEnabledSecretResolver(tel)
	resolver.commandHookFunc = func(string) ([]byte, error) { return nil, fmt.Errorf("some error") }
	_, err := resolver.fetchSecret([]string{"handle1", "handle2"})
	assert.NotNil(t, err)
}

func TestFetchSecretUnmarshalError(t *testing.T) {
	tel := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	resolver := newEnabledSecretResolver(tel)
	resolver.commandHookFunc = func(string) ([]byte, error) { return []byte("{"), nil }
	_, err := resolver.fetchSecret([]string{"handle1", "handle2"})
	assert.NotNil(t, err)

	metrics, err := tel.GetCountMetric("secret_backend", "unmarshal_errors_count")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, map[string]string{}, metrics[0].Tags())
	assert.EqualValues(t, 1, metrics[0].Value())
}

func TestFetchSecretMissingSecret(t *testing.T) {
	tel := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	secrets := []string{"handle1", "handle2"}
	resolver := newEnabledSecretResolver(tel)
	resolver.commandHookFunc = func(string) ([]byte, error) { return []byte("{}"), nil }
	_, err := resolver.fetchSecret(secrets)
	assert.NotNil(t, err)
	assert.Equal(t, "secret handle 'handle1' was not resolved by the secret_backend_command", err.Error())
	checkErrorCountMetric(t, tel, 1, "missing", "handle1")
}

func TestFetchSecretErrorForHandle(t *testing.T) {
	tel := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	resolver := newEnabledSecretResolver(tel)
	resolver.commandHookFunc = func(string) ([]byte, error) {
		return []byte("{\"handle1\":{\"value\": null, \"error\": \"some error\"}}"), nil
	}
	_, err := resolver.fetchSecret([]string{"handle1"})
	assert.NotNil(t, err)
	assert.Equal(t, "an error occurred while resolving 'handle1': some error", err.Error())
	checkErrorCountMetric(t, tel, 1, "error", "handle1")
}

func TestFetchSecretEmptyValue(t *testing.T) {
	tel := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	resolver := newEnabledSecretResolver(tel)
	resolver.commandHookFunc = func(string) ([]byte, error) {
		return []byte("{\"handle1\":{\"value\": null}}"), nil
	}
	_, err := resolver.fetchSecret([]string{"handle1"})
	assert.NotNil(t, err)
	assert.Equal(t, "resolved secret for 'handle1' is empty", err.Error())
	checkErrorCountMetric(t, tel, 1, "empty", "handle1")

	resolver.commandHookFunc = func(string) ([]byte, error) {
		return []byte("{\"handle1\":{\"value\": \"\"}}"), nil
	}
	_, err = resolver.fetchSecret([]string{"handle1"})
	assert.NotNil(t, err)
	assert.Equal(t, "resolved secret for 'handle1' is empty", err.Error())
	checkErrorCountMetric(t, tel, 2, "empty", "handle1")
}

func checkErrorCountMetric(t *testing.T, tel telemetry.Mock, expected int, errorKind, handle string) {
	metrics, err := tel.GetCountMetric("secret_backend", "resolve_errors_count")
	require.NoError(t, err)
	require.NotEmpty(t, metrics)
	assert.EqualValues(t, expected, metrics[0].Value())
	expectedTags := map[string]string{
		"error_kind": errorKind,
		"handle":     handle,
	}
	assert.NotEqual(t, -1, slices.IndexFunc(metrics, func(m telemetry.Metric) bool {
		return int(m.Value()) == expected && maps.Equal(m.Tags(), expectedTags)
	}))
}

func TestFetchSecret(t *testing.T) {
	secrets := []string{"handle1", "handle2"}
	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())
	resolver := newEnabledSecretResolver(tel)
	// some dummy value to check the cache is not purge
	resolver.cache["test"] = "yes"
	resolver.commandHookFunc = func(string) ([]byte, error) {
		res := []byte(`{"handle1":{"value":"p1"},
		                "handle2":{"value":"p2"},
		                "handle3":{"value":"p3"}}`)
		return res, nil
	}
	resp, err := resolver.fetchSecret(secrets)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"handle1": "p1",
		"handle2": "p2",
	}, resp)
	assert.Equal(t, map[string]string{
		"test": "yes",
	}, resolver.cache)
}

func TestFetchSecretRemoveTrailingLineBreak(t *testing.T) {
	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())
	resolver := newEnabledSecretResolver(tel)
	resolver.commandHookFunc = func(string) ([]byte, error) {
		return []byte("{\"handle1\":{\"value\":\"some data\\r\\n\"}}"), nil
	}
	resolver.removeTrailingLinebreak = true
	secrets := []string{"handle1"}
	resp, err := resolver.fetchSecret(secrets)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"handle1": "some data"}, resp)
}

func TestFetchSecretPayloadIncludesBackendConfig(t *testing.T) {
	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())
	resolver := newEnabledSecretResolver(tel)
	resolver.backendType = "aws.secrets"
	resolver.backendConfig = map[string]interface{}{"foo": "bar"}
	var capturedPayload string
	resolver.commandHookFunc = func(payload string) ([]byte, error) {
		capturedPayload = payload
		return []byte(`{"handle1":{"value":"test_value"}}`), nil
	}
	_, err := resolver.fetchSecret([]string{"handle1"})
	require.NoError(t, err)
	assert.Contains(t, capturedPayload, `"type":"aws.secrets"`)
	assert.Contains(t, capturedPayload, `"config":{"foo":"bar"}`)
}
