// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

import (
	_ "embed"
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testfixtures/test_config_with_stackparams.yaml
var configWithStackparams []byte

func Test_parseConfigFileContent(t *testing.T) {
	store := configFileValueStore{}
	store.parseConfigFileContent(configWithStackparams)
	assert.Equal(t, "totoro", store.config.ConfigParams.AWS.KeyPairName)
	assert.Equal(t, "/Users/totoro/.ssh/id_rsa.pub", store.config.ConfigParams.AWS.PublicKeyPath)
	assert.Equal(t, "kiki", store.config.ConfigParams.AWS.Account)
	assert.Equal(t, "00000000000000000000000000000000", store.config.ConfigParams.Agent.APIKey)
	assert.Equal(t, "miyazaki", store.config.ConfigParams.AWS.TeamTag)

	stackParamsStr, err := store.get(StackParameters)
	require.NoError(t, err)
	require.NotEmpty(t, stackParamsStr)
	var stackParams map[string]string
	err = json.Unmarshal([]byte(stackParamsStr), &stackParams)
	require.NoError(t, err)
	expectedStackparams := map[string]string{
		"ddinfra:agent/foo": "42",
	}
	assert.Equal(t, expectedStackparams, stackParams)
}

func Test_NewConfigFileStore(t *testing.T) {
	dir, err := os.Getwd()
	require.NoError(t, err)
	configPath := path.Join(dir, "testfixtures/test_config_with_stackparams.yaml")

	store, err := NewConfigFileStore(configPath)
	require.NoError(t, err)

	value, err := store.Get(KeyPairName)
	assert.NoError(t, err)
	assert.Equal(t, "totoro", value)

	value, err = store.Get(PublicKeyPath)
	assert.NoError(t, err)
	assert.Equal(t, "/Users/totoro/.ssh/id_rsa.pub", value)

	value, err = store.Get(Environments)
	assert.NoError(t, err)
	assert.Equal(t, "aws/kiki", value)

	value, err = store.Get(APIKey)
	assert.NoError(t, err)
	assert.Equal(t, "00000000000000000000000000000000", value)

	value, err = store.Get(ExtraResourcesTags)
	assert.NoError(t, err)
	assert.Equal(t, "team:miyazaki", value)
}

func Test_NewConfigFileStoreNoAWSAccount(t *testing.T) {
	dir, err := os.Getwd()
	require.NoError(t, err)
	configPath := path.Join(dir, "testfixtures/test_config_no_aws_account.yaml")

	store, err := NewConfigFileStore(configPath)
	require.NoError(t, err)

	value, err := store.Get(KeyPairName)
	assert.NoError(t, err)
	assert.Equal(t, "totoro", value)

	value, err = store.Get(PublicKeyPath)
	assert.NoError(t, err)
	assert.Equal(t, "/Users/totoro/.ssh/id_rsa.pub", value)

	value, err = store.Get(Environments)
	assert.ErrorIs(t, err, ParameterNotFoundError{Environments})
	assert.Equal(t, "", value)

	value, err = store.Get(APIKey)
	assert.NoError(t, err)
	assert.Equal(t, "00000000000000000000000000000000", value)

	_, err = store.Get(APPKey)
	assert.Error(t, err)
}
