// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package workflowjsonschema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateParametersPropertiesAndRequired(t *testing.T) {
	schema := map[string]interface{}{
		"properties": map[string]interface{}{
			"name": map[string]interface{}{"type": "string", "pattern": "^[a-z]+$"},
		},
		"required": []interface{}{"name"},
	}

	assert.NoError(t, ValidateParameters(schema, map[string]interface{}{"name": "abc"}))
	assert.Error(t, ValidateParameters(schema, map[string]interface{}{}))
	assert.Error(t, ValidateParameters(schema, map[string]interface{}{"name": "ABC"}))
}

func TestValidateParametersEnforcesRootKeywords(t *testing.T) {
	str := map[string]interface{}{"type": "string"}
	tests := []struct {
		name    string
		schema  map[string]interface{}
		valid   map[string]interface{}
		invalid map[string]interface{}
	}{
		{
			name: "additionalProperties",
			schema: map[string]interface{}{
				"properties":           map[string]interface{}{"a": str},
				"additionalProperties": false,
			},
			valid:   map[string]interface{}{"a": "x"},
			invalid: map[string]interface{}{"a": "x", "injected": "y"},
		},
		{
			name: "maxProperties",
			schema: map[string]interface{}{
				"properties":    map[string]interface{}{"a": str, "b": str},
				"maxProperties": 1,
			},
			valid:   map[string]interface{}{"a": "x"},
			invalid: map[string]interface{}{"a": "x", "b": "y"},
		},
		{
			name: "oneOf",
			schema: map[string]interface{}{
				"properties": map[string]interface{}{"a": str, "b": str},
				"oneOf": []interface{}{
					map[string]interface{}{"required": []interface{}{"a"}},
					map[string]interface{}{"required": []interface{}{"b"}},
				},
			},
			valid:   map[string]interface{}{"a": "x"},
			invalid: map[string]interface{}{"a": "x", "b": "y"},
		},
		{
			name: "allOf",
			schema: map[string]interface{}{
				"properties": map[string]interface{}{"a": str},
				"allOf": []interface{}{
					map[string]interface{}{"properties": map[string]interface{}{"a": map[string]interface{}{"maxLength": 3}}},
				},
			},
			valid:   map[string]interface{}{"a": "abc"},
			invalid: map[string]interface{}{"a": "abcd"},
		},
		{
			name: "dependentRequired",
			schema: map[string]interface{}{
				"properties":        map[string]interface{}{"a": str, "b": str},
				"dependentRequired": map[string]interface{}{"a": []interface{}{"b"}},
			},
			valid:   map[string]interface{}{"a": "x", "b": "y"},
			invalid: map[string]interface{}{"a": "x"},
		},
		{
			name: "propertyNames",
			schema: map[string]interface{}{
				"propertyNames": map[string]interface{}{"pattern": "^[a-z]+$"},
			},
			valid:   map[string]interface{}{"name": "x"},
			invalid: map[string]interface{}{"Name": "x"},
		},
		{
			name: "not",
			schema: map[string]interface{}{
				"properties": map[string]interface{}{"a": str},
				"not":        map[string]interface{}{"required": []interface{}{"a"}},
			},
			valid:   map[string]interface{}{},
			invalid: map[string]interface{}{"a": "x"},
		},
		{
			name: "patternProperties",
			schema: map[string]interface{}{
				"patternProperties": map[string]interface{}{"^n_": map[string]interface{}{"type": "integer"}},
			},
			valid:   map[string]interface{}{"n_count": 1},
			invalid: map[string]interface{}{"n_count": "many"},
		},
		{
			name: "if/then",
			schema: map[string]interface{}{
				"properties": map[string]interface{}{"mode": str, "target": str},
				"if": map[string]interface{}{
					"properties": map[string]interface{}{"mode": map[string]interface{}{"const": "safe"}},
				},
				"then": map[string]interface{}{
					"properties": map[string]interface{}{"target": map[string]interface{}{"enum": []interface{}{"tmp"}}},
				},
			},
			valid:   map[string]interface{}{"mode": "safe", "target": "tmp"},
			invalid: map[string]interface{}{"mode": "safe", "target": "/etc"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NoError(t, ValidateParameters(tt.schema, tt.valid))
			assert.Error(t, ValidateParameters(tt.schema, tt.invalid))
		})
	}
}

func TestValidateParametersForcesObjectType(t *testing.T) {
	schema := map[string]interface{}{"type": "string"}
	assert.Error(t, ValidateParameters(schema, "not-an-object"))
	assert.NoError(t, ValidateParameters(schema, map[string]interface{}{}))
}

func TestValidateParametersDoesNotLoadFileRef(t *testing.T) {
	refPath := filepath.Join(t.TempDir(), "name.json")
	require.NoError(t, os.WriteFile(refPath, []byte(`{"type": "string"}`), 0o600))
	schema := map[string]interface{}{
		"properties": map[string]interface{}{
			"a": map[string]interface{}{"$ref": "file://" + filepath.ToSlash(refPath)},
		},
	}
	err := ValidateParameters(schema, map[string]interface{}{"a": "x"})
	var loadErr *jsonschema.LoadURLError
	require.ErrorAs(t, err, &loadErr)
}

func TestValidateParametersLocalRef(t *testing.T) {
	schema := map[string]interface{}{
		"$defs":      map[string]interface{}{"name": map[string]interface{}{"type": "string"}},
		"properties": map[string]interface{}{"a": map[string]interface{}{"$ref": "#/$defs/name"}},
	}
	assert.NoError(t, ValidateParameters(schema, map[string]interface{}{"a": "x"}))
	assert.Error(t, ValidateParameters(schema, map[string]interface{}{"a": 1}))
}

func TestValidateParametersAppliesDefaultDraftForAnyDeclaredDraft(t *testing.T) {
	for _, draft := range []string{
		"http://json-schema.org/draft-04/schema#",
		"http://json-schema.org/draft-07/schema#",
		"https://json-schema.org/draft/2020-12/schema",
		"https://example.com/custom-meta-schema",
	} {
		schema := map[string]interface{}{
			"$schema":              draft,
			"properties":           map[string]interface{}{"a": map[string]interface{}{"type": "string"}},
			"additionalProperties": false,
		}
		assert.NoError(t, ValidateParameters(schema, map[string]interface{}{"a": "x"}), draft)
		assert.Error(t, ValidateParameters(schema, map[string]interface{}{"a": "x", "b": "y"}), draft)
	}
}

func TestValidateParametersDependencies(t *testing.T) {
	schema := map[string]interface{}{
		"$schema":    "http://json-schema.org/draft-07/schema#",
		"properties": map[string]interface{}{"a": map[string]interface{}{"type": "string"}, "b": map[string]interface{}{"type": "string"}},
		"allOf":      []interface{}{map[string]interface{}{"maxProperties": 2}},
		"dependencies": map[string]interface{}{
			"a": []interface{}{"b"},
			"b": map[string]interface{}{"properties": map[string]interface{}{"a": map[string]interface{}{"const": "x"}}},
		},
	}
	assert.NoError(t, ValidateParameters(schema, map[string]interface{}{"a": "x", "b": "y"}))
	assert.Error(t, ValidateParameters(schema, map[string]interface{}{"a": "x"}))
	assert.Error(t, ValidateParameters(schema, map[string]interface{}{"a": "z", "b": "y"}))
	assert.Contains(t, schema, "dependencies")
}

func TestValidateParametersRootErrorMessage(t *testing.T) {
	schema := map[string]interface{}{
		"properties":           map[string]interface{}{"a": map[string]interface{}{"type": "string"}},
		"additionalProperties": false,
	}
	err := ValidateParameters(schema, map[string]interface{}{"a": "x", "extra": "y"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extra")
}

func TestValidateParametersKeepsPropertiesBesideRootRef(t *testing.T) {
	schema := map[string]interface{}{
		"$schema":              "http://json-schema.org/draft-07/schema#",
		"definitions":          map[string]interface{}{"any": map[string]interface{}{}},
		"$ref":                 "#/definitions/any",
		"properties":           map[string]interface{}{"a": map[string]interface{}{"type": "string"}},
		"required":             []interface{}{"a"},
		"additionalProperties": false,
	}
	assert.NoError(t, ValidateParameters(schema, map[string]interface{}{"a": "x"}))
	assert.Error(t, ValidateParameters(schema, map[string]interface{}{"a": "x", "z": 1}))
	assert.Error(t, ValidateParameters(schema, map[string]interface{}{}))
	assert.Error(t, ValidateParameters(schema, map[string]interface{}{"a": 1}))
	assert.Error(t, ValidateParameters(schema, "not-an-object"))
}

func TestValidateParametersPropertyErrorMessage(t *testing.T) {
	schema := map[string]interface{}{
		"properties": map[string]interface{}{"a": map[string]interface{}{"type": "string"}},
	}
	err := ValidateParameters(schema, map[string]interface{}{"a": 1})
	require.Error(t, err)
	assert.Equal(t, "parameter validation failed: .a: got number, want string", err.Error())
}

func TestValidateParametersWithRootID(t *testing.T) {
	schema := map[string]interface{}{
		"$id":                  "https://example.com/schemas/params.json",
		"$defs":                map[string]interface{}{"name": map[string]interface{}{"type": "string"}},
		"properties":           map[string]interface{}{"a": map[string]interface{}{"$ref": "#/$defs/name"}},
		"additionalProperties": false,
	}
	assert.NoError(t, ValidateParameters(schema, map[string]interface{}{"a": "x"}))
	assert.Error(t, ValidateParameters(schema, map[string]interface{}{"a": 1}))
	assert.Error(t, ValidateParameters(schema, map[string]interface{}{"a": "x", "b": "y"}))
}

func TestValidateParametersKeepsKeywordsBesidePropertyRef(t *testing.T) {
	schema := map[string]interface{}{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"definitions": map[string]interface{}{"text": map[string]interface{}{"type": "string"}},
		"properties": map[string]interface{}{
			"a": map[string]interface{}{"$ref": "#/definitions/text", "pattern": "^[a-z]+$"},
		},
	}
	assert.NoError(t, ValidateParameters(schema, map[string]interface{}{"a": "abc"}))
	assert.Error(t, ValidateParameters(schema, map[string]interface{}{"a": "ABC"}))
}

func TestValidateParametersNilSchema(t *testing.T) {
	assert.NoError(t, ValidateParameters(nil, map[string]interface{}{"a": "x"}))
	assert.Error(t, ValidateParameters(nil, "not-an-object"))
}
