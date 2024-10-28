// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"

	"gopkg.in/yaml.v2"
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
		yamlFile, err := os.ReadFile(fmt.Sprintf("../jmxfetch/fixtures/%s.yaml", c))
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
	j["configurations"] = c

	//json encode
	_, err := json.Marshal(GetJSONSerializableMap(j))
	assert.NoError(t, err)
}

func TestCopyDir(t *testing.T) {
	assert := assert.New(t)
	src := t.TempDir()
	dst := t.TempDir()

	files := map[string]string{
		"a/b/c/d.txt": "d.txt",
		"e/f/g/h.txt": "h.txt",
		"i/j/k.txt":   "k.txt",
	}

	for file, content := range files {
		p := filepath.Join(src, file)
		err := os.MkdirAll(filepath.Dir(p), os.ModePerm)
		assert.NoError(err)
		err = os.WriteFile(p, []byte(content), os.ModePerm)
		assert.NoError(err)
	}
	err := CopyDir(src, dst)
	assert.NoError(err)

	for file, content := range files {
		p := filepath.Join(dst, file)
		actual, err := os.ReadFile(p)
		assert.NoError(err)
		assert.Equal(string(actual), content)
	}
}
