// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

package jmxfetch

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestJSONConverter(t *testing.T) {
	checks := []string{
		"cassandra",
		"kafka",
		"jmx",
		"jmx_alt",
	}

	cache := map[string]integration.RawMap{}
	for _, c := range checks {
		var cf integration.RawMap

		// Read file contents
		yamlFile, err := os.ReadFile(fmt.Sprintf("./fixtures/%s.yaml", c))
		assert.NoError(t, err)

		// Parse configuration
		err = yaml.Unmarshal(yamlFile, &cf)
		assert.NoError(t, err)

		cache[c] = cf
	}

	//convert
	j := map[string]interface{}{}
	c := map[string]interface{}{}
	for name, config := range cache {
		c[name] = GetJSONSerializableMap(config)
	}
	j["configs"] = c

	//json encode
	_, err := json.Marshal(GetJSONSerializableMap(j))
	assert.NoError(t, err)
}
