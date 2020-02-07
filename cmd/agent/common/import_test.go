// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

func TestBasicMinCollectionIntervalRelocation(t *testing.T) {
	input := `
init_config:
   min_collection_interval: 30
instances: [{}, {}]`
	output := `
init_config: {}
instances:
  - min_collection_interval: 30
  - min_collection_interval: 30`
	assertRelocation(t, input, output)
}

func TestEmptyInstances(t *testing.T) {
	input := `
init_config:
   min_collection_interval: 30
instances:`
	output := `
init_config: {}
instances:`
	assertRelocation(t, input, output)
}

func TestEmptyYaml(t *testing.T) {
	assertRelocation(t, "", "")
}
func TestUntouchedYaml(t *testing.T) {
	input := `
instances:
  - host: localhost
    port: 7199
    cassandra_aliasing: true
init_config:
  is_jmx: true
  collect_default_metrics: true`
	assertRelocation(t, input, input)
}

func assertRelocation(t *testing.T, input, expectedOutput string) {
	output, _ := relocateMinCollectionInterval([]byte(input))
	expectedYamlOuput := make(map[interface{}]interface{})
	yamlOutput := make(map[interface{}]interface{})
	yaml.Unmarshal(output, &yamlOutput)
	yaml.Unmarshal([]byte(expectedOutput), &expectedYamlOuput)
	assert.Equal(t, yamlOutput, expectedYamlOuput)
}
