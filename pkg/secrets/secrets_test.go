// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build secrets

package secrets

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v2"
)

var (
	testYamlHash = []byte(`
slice:
  - "1"
  - [test1, test2]
  - 123
hash:
  a: test3
  b: "2"
  c: 456
  slice:
    - test4
    - test5
`)

	testYamlHashUpdated = []byte(`hash:
  a: test3_verified
  b: 2_verified
  c: 456
  slice:
  - test4_verified
  - test5_verified
slice:
- 1_verified
- - test1_verified
  - test2_verified
- 123
`)

	testConf = []byte(`---
instances:
- password: ENC[pass1]
  user: test
- password: ENC[pass2]
  user: test2
`)

	testConfDecrypted = []byte(`instances:
- password: password1
  user: test
- password: password2
  user: test2
`)
)

func TestIsEnc(t *testing.T) {
	enc, secret := isEnc("")
	assert.False(t, enc)
	assert.Equal(t, "", secret)

	enc, secret = isEnc("ENC[]")
	assert.True(t, enc)
	assert.Equal(t, "", secret)

	enc, _ = isEnc("test")
	assert.False(t, enc)

	enc, _ = isEnc("ENC[")
	assert.False(t, enc)

	enc, secret = isEnc("ENC[test]")
	assert.True(t, enc)
	assert.Equal(t, "test", secret)

	enc, secret = isEnc("ENC[]]]]")
	assert.True(t, enc)
	assert.Equal(t, "]]]", secret)

	enc, secret = isEnc("  ENC[test]	")
	assert.True(t, enc)
	assert.Equal(t, "test", secret)
}

func TestWalkerError(t *testing.T) {
	var config interface{}
	err := yaml.Unmarshal(testYamlHash, &config)
	require.Nil(t, err)

	err = walk(&config, func(str string) (string, error) {
		return "", fmt.Errorf("some error")
	})
	assert.NotNil(t, err)
}

func TestWalkerSimple(t *testing.T) {
	var config interface{}
	err := yaml.Unmarshal([]byte("test"), &config)
	require.Nil(t, err)

	stringsCollected := []string{}
	err = walk(&config, func(str string) (string, error) {
		stringsCollected = append(stringsCollected, str)
		return str + "_verified", nil
	})
	require.Nil(t, err)

	assert.Equal(t, []string{"test"}, stringsCollected)

	updatedConf, err := yaml.Marshal(config)
	require.Nil(t, err)
	assert.Equal(t, string("test_verified\n"), string(updatedConf))
}

func TestWalkerComplex(t *testing.T) {
	var config interface{}
	err := yaml.Unmarshal(testYamlHash, &config)
	require.Nil(t, err)

	stringsCollected := []string{}
	err = walk(&config, func(str string) (string, error) {
		stringsCollected = append(stringsCollected, str)
		return str + "_verified", nil
	})
	require.Nil(t, err)

	sort.Strings(stringsCollected)
	assert.Equal(t, []string{
		"1",
		"2",
		"test1",
		"test2",
		"test3",
		"test4",
		"test5",
	}, stringsCollected)

	updatedConf, err := yaml.Marshal(config)
	require.Nil(t, err)
	assert.Equal(t, string(testYamlHashUpdated), string(updatedConf))
}

func TestDecryptNoCommand(t *testing.T) {
	secretFetcher = func(secrets []string) (map[string]string, error) {
		return nil, fmt.Errorf("some error")
	}

	// since we didn't set any command this should return without any error
	_, err := Decrypt(testConf)
	require.Nil(t, err)
}

func TestDecryptSecretError(t *testing.T) {
	secretBackendCommand = "some_command"
	defer func() { secretBackendCommand = "" }()

	secretFetcher = func(secrets []string) (map[string]string, error) {
		return nil, fmt.Errorf("some error")
	}

	_, err := Decrypt(testConf)
	require.NotNil(t, err)
}

func TestDecryptSecretNoCache(t *testing.T) {
	secretBackendCommand = "some_command"
	defer func() { secretBackendCommand = "" }()

	secretFetcher = func(secrets []string) (map[string]string, error) {
		sort.Strings(secrets)
		assert.Equal(t, []string{
			"pass1",
			"pass2",
		}, secrets)

		return map[string]string{
			"pass1": "password1",
			"pass2": "password2",
		}, nil
	}

	newConf, err := Decrypt(testConf)
	require.Nil(t, err)
	assert.Equal(t, string(testConfDecrypted), string(newConf))
}

func TestDecryptSecretPartialCache(t *testing.T) {
	secretBackendCommand = "some_command"
	defer func() { secretBackendCommand = "" }()

	secretCache["pass1"] = "password1"
	defer func() { secretCache = map[string]string{} }()

	secretFetcher = func(secrets []string) (map[string]string, error) {
		sort.Strings(secrets)
		assert.Equal(t, []string{
			"pass2",
		}, secrets)

		return map[string]string{
			"pass2": "password2",
		}, nil
	}

	newConf, err := Decrypt(testConf)
	require.Nil(t, err)
	assert.Equal(t, testConfDecrypted, newConf)
}

func TestDecryptSecretFullCache(t *testing.T) {
	secretBackendCommand = "some_command"
	defer func() { secretBackendCommand = "" }()

	secretCache["pass1"] = "password1"
	secretCache["pass2"] = "password2"
	defer func() { secretCache = map[string]string{} }()

	secretFetcher = func(secrets []string) (map[string]string, error) {
		require.Fail(t, "Secret Cache was not used properly")
		return nil, nil
	}

	newConf, err := Decrypt(testConf)
	require.Nil(t, err)
	assert.Equal(t, testConfDecrypted, newConf)
}
