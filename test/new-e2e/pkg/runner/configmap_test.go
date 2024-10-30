// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package runner

import (
	"encoding/json"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
)

func Test_BuildStackParameters(t *testing.T) {
	jsonStr, err := json.Marshal(map[string]string{
		"namespace:key/foo": "42",
	})
	require.NoError(t, err)
	configMap, err := BuildStackParameters(newMockProfile(map[parameters.StoreKey]string{
		parameters.StackParameters: string(jsonStr),
	}), ConfigMap{})

	require.NoError(t, err)
	require.NotEmpty(t, configMap)
	assert.Equal(t, ConfigMap{
		"ddagent:apiKey":                        auto.ConfigValue{Value: "api_key", Secret: true},
		"ddagent:appKey":                        auto.ConfigValue{Value: "app_key", Secret: true},
		"namespace:key/foo":                     auto.ConfigValue{Value: "42", Secret: false},
		"ddinfra:aws/defaultKeyPairName":        auto.ConfigValue{Value: "key_pair_name", Secret: false},
		"ddinfra:env":                           auto.ConfigValue{Value: "", Secret: false},
		"ddinfra:extraResourcesTags":            auto.ConfigValue{Value: "extra_resources_tags", Secret: false},
		"ddinfra:initOnly":                      auto.ConfigValue{Value: "init_only", Secret: false},
		"ddinfra:aws/defaultPublicKeyPath":      auto.ConfigValue{Value: "public_key_path", Secret: false},
		"ddinfra:aws/defaultPrivateKeyPath":     auto.ConfigValue{Value: "private_key_path", Secret: false},
		"ddinfra:aws/defaultPrivateKeyPassword": auto.ConfigValue{Value: "private_key_password", Secret: true},
		"ddinfra:az/defaultPublicKeyPath":       auto.ConfigValue{Value: "public_key_path", Secret: false},
		"ddinfra:az/defaultPrivateKeyPath":      auto.ConfigValue{Value: "private_key_path", Secret: false},
		"ddinfra:az/defaultPrivateKeyPassword":  auto.ConfigValue{Value: "private_key_password", Secret: true},
		"ddinfra:gcp/defaultPublicKeyPath":      auto.ConfigValue{Value: "public_key_path", Secret: false},
		"ddinfra:gcp/defaultPrivateKeyPath":     auto.ConfigValue{Value: "private_key_path", Secret: false},
		"ddinfra:gcp/defaultPrivateKeyPassword": auto.ConfigValue{Value: "private_key_password", Secret: true},
		"ddagent:pipeline_id":                   auto.ConfigValue{Value: "pipeline_id", Secret: false},
		"ddagent:commit_sha":                    auto.ConfigValue{Value: "commit_sha", Secret: false},
		"ddagent:majorVersion":                  auto.ConfigValue{Value: "major_version", Secret: false},
	}, configMap)
}
