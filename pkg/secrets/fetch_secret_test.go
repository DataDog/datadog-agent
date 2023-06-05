// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets

package secrets

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	binExtension = ""
)

func build(m *testing.M, outBin, pkg string) {
	_, err := exec.Command("go", "build", "-o", outBin+binExtension, pkg).Output()
	if err != nil {
		fmt.Printf("Could not compile test secretBackendCommand: %s", err)
		os.Exit(1)
	}
}

func TestMain(m *testing.M) {
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
	t.Cleanup(resetPackageVars)

	inputPayload := "{\"version\": \"" + PayloadVersion + "\" , \"secrets\": [\"sec1\", \"sec2\"]}"

	// empty secretBackendCommand
	secretBackendCommand = ""
	_, err := execCommand(inputPayload)
	require.NotNil(t, err)

	// test timeout
	secretBackendCommand = "./test/timeout/timeout" + binExtension
	setCorrectRight(secretBackendCommand)
	secretBackendTimeout = 2
	_, err = execCommand(inputPayload)
	require.NotNil(t, err)
	require.Equal(t, "error while running './test/timeout/timeout"+binExtension+"': command timeout", err.Error())

	// test simple (no error)
	secretBackendCommand = "./test/simple/simple" + binExtension
	setCorrectRight(secretBackendCommand)
	resp, err := execCommand(inputPayload)
	require.NoError(t, err)
	require.Equal(t, []byte("{\"handle1\":{\"value\":\"simple_password\"}}"), resp)

	// test error
	secretBackendCommand = "./test/error/error" + binExtension
	setCorrectRight(secretBackendCommand)
	_, err = execCommand(inputPayload)
	require.NotNil(t, err)

	// test arguments
	secretBackendCommand = "./test/argument/argument" + binExtension
	setCorrectRight(secretBackendCommand)
	secretBackendArguments = []string{"arg1"}
	_, err = execCommand(inputPayload)
	require.NotNil(t, err)
	secretBackendArguments = []string{"arg1", "arg2"}
	resp, err = execCommand(inputPayload)
	require.NoError(t, err)
	require.Equal(t, []byte("{\"handle1\":{\"value\":\"arg_password\"}}"), resp)

	// test input
	secretBackendCommand = "./test/input/input" + binExtension
	setCorrectRight(secretBackendCommand)
	resp, err = execCommand(inputPayload)
	require.NoError(t, err)
	require.Equal(t, []byte("{\"handle1\":{\"value\":\"input_password\"}}"), resp)

	// test buffer limit
	secretBackendCommand = "./test/response_too_long/response_too_long" + binExtension
	setCorrectRight(secretBackendCommand)
	SecretBackendOutputMaxSize = 20
	_, err = execCommand(inputPayload)
	require.NotNil(t, err)
	assert.Equal(t, "error while running './test/response_too_long/response_too_long"+binExtension+"': command output was too long: exceeded 20 bytes", err.Error())
}

func TestFetchSecretExecError(t *testing.T) {
	t.Cleanup(resetPackageVars)

	runCommand = func(string) ([]byte, error) { return nil, fmt.Errorf("some error") }
	_, err := fetchSecret([]string{"handle1", "handle2"})
	assert.NotNil(t, err)
}

func TestFetchSecretUnmarshalError(t *testing.T) {
	t.Cleanup(resetPackageVars)

	runCommand = func(string) ([]byte, error) { return []byte("{"), nil }
	_, err := fetchSecret([]string{"handle1", "handle2"})
	assert.NotNil(t, err)
}

func TestFetchSecretMissingSecret(t *testing.T) {
	t.Cleanup(resetPackageVars)

	secrets := []string{"handle1", "handle2"}

	runCommand = func(string) ([]byte, error) { return []byte("{}"), nil }
	_, err := fetchSecret(secrets)
	assert.NotNil(t, err)
	assert.Equal(t, "secret handle 'handle1' was not decrypted by the secret_backend_command", err.Error())
}

func TestFetchSecretErrorForHandle(t *testing.T) {
	t.Cleanup(resetPackageVars)

	runCommand = func(string) ([]byte, error) {
		return []byte("{\"handle1\":{\"value\": null, \"error\": \"some error\"}}"), nil
	}
	_, err := fetchSecret([]string{"handle1"})
	assert.NotNil(t, err)
	assert.Equal(t, "an error occurred while decrypting 'handle1': some error", err.Error())
}

func TestFetchSecretEmptyValue(t *testing.T) {
	t.Cleanup(resetPackageVars)

	runCommand = func(string) ([]byte, error) {
		return []byte("{\"handle1\":{\"value\": null}}"), nil
	}
	_, err := fetchSecret([]string{"handle1"})
	assert.NotNil(t, err)
	assert.Equal(t, "decrypted secret for 'handle1' is empty", err.Error())

	runCommand = func(string) ([]byte, error) {
		return []byte("{\"handle1\":{\"value\": \"\"}}"), nil
	}
	_, err = fetchSecret([]string{"handle1"})
	assert.NotNil(t, err)
	assert.Equal(t, "decrypted secret for 'handle1' is empty", err.Error())
}

func TestFetchSecret(t *testing.T) {
	t.Cleanup(resetPackageVars)

	secrets := []string{"handle1", "handle2"}
	// some dummy value to check the cache is not purge
	secretCache["test"] = "yes"

	runCommand = func(string) ([]byte, error) {
		res := []byte("{\"handle1\":{\"value\":\"p1\"},")
		res = append(res, []byte("\"handle2\":{\"value\":\"p2\"},")...)
		res = append(res, []byte("\"handle3\":{\"value\":\"p3\"}}")...)
		return res, nil
	}
	resp, err := fetchSecret(secrets)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"handle1": "p1",
		"handle2": "p2",
	}, resp)
	assert.Equal(t, map[string]string{
		"test":    "yes",
		"handle1": "p1",
		"handle2": "p2",
	}, secretCache)
}

func TestFetchSecretRemoveTrailingLineBreak(t *testing.T) {
	t.Cleanup(resetPackageVars)
	removeTrailingLinebreak = true

	runCommand = func(string) ([]byte, error) {
		return []byte("{\"handle1\":{\"value\":\"some data\\r\\n\"}}"), nil
	}
	secrets := []string{"handle1"}
	resp, err := fetchSecret(secrets)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"handle1": "some data"}, resp)
}
