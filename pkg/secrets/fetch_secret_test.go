// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !windows

package secrets

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/common"
)

func TestLimitBuffer(t *testing.T) {
	lb := limitBuffer{
		buf: &bytes.Buffer{},
		max: 5,
	}

	n, err := lb.Write([]byte("012"))
	assert.Equal(t, 3, n)
	require.Nil(t, err)
	assert.Equal(t, []byte("012"), lb.buf.Bytes())

	n, err = lb.Write([]byte("abc"))
	assert.Equal(t, 0, n)
	require.NotNil(t, err)
	assert.Equal(t, []byte("012"), lb.buf.Bytes())

	n, err = lb.Write([]byte("ab"))
	assert.Equal(t, 2, n)
	require.Nil(t, err)
	assert.Equal(t, []byte("012ab"), lb.buf.Bytes())
}

func TestExecCommandError(t *testing.T) {
	defer func() {
		secretBackendCommand = ""
		secretBackendArguments = []string{}
		secretBackendTimeout = 0
	}()

	inputPayload := "{\"version\": \"" + payloadVersion + "\" , \"secrets\": [\"sec1\", \"sec2\"]}"

	// empty secretBackendCommand
	secretBackendCommand = ""
	_, err := execCommand(inputPayload)
	require.NotNil(t, err)

	// test timeout
	os.Chmod("./test/timeout.sh", 0700)
	secretBackendCommand = "./test/timeout.sh"
	secretBackendTimeout = 2
	_, err = execCommand(inputPayload)
	require.NotNil(t, err)
	require.Equal(t, "error while running './test/timeout.sh': command timeout", err.Error())

	// test simple (no error)
	os.Chmod("./test/simple.sh", 0700)
	secretBackendCommand = "./test/simple.sh"
	resp, err := execCommand(inputPayload)
	require.Nil(t, err)
	require.Equal(t, []byte("{\"handle1\":{\"value\":\"simple_password\"}}"), resp)

	// test error
	secretBackendCommand = "./test/error.sh"
	_, err = execCommand(inputPayload)
	require.NotNil(t, err)

	// test arguments
	os.Chmod("./test/argument.sh", 0700)
	secretBackendCommand = "./test/argument.sh"
	secretBackendArguments = []string{"arg1"}
	_, err = execCommand(inputPayload)
	require.NotNil(t, err)
	secretBackendCommand = "./test/argument.sh"
	secretBackendArguments = []string{"arg1", "arg2"}
	resp, err = execCommand(inputPayload)
	require.Nil(t, err)
	require.Equal(t, []byte("{\"handle1\":{\"value\":\"arg_password\"}}"), resp)

	// test input
	os.Chmod("./test/input.sh", 0700)
	secretBackendCommand = "./test/input.sh"
	resp, err = execCommand(inputPayload)
	require.Nil(t, err)
	require.Equal(t, []byte("{\"handle1\":{\"value\":\"input_password\"}}"), resp)

	// test buffer limit
	os.Chmod("./test/response_too_long.sh", 0700)
	secretBackendCommand = "./test/response_too_long.sh"
	secretBackendOutputMaxSize = 20
	_, err = execCommand(inputPayload)
	require.NotNil(t, err)
	assert.Equal(t, "error while running './test/response_too_long.sh': command output was too long: exceeded 20 bytes", err.Error())
}

func TestFetchSecretExecError(t *testing.T) {
	defer func() {
		secretCache = map[string]string{}
		secretOrigin = map[string]common.StringSet{}
	}()

	runCommand = func(string) ([]byte, error) { return nil, fmt.Errorf("some error") }
	_, err := fetchSecret([]string{"handle1", "handle2"}, "test")
	assert.NotNil(t, err)
}

func TestFetchSecretUnmarshalError(t *testing.T) {
	defer func() {
		secretCache = map[string]string{}
		secretOrigin = map[string]common.StringSet{}
	}()

	runCommand = func(string) ([]byte, error) { return []byte("{"), nil }
	_, err := fetchSecret([]string{"handle1", "handle2"}, "test")
	assert.NotNil(t, err)
}

func TestFetchSecretMissingSecret(t *testing.T) {
	defer func() {
		secretCache = map[string]string{}
		secretOrigin = map[string]common.StringSet{}
	}()

	secrets := []string{"handle1", "handle2"}

	runCommand = func(string) ([]byte, error) { return []byte("{}"), nil }
	_, err := fetchSecret(secrets, "test")
	assert.NotNil(t, err)
	assert.Equal(t, "secret handle 'handle1' was not decrypted by the secret_backend_command", err.Error())
}

func TestFetchSecretErrorForHandle(t *testing.T) {
	defer func() {
		secretCache = map[string]string{}
		secretOrigin = map[string]common.StringSet{}
	}()

	runCommand = func(string) ([]byte, error) {
		return []byte("{\"handle1\":{\"value\": null, \"error\": \"some error\"}}"), nil
	}
	_, err := fetchSecret([]string{"handle1"}, "test")
	assert.NotNil(t, err)
	assert.Equal(t, "an error occurred while decrypting 'handle1': some error", err.Error())
}

func TestFetchSecretEmptyValue(t *testing.T) {
	defer func() {
		secretCache = map[string]string{}
		secretOrigin = map[string]common.StringSet{}
	}()

	runCommand = func(string) ([]byte, error) {
		return []byte("{\"handle1\":{\"value\": null}}"), nil
	}
	_, err := fetchSecret([]string{"handle1"}, "test")
	assert.NotNil(t, err)
	assert.Equal(t, "decrypted secret for 'handle1' is empty", err.Error())

	runCommand = func(string) ([]byte, error) {
		return []byte("{\"handle1\":{\"value\": \"\"}}"), nil
	}
	_, err = fetchSecret([]string{"handle1"}, "test")
	assert.NotNil(t, err)
	assert.Equal(t, "decrypted secret for 'handle1' is empty", err.Error())
}

func TestFetchSecret(t *testing.T) {
	defer func() {
		secretCache = map[string]string{}
		secretOrigin = map[string]common.StringSet{}
	}()

	secrets := []string{"handle1", "handle2"}
	// some dummy value to check the cache is not purge
	secretCache["test"] = "yes"

	runCommand = func(string) ([]byte, error) {
		res := []byte("{\"handle1\":{\"value\":\"p1\"},")
		res = append(res, []byte("\"handle2\":{\"value\":\"p2\"},")...)
		res = append(res, []byte("\"handle3\":{\"value\":\"p3\"}}")...)
		return res, nil
	}
	resp, err := fetchSecret(secrets, "test")
	require.Nil(t, err)
	assert.Equal(t, map[string]string{
		"handle1": "p1",
		"handle2": "p2",
	}, resp)
	assert.Equal(t, map[string]string{
		"test":    "yes",
		"handle1": "p1",
		"handle2": "p2",
	}, secretCache)
	assert.Equal(t, map[string]common.StringSet{"handle1": common.NewStringSet("test"), "handle2": common.NewStringSet("test")}, secretOrigin)
}
