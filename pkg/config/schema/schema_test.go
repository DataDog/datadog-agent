// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package schema

import (
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/assert/yaml"
	"github.com/stretchr/testify/require"
)

var (
	testYAMLSchema = `
properties:
  api_key:
    type: string
    default: ""
  tags:
    type: array
    items:
      type: string
    default: []
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

// fakeReader is a minimal model.Reader returning a fixed set of non-default settings.
// pkg/config/mock can't be used here: it imports pkg/config/setup which imports this package.
type fakeReader struct {
	model.Reader
	settings map[string]interface{}
}

func (f fakeReader) AllSettingsWithoutDefault() map[string]interface{} { return f.settings }

func (f fakeReader) AllSettingsBySource() map[model.Source]interface{} { return nil }

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

func TestValidateYAMLEmpty(t *testing.T) {
	initTestSchema(t)
	for _, id := range []model.SchemaID{model.CoreSchema, model.SystemProbeSchema} {
		violations, err := ValidateYAML([]byte("{}"), id)
		assert.NoError(t, err)
		assert.Empty(t, violations, "empty config should be valid against placeholder schema")
	}
}

func TestValidateYAMLValid(t *testing.T) {
	initTestSchema(t)
	violations, err := ValidateYAML([]byte("api_key: abc123\nsite: datadoghq.com\n"), model.CoreSchema)
	assert.NoError(t, err)
	assert.Empty(t, violations)

	violations, err = ValidateYAML([]byte("system_probe_config:\n  enabled: true\n"), model.SystemProbeSchema)
	assert.NoError(t, err)
	assert.Empty(t, violations)
}

func TestValidateYAMLInvalidType(t *testing.T) {
	initTestSchema(t)
	violations, err := ValidateYAML([]byte("api_key: 1234"), model.CoreSchema)
	require.NoError(t, err)
	assert.Equal(t, []Violation{{
		Message:       "at '/api_key': got number, want string",
		Path:          "/api_key",
		ActualType:    "number",
		ExpectedTypes: []string{"string"},
	}}, violations)

	violations, err = ValidateYAML([]byte("tags:\n  a: \"1234\"\n"), model.CoreSchema)
	require.NoError(t, err)
	assert.Equal(t, []Violation{{
		Message:       "at '/tags': got object, want array",
		Path:          "/tags",
		ActualType:    "object",
		ExpectedTypes: []string{"array"},
	}}, violations)
}

func TestValidateYAMLInvalidYAML(t *testing.T) {
	initTestSchema(t)
	_, err := ValidateYAML([]byte("api_key: [unclosed"), model.CoreSchema)
	assert.ErrorContains(t, err, "invalid YAML for schema 'core'")
}

func TestValidateYAMLUnknownSchemaID(t *testing.T) {
	initTestSchema(t)
	_, err := ValidateYAML([]byte("api_key: abc"), "unknown")
	assert.ErrorContains(t, err, "unknown schema ID 'unknown'")
}

func TestValidateConfigEmpty(t *testing.T) {
	initTestSchema(t)
	violations, err := ValidateConfig(fakeReader{}, model.CoreSchema)
	assert.NoError(t, err)
	assert.Empty(t, violations)
}

func TestValidateConfigTypeViolation(t *testing.T) {
	initTestSchema(t)
	for _, id := range []model.SchemaID{model.CoreSchema, model.SystemProbeSchema} {
		violations, err := ValidateConfig(fakeReader{settings: map[string]interface{}{"api_key": 1234}}, id)
		require.NoError(t, err)
		assert.Equal(t, []Violation{{
			Message:       "at '/api_key': got number, want string",
			Path:          "/api_key",
			ActualType:    "number",
			ExpectedTypes: []string{"string"},
		}}, violations)
	}
}

func TestValidateConfigUnknownSchemaID(t *testing.T) {
	initTestSchema(t)
	_, err := ValidateConfig(fakeReader{settings: map[string]interface{}{"api_key": "abc"}}, "unknown")
	assert.ErrorContains(t, err, "unknown schema ID 'unknown'")
}

// layeredReader is a minimal model.Reader where the effective value of every setting comes from the agent-runtime layer.
type layeredReader struct {
	model.Reader
	effective map[string]interface{}
	layers    map[model.Source]interface{}
}

func (l layeredReader) AllSettingsWithoutDefault() map[string]interface{} { return l.effective }

func (l layeredReader) AllSettingsBySource() map[model.Source]interface{} { return l.layers }

func (l layeredReader) GetSource(string) model.Source { return model.SourceAgentRuntime }

func (l layeredReader) IsSetting(key string) bool { return key == "api_key" || key == "tags" }

func TestValidateConfigIgnoresAgentRuntime(t *testing.T) {
	initTestSchema(t)
	cfg := layeredReader{
		effective: map[string]interface{}{"api_key": "repaired", "tags": []interface{}{"runtime"}},
		layers: map[model.Source]interface{}{
			model.SourceDefault:      map[string]interface{}{"tags": []interface{}{}},
			model.SourceFile:         map[string]interface{}{"api_key": 1234},
			model.SourceAgentRuntime: map[string]interface{}{"api_key": "repaired", "tags": []interface{}{"runtime"}},
		},
	}
	// api_key is reverted to the customer's value and tags, only set by the agent, is not validated
	assert.Equal(t, map[string]interface{}{"api_key": 1234}, customerConfig(cfg))

	violations, err := ValidateConfig(cfg, model.CoreSchema)
	require.NoError(t, err)
	assert.Equal(t, []Violation{{
		Message:       "at '/api_key': got number, want string",
		Path:          "/api_key",
		ActualType:    "number",
		ExpectedTypes: []string{"string"},
	}}, violations)
}
