// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package schema

import (
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/assert/yaml"
	"github.com/stretchr/testify/require"
)

var (
	testYAMLSchema = `
$defs:
  parent:
    type: object
    properties:
      optional:
        type: boolean
      "required/key~name":
        type: integer
    required:
      - "required/key~name"
properties:
  api_key:
    type: string
    default: ""
  tags:
    type: array
    items:
      type: string
    default: []
  parent:
    $ref: "#/$defs/parent"
`
	testSchema        *jsonschema.Schema
	schemaCompileOnce sync.Once
)

func initTestSchema(t *testing.T) {
	schemaCompileOnce.Do(func() {
		c := jsonschema.NewCompiler()

		var loadSchema interface{}
		err := yaml.Unmarshal([]byte(testYAMLSchema), &loadSchema)
		if err != nil {
			assert.Failf(t, "could not unmarshal schema: '%s", err.Error())
		}

		if err := c.AddResource("test_schema", loadSchema); err != nil {
			assert.Failf(t, "could not add schema resource: %s", err.Error())
		}
		testSchema, err = c.Compile("test_schema")
		if err != nil {
			assert.Failf(t, "could not add compile schema: %s", err.Error())
		}
	})

	t.Cleanup(func() {
		coreSchemaGetter = getCoreSchema
		sysprobeSchemaGetter = getSysprobeSchema
	})

	coreSchemaGetter = func() (*jsonschema.Schema, error) { return testSchema, nil }
	sysprobeSchemaGetter = func() (*jsonschema.Schema, error) { return testSchema, nil }
}

func TestGetCoreSchema(t *testing.T) {
	data, err := GetCoreSchema()
	if err == nil {
		assert.NoError(t, yaml.Unmarshal(data, new(interface{})))
		return
	}
	// Schema files are compressed by `dda inv schema.compress` and not embedded in the test build;
	// we verify the function returns a proper error rather than panicking.
	assert.ErrorContains(t, err, "no embedded schema")
}

func TestGetSystemProbeSchema(t *testing.T) {
	data, err := GetSystemProbeSchema()
	if err == nil {
		assert.NoError(t, yaml.Unmarshal(data, new(interface{})))
		return
	}
	// Same as above: no compressed schema in test build.
	assert.ErrorContains(t, err, "no embedded schema")
}

func TestValidateCoreFileEmpty(t *testing.T) {
	initTestSchema(t)
	errs, err := ValidateCoreConfig(map[string]interface{}{})
	assert.NoError(t, err)
	assert.Empty(t, errs, "empty config should be valid against placeholder schema")

	errs, err = ValidateSystemProbeConfig(map[string]interface{}{})
	assert.NoError(t, err)
	assert.Empty(t, errs, "empty config should be valid against placeholder schema")
}

func TestValidateCoreFileValid(t *testing.T) {
	initTestSchema(t)
	errs, err := ValidateCoreConfig(map[string]interface{}{
		"api_key": "abc123",
		"site":    "datadoghq.com",
	})
	assert.NoError(t, err)
	assert.Empty(t, errs)
	assert.Nil(t, errs)
}

func TestValidateSystemProbeFileValid(t *testing.T) {
	initTestSchema(t)
	errs, err := ValidateSystemProbeConfig(map[string]interface{}{
		"system_probe_config": map[string]interface{}{
			"enabled": true,
		},
	})
	assert.NoError(t, err)
	assert.Empty(t, errs)
}

func TestValidateCoreFileInvalidType(t *testing.T) {
	initTestSchema(t)
	errs, err := ValidateCoreConfig(map[string]interface{}{"api_key": 1234})
	assert.NoError(t, err)
	assert.Equal(t, []string{"at '/api_key': got number, want string"}, errs)

	errs, err = ValidateCoreConfig(map[string]interface{}{"tags": map[string]interface{}{"a": "1234"}})
	assert.NoError(t, err)
	assert.Equal(t, []string{"at '/tags': got object, want array"}, errs)
}

func TestValidateCoreConfigDetailed(t *testing.T) {
	initTestSchema(t)
	tests := []struct {
		name       string
		config     interface{}
		violations []Violation
		errors     []string
	}{
		{
			name: "required nested property with escaped pointer tokens",
			config: map[string]interface{}{
				"parent": map[string]interface{}{"required/key~name": "not an integer"},
			},
			violations: []Violation{{
				Message:       "at '/parent/required~1key~0name': got string, want integer",
				Path:          "/parent/required~1key~0name",
				Rule:          "type",
				ActualType:    "string",
				ExpectedTypes: []string{"integer"},
				Required:      true,
			}},
			errors: []string{"at '/parent/required~1key~0name': got string, want integer"},
		},
		{
			name: "optional nested property",
			config: map[string]interface{}{
				"parent": map[string]interface{}{
					"optional":          "not a boolean",
					"required/key~name": 1,
				},
			},
			violations: []Violation{{
				Message:       "at '/parent/optional': got string, want boolean",
				Path:          "/parent/optional",
				Rule:          "type",
				ActualType:    "string",
				ExpectedTypes: []string{"boolean"},
				Required:      false,
			}},
			errors: []string{"at '/parent/optional': got string, want boolean"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, err := ValidateCoreConfigDetailed(tt.config)
			assert.NoError(t, err)
			assert.Equal(t, tt.violations, violations)

			errors, err := ValidateCoreConfig(tt.config)
			assert.NoError(t, err)
			assert.Equal(t, tt.errors, errors)
		})
	}
}

func TestValidateCoreConfigDetailedRootPath(t *testing.T) {
	initTestSchema(t)
	c := jsonschema.NewCompiler()
	require.NoError(t, c.AddResource("root_type_schema", map[string]interface{}{"type": "integer"}))
	rootTypeSchema, err := c.Compile("root_type_schema")
	require.NoError(t, err)
	previousCoreSchemaGetter := coreSchemaGetter
	t.Cleanup(func() { coreSchemaGetter = previousCoreSchemaGetter })
	coreSchemaGetter = func() (*jsonschema.Schema, error) { return rootTypeSchema, nil }

	violations, err := ValidateCoreConfigDetailed("not an integer")
	assert.NoError(t, err)
	assert.Equal(t, []Violation{{
		Message:       "at '': got string, want integer",
		Path:          "",
		Rule:          "type",
		ActualType:    "string",
		ExpectedTypes: []string{"integer"},
		Required:      false,
	}}, violations)
}
