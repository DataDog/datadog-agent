// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsimpl

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
)

var (
	binExtension = ""
)

func build(m *testing.M, outBin, pkg string) { //nolint:revive // TODO fix revive unused-parameter
	_, err := exec.Command("go", "build", "-o", outBin+binExtension, pkg).Output()
	if err != nil {
		fmt.Printf("Could not compile test secretBackendCommand: %s", err)
		os.Exit(1)
	}
}

func TestMain(m *testing.M) {
	// TODO: remove once the issue is resolved
	//
	// This test has an issue on Windows where it can block entirely and timeout after 4 minutes
	// (while the go test timeout is 3 minutes), and in that case it doesn't print stack traces.
	//
	// As a workaround, we explicitly write routine stack traces in an artifact if the test
	// is not finished after 2 minutes.
	if _, ok := os.LookupEnv("CI_PIPELINE_ID"); ok && runtime.GOOS == "windows" {
		done := make(chan struct{}, 1)
		defer func() {
			done <- struct{}{}
		}()
		go func() {
			select {
			case <-done:
			case <-time.After(2 * time.Minute):
				// files junit-*.tgz are automatically considered as artifacts
				file, err := os.OpenFile(`C:\mnt\junit-TestExecCommandError.tgz`, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					fmt.Fprintf(os.Stderr, "could not write stack traces: %v", err)
					return
				}
				defer file.Close()
				fmt.Fprintf(file, "test will timeout, printing goroutine stack traces:\n")
				pprof.Lookup("goroutine").WriteTo(file, 2)
			}
		}()
	}

	testCheckRightsStub()

	if runtime.GOOS == "windows" {
		binExtension = ".exe"
	}

	// We rely on Go for the test executable since it's the only common
	// tool we're sure to have on Windows, OSX and Linux.
	build(m, "./test/argument/argument", "./test/argument")
	build(m, "./test/error/error", "./test/error")
	build(m, "./test/input/input", "./test/input")
	build(m, "./test/response_too_long/response_too_long", "./test/response_too_long")
	build(m, "./test/simple/simple", "./test/simple")
	build(m, "./test/timeout/timeout", "./test/timeout")

	res := m.Run()

	os.Remove("test/argument/argument" + binExtension)
	os.Remove("test/error/error" + binExtension)
	os.Remove("test/input/input" + binExtension)
	os.Remove("test/response_too_long/response_too_long" + binExtension)
	os.Remove("test/simple/simple" + binExtension)
	os.Remove("test/timeout/timeout" + binExtension)

	os.Exit(res)
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

func TestExecCommandError(t *testing.T) {
	inputPayload := "{\"version\": \"" + secrets.PayloadVersion + "\" , \"secrets\": [\"sec1\", \"sec2\"]}"

	t.Run("Empty secretBackendCommand", func(t *testing.T) {
		resolver := newEnabledSecretResolver()
		_, err := resolver.execCommand(inputPayload)
		require.NotNil(t, err)
	})

	t.Run("timeout", func(t *testing.T) {
		resolver := newEnabledSecretResolver()
		resolver.backendCommand = "./test/timeout/timeout" + binExtension
		setCorrectRight(resolver.backendCommand)
		resolver.backendTimeout = 1
		_, err := resolver.execCommand(inputPayload)
		require.NotNil(t, err)
		require.Equal(t, "error while running './test/timeout/timeout"+binExtension+"': command timeout", err.Error())
	})

	t.Run("No Error", func(t *testing.T) {
		resolver := newEnabledSecretResolver()
		resolver.Configure("./test/simple/simple"+binExtension, nil, 0, 0, 0, false, false, "")
		setCorrectRight(resolver.backendCommand)
		resp, err := resolver.execCommand(inputPayload)
		require.NoError(t, err)
		require.Equal(t, []byte("{\"handle1\":{\"value\":\"simple_password\"}}"), resp)
	})

	t.Run("Error returned", func(t *testing.T) {
		resolver := newEnabledSecretResolver()
		resolver.backendCommand = "./test/error/error" + binExtension
		setCorrectRight(resolver.backendCommand)
		_, err := resolver.execCommand(inputPayload)
		require.NotNil(t, err)
	})

	t.Run("argument", func(t *testing.T) {
		resolver := newEnabledSecretResolver()
		resolver.Configure("./test/argument/argument"+binExtension, nil, 0, 0, 0, false, false, "")
		setCorrectRight(resolver.backendCommand)
		resolver.backendArguments = []string{"arg1"}
		_, err := resolver.execCommand(inputPayload)
		require.NotNil(t, err)
		resolver.backendArguments = []string{"arg1", "arg2"}
		resp, err := resolver.execCommand(inputPayload)
		require.NoError(t, err)
		require.Equal(t, []byte("{\"handle1\":{\"value\":\"arg_password\"}}"), resp)
	})

	t.Run("input", func(t *testing.T) {
		resolver := newEnabledSecretResolver()
		resolver.Configure("./test/input/input"+binExtension, nil, 0, 0, 0, false, false, "")
		setCorrectRight(resolver.backendCommand)
		resp, err := resolver.execCommand(inputPayload)
		require.NoError(t, err)
		require.Equal(t, []byte("{\"handle1\":{\"value\":\"input_password\"}}"), resp)
	})

	t.Run("buffer limit", func(t *testing.T) {
		resolver := newEnabledSecretResolver()
		resolver.Configure("./test/response_too_long/response_too_long"+binExtension, nil, 0, 0, 0, false, false, "")
		setCorrectRight(resolver.backendCommand)
		resolver.responseMaxSize = 20
		_, err := resolver.execCommand(inputPayload)
		require.NotNil(t, err)
		assert.Equal(t, "error while running './test/response_too_long/response_too_long"+binExtension+"': command output was too long: exceeded 20 bytes", err.Error())
	})
}

func TestFetchSecretExeceError(t *testing.T) {
	resolver := newEnabledSecretResolver()
	resolver.commandHookFunc = func(string) ([]byte, error) { return nil, fmt.Errorf("some error") }
	_, err := resolver.fetchSecret([]string{"handle1", "handle2"})
	assert.NotNil(t, err)
}

func TestFetchSecretUnmarshalError(t *testing.T) {
	resolver := newEnabledSecretResolver()
	resolver.commandHookFunc = func(string) ([]byte, error) { return []byte("{"), nil }
	_, err := resolver.fetchSecret([]string{"handle1", "handle2"})
	assert.NotNil(t, err)
}

func TestFetchSecretMissingSecret(t *testing.T) {
	secrets := []string{"handle1", "handle2"}
	resolver := newEnabledSecretResolver()
	resolver.commandHookFunc = func(string) ([]byte, error) { return []byte("{}"), nil }
	_, err := resolver.fetchSecret(secrets)
	assert.NotNil(t, err)
	assert.Equal(t, "secret handle 'handle1' was not resolved by the secret_backend_command", err.Error())
}

func TestFetchSecretErrorForHandle(t *testing.T) {
	resolver := newEnabledSecretResolver()
	resolver.commandHookFunc = func(string) ([]byte, error) {
		return []byte("{\"handle1\":{\"value\": null, \"error\": \"some error\"}}"), nil
	}
	_, err := resolver.fetchSecret([]string{"handle1"})
	assert.NotNil(t, err)
	assert.Equal(t, "an error occurred while resolving 'handle1': some error", err.Error())
}

func TestFetchSecretEmptyValue(t *testing.T) {
	resolver := newEnabledSecretResolver()
	resolver.commandHookFunc = func(string) ([]byte, error) {
		return []byte("{\"handle1\":{\"value\": null}}"), nil
	}
	_, err := resolver.fetchSecret([]string{"handle1"})
	assert.NotNil(t, err)
	assert.Equal(t, "resolved secret for 'handle1' is empty", err.Error())

	resolver.commandHookFunc = func(string) ([]byte, error) {
		return []byte("{\"handle1\":{\"value\": \"\"}}"), nil
	}
	_, err = resolver.fetchSecret([]string{"handle1"})
	assert.NotNil(t, err)
	assert.Equal(t, "resolved secret for 'handle1' is empty", err.Error())
}

func TestFetchSecret(t *testing.T) {
	secrets := []string{"handle1", "handle2"}
	resolver := newEnabledSecretResolver()
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
	resolver := newEnabledSecretResolver()
	resolver.commandHookFunc = func(string) ([]byte, error) {
		return []byte("{\"handle1\":{\"value\":\"some data\\r\\n\"}}"), nil
	}
	resolver.removeTrailingLinebreak = true
	secrets := []string{"handle1"}
	resp, err := resolver.fetchSecret(secrets)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"handle1": "some data"}, resp)
}
