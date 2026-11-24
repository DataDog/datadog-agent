// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/assert/yaml"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestGetRawSchema(t *testing.T) {
	for _, id := range []model.SchemaID{model.CoreSchema, model.SystemProbeSchema} {
		t.Run(string(id), func(t *testing.T) {
			data, err := GetRawSchema(id)
			if err == nil {
				assert.NoError(t, yaml.Unmarshal(data, new(interface{})))
				return
			}
			// Schema files are compressed by `dda inv schema.compress` and not embedded in the test build;
			// we verify the function returns a proper error rather than panicking.
			assert.ErrorContains(t, err, "no embedded schema")
		})
	}
}

func TestGetRawSchemaUnknownID(t *testing.T) {
	_, err := GetRawSchema("unknown")
	assert.ErrorContains(t, err, "unknown schema ID 'unknown'")
}
