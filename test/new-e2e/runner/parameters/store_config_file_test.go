// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures/config_with_stackparams.yaml
var config_with_stackparams []byte

//go:embed fixtures/config_no_aws_account.yaml
var config_no_aws_account []byte

func Test_parseConfigFileContent(t *testing.T) {
	store := configFileValueStore{}
	store.parseConfigFileContent(config_with_stackparams)
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

func Test_parseConfigFileStoreContent(t *testing.T) {
	valueStore := configFileValueStore{}
	valueStore.parseConfigFileContent(config_with_stackparams)
	store := NewCascadingStore(valueStore)

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

func Test_parseConfigFileStoreContentNoAWSAccount(t *testing.T) {
	valueStore := configFileValueStore{}
	valueStore.parseConfigFileContent(config_no_aws_account)
	store := NewCascadingStore(valueStore)

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
}
