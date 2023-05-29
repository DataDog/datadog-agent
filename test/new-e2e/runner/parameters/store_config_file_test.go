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

func Test_parseConfigFileContent(t *testing.T) {
	store := configFileValueStore{}
	store.parseConfigFileContent(config_with_stackparams)
	assert.Equal(t, "totoro", store.config.ConfigParams.AWS.KeyPairName)
	assert.Equal(t, "/Users/totoro/.ssh/id_rsa.pub", store.config.ConfigParams.AWS.PublicKeyPath)
	assert.Equal(t, "00000000000000000000000000000000", store.config.ConfigParams.Agent.APIKey)

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
