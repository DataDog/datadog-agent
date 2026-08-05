// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

package jsonquery

import (
	"bytes"
	"fmt"
	"time"

	"go.yaml.in/yaml/v3"
)

// Copy from https://github.com/itchyny/gojq/blob/main/cli/yaml.go
// Copyright (c) 2019-2021 itchyny
// Workaround for https://github.com/go-yaml/yaml/issues/139

// NormalizeYAMLForJQ normalizes output from YAML parsing so that it can be encoded as
// the JSON a jq query runs against.
func NormalizeYAMLForJQ(v interface{}) interface{} {
	switch v := v.(type) {
	case map[interface{}]interface{}:
		w := make(map[string]interface{}, len(v))
		for k, v := range v {
			w[fmt.Sprint(k)] = NormalizeYAMLForJQ(v)
		}
		return w

	case map[string]interface{}:
		w := make(map[string]interface{}, len(v))
		for k, v := range v {
			w[k] = NormalizeYAMLForJQ(v)
		}
		return w

	case []interface{}:
		for i, w := range v {
			v[i] = NormalizeYAMLForJQ(w)
		}
		return v

	// go-yaml unmarshals timestamp strings to time.Time, which has no JSON scalar
	// equivalent. It is impossible to keep the original timestamp strings.
	case time.Time:
		return v.Format(time.RFC3339)

	default:
		return v
	}
}

// YAMLCheckExist check a property/value from a YAML exist (jq style syntax)
func YAMLCheckExist(yamlData []byte, query string) (bool, error) {
	var yamlContent interface{}
	if err := yaml.Unmarshal(yamlData, &yamlContent); err != nil {
		return false, err
	}

	program, err := Parse(query)
	if err != nil {
		return false, err
	}

	jsonData, err := marshalJSON(NormalizeYAMLForJQ(yamlContent))
	if err != nil {
		return false, err
	}

	output, found, err := runFirstOutput(program, jsonData)
	if err != nil {
		return false, err
	}
	if found {
		switch {
		case bytes.Equal(output, []byte("true")):
			return true, nil
		case bytes.Equal(output, []byte("false")):
			return false, nil
		}
	}
	return false, fmt.Errorf("filter query must return a boolean, got %q", output)
}
