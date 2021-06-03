// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

package jsonquery

import (
	"fmt"
	"time"
)

// Copy from https://github.com/itchyny/gojq/blob/main/cli/yaml.go
// Copyright (c) 2019-2021 itchyny
// Workaround for https://github.com/go-yaml/yaml/issues/139

// NormalizeYAMLForGoJQ normalizes output from YAML parsing to be gojq compatible
func NormalizeYAMLForGoJQ(v interface{}) interface{} {
	switch v := v.(type) {
	case map[interface{}]interface{}:
		w := make(map[string]interface{}, len(v))
		for k, v := range v {
			w[fmt.Sprint(k)] = NormalizeYAMLForGoJQ(v)
		}
		return w

	case map[string]interface{}:
		w := make(map[string]interface{}, len(v))
		for k, v := range v {
			w[k] = NormalizeYAMLForGoJQ(v)
		}
		return w

	case []interface{}:
		for i, w := range v {
			v[i] = NormalizeYAMLForGoJQ(w)
		}
		return v

	// go-yaml unmarshals timestamp string to time.Time but gojq cannot handle it.
	// It is impossible to keep the original timestamp strings.
	case time.Time:
		return v.Format(time.RFC3339)

	default:
		return v
	}
}
