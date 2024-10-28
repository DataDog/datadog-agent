// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

package jsonquery

import (
	"testing"

	"github.com/itchyny/gojq"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

func TestJsonQueryParse(t *testing.T) {
	var code *gojq.Code
	var err error

	code, err = Parse(".spec.foo")
	assert.NotNil(t, code)
	assert.NoError(t, err)
	value, found := cache.Cache.Get("jq-" + ".spec.foo")
	assert.True(t, found)
	assert.Equal(t, code, value)

	code, err = Parse(".$spec.foo")
	assert.Nil(t, code)
	assert.Error(t, err)
}

func TestQueryRun(t *testing.T) {
	object := map[string]interface{}{
		"foo": "bar",
		"baz": []interface{}{"toto", "titi"},
	}

	value, hasValue, err := RunSingleOutput(".foo", object)
	assert.Equal(t, "bar", value)
	assert.True(t, hasValue)
	assert.NoError(t, err)

	value, hasValue, err = RunSingleOutput(".bar", object)
	assert.Equal(t, "", value)
	assert.False(t, hasValue)
	assert.NoError(t, err)

	value, hasValue, err = RunSingleOutput(".%bar", object)
	assert.Equal(t, "", value)
	assert.False(t, hasValue)
	assert.Error(t, err)
}

var yamlTest = `
apiVersion: kubelet.config.k8s.io/v1beta1
authentication:
  anonymous:
    enabled: false
  webhook:
    cacheTTL: 0s
    enabled: foobar
  x509:
    clientCAFile: /etc/kubernetes/pki/ca.crt
authorization:
  mode: Webhook
  webhook:
    cacheAuthorizedTTL: 0s
    cacheUnauthorizedTTL: 0s
`

func TestYAML(t *testing.T) {
	var yamlContent interface{}
	err := yaml.Unmarshal([]byte(yamlTest), &yamlContent)
	assert.NoError(t, err)
	yamlContent = NormalizeYAMLForGoJQ(yamlContent)

	value, _, err := RunSingleOutput(".authentication.anonymous.enabled", yamlContent)
	assert.NoError(t, err)
	assert.Equal(t, "false", value)
}
