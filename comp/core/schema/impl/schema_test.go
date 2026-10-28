// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package schemaimpl

import (
	"errors"
	"fmt"
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
  proxy_url:
    type: string
    pattern: "^https://"
    default: ""
`
	testSchema        *jsonschema.Schema
	schemaCompileOnce sync.Once
)

// newTestValidator returns a schema validator using testYAMLSchema for every known schema ID. The schema getter is
// restored when the test ends.
func newTestValidator(t *testing.T) *schemaValidator {
	schemaCompileOnce.Do(func() {
		c := jsonschema.NewCompiler()

		var loadSchema interface{}
		err := yaml.Unmarshal([]byte(testYAMLSchema), &loadSchema)
		require.NoError(t, err, "could not unmarshal schema")
		require.NoError(t, c.AddResource("test_schema", loadSchema), "could not add schema resource")
		testSchema, err = c.Compile("test_schema")
		require.NoError(t, err, "could not compile schema")
	})

	t.Cleanup(func() { schemaGetter = loadSchema })
	schemaGetter = func(schemaID model.SchemaID) (*jsonschema.Schema, error) {
		if schemaID != model.CoreSchema && schemaID != model.SystemProbeSchema {
			return nil, fmt.Errorf("unknown schema ID '%s'", schemaID)
		}
		return testSchema, nil
	}
	return NewComponent().Comp.(*schemaValidator)
}

// fakeReader is a minimal model.Reader returning a fixed set of non-default settings.
type fakeReader struct {
	model.Reader
	schemaID model.SchemaID
	settings map[string]interface{}
	// notSettings lists the top-level keys that aren't known settings
	notSettings map[string]bool
}

func (f fakeReader) GetSchemaID() model.SchemaID { return f.schemaID }

func (f fakeReader) AllSettingsWithoutDefault() map[string]interface{} { return f.settings }

func (f fakeReader) AllSettingsBySource() map[model.Source]interface{} { return nil }

func (f fakeReader) IsSetting(key string) bool {
	_, ok := f.settings[key]
	return ok && !f.notSettings[key]
}

func TestValidateYAMLEmpty(t *testing.T) {
	v := newTestValidator(t)
	for _, id := range []model.SchemaID{model.CoreSchema, model.SystemProbeSchema} {
		violations, err := v.ValidateYAML([]byte("{}"), id)
		assert.NoError(t, err)
		assert.Empty(t, violations, "empty config should be valid against placeholder schema")
	}
}

func TestValidateYAMLValid(t *testing.T) {
	v := newTestValidator(t)
	violations, err := v.ValidateYAML([]byte("api_key: abc123\nsite: datadoghq.com\n"), model.CoreSchema)
	assert.NoError(t, err)
	assert.Empty(t, violations)

	violations, err = v.ValidateYAML([]byte("system_probe_config:\n  enabled: true\n"), model.SystemProbeSchema)
	assert.NoError(t, err)
	assert.Empty(t, violations)
}

func TestValidateYAMLInvalidType(t *testing.T) {
	v := newTestValidator(t)
	violations, err := v.ValidateYAML([]byte("api_key: 1234"), model.CoreSchema)
	require.NoError(t, err)
	assert.Equal(t, []*model.Violation{{
		Message:       "at '/api_key': got number, want string",
		Path:          "/api_key",
		ActualType:    "number",
		ExpectedTypes: []string{"string"},
	}}, violations)

	violations, err = v.ValidateYAML([]byte("tags:\n  a: \"1234\"\n"), model.CoreSchema)
	require.NoError(t, err)
	assert.Equal(t, []*model.Violation{{
		Message:       "at '/tags': got object, want array",
		Path:          "/tags",
		ActualType:    "object",
		ExpectedTypes: []string{"array"},
	}}, violations)
}

func TestValidateYAMLInvalidYAML(t *testing.T) {
	v := newTestValidator(t)
	_, err := v.ValidateYAML([]byte("api_key: [unclosed"), model.CoreSchema)
	assert.ErrorContains(t, err, "invalid YAML for schema 'core'")
}

func TestValidateYAMLUnknownSchemaID(t *testing.T) {
	v := newTestValidator(t)
	_, err := v.ValidateYAML([]byte("api_key: abc"), "unknown")
	assert.ErrorContains(t, err, "unknown schema ID 'unknown'")
}

func TestValidateConfigEmpty(t *testing.T) {
	v := newTestValidator(t)
	violations, err := v.ValidateConfig(fakeReader{schemaID: model.CoreSchema})
	assert.NoError(t, err)
	assert.Empty(t, violations)
}

func TestValidateConfigTypeViolation(t *testing.T) {
	v := newTestValidator(t)
	for _, id := range []model.SchemaID{model.CoreSchema, model.SystemProbeSchema} {
		violations, err := v.ValidateConfig(fakeReader{schemaID: id, settings: map[string]interface{}{"api_key": 1234}})
		require.NoError(t, err)
		assert.Equal(t, []*model.Violation{{
			Message:       "at '/api_key': got number, want string",
			Path:          "/api_key",
			SettingKey:    "api_key",
			ActualType:    "number",
			ExpectedTypes: []string{"string"},
		}}, violations)
	}
}

func TestValidateConfigUnknownSetting(t *testing.T) {
	v := newTestValidator(t)
	cfg := fakeReader{schemaID: model.CoreSchema, settings: map[string]interface{}{"api_key": 1234}, notSettings: map[string]bool{"api_key": true}}
	violations, err := v.ValidateConfig(cfg)
	require.NoError(t, err)
	require.Len(t, violations, 1)
	assert.Equal(t, "/api_key", violations[0].Path)
	assert.Empty(t, violations[0].SettingKey)
}

func TestValidateConfigUnknownSchemaID(t *testing.T) {
	v := newTestValidator(t)
	_, err := v.ValidateConfig(fakeReader{schemaID: "unknown", settings: map[string]interface{}{"api_key": "abc"}})
	assert.ErrorContains(t, err, "unknown schema ID 'unknown'")
}

func TestValidateConfigSettingKey(t *testing.T) {
	v := newTestValidator(t)
	violations, err := v.ValidateConfig(fakeReader{schemaID: model.CoreSchema, settings: map[string]interface{}{"tags": []interface{}{"a", 1}}})
	require.NoError(t, err)
	require.Len(t, violations, 1)
	assert.Equal(t, "/tags/1", violations[0].Path)
	assert.Equal(t, "tags", violations[0].SettingKey)
}

func TestValidateConfigScrubsMessages(t *testing.T) {
	v := newTestValidator(t)
	cfg := fakeReader{schemaID: model.CoreSchema, settings: map[string]interface{}{"proxy_url": "http://user:PRIVATE_PASSWORD@proxy.test"}}
	violations, err := v.ValidateConfig(cfg)
	require.NoError(t, err)
	require.Len(t, violations, 1)
	assert.Equal(t, "at '/proxy_url': 'http://user:********@proxy.test' does not match pattern '^https://'", violations[0].Message)
}

func TestValidateConfigSkipsSecretPlaceholders(t *testing.T) {
	// the configuration is validated before secrets are resolved: a placeholder isn't the actual value
	for _, tc := range []struct {
		name     string
		settings map[string]interface{}
		want     int
	}{
		{"placeholder", map[string]interface{}{"tags": "ENC[handle]"}, 0},
		{"placeholder with spaces", map[string]interface{}{"tags": " ENC[handle] "}, 0},
		{"placeholder item", map[string]interface{}{"tags": []interface{}{"a", "ENC[handle]"}}, 0},
		{"value", map[string]interface{}{"tags": "not a list"}, 1},
		{"enc-looking item in an invalid list", map[string]interface{}{"tags": []interface{}{1, "ENC[handle]"}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestValidator(t)
			violations, err := v.ValidateConfig(fakeReader{schemaID: model.CoreSchema, settings: tc.settings})
			require.NoError(t, err)
			assert.Len(t, violations, tc.want)
		})
	}
}

func TestGetViolations(t *testing.T) {
	v := newTestValidator(t)
	invalid := fakeReader{schemaID: model.CoreSchema, settings: map[string]interface{}{"api_key": 1234}}
	valid := fakeReader{schemaID: model.CoreSchema, settings: map[string]interface{}{"api_key": "abc"}}
	invalidSysprobe := fakeReader{schemaID: model.SystemProbeSchema, settings: map[string]interface{}{"tags": "abc"}}

	assert.Nil(t, v.GetViolations(model.CoreSchema), "no configuration was validated")

	violations, err := v.ValidateConfig(invalid)
	require.NoError(t, err)
	require.Len(t, violations, 1)
	assert.Equal(t, violations, v.GetViolations(model.CoreSchema))

	// each schema has its own violations
	sysprobeViolations, err := v.ValidateConfig(invalidSysprobe)
	require.NoError(t, err)
	assert.Equal(t, sysprobeViolations, v.GetViolations(model.SystemProbeSchema))
	assert.Equal(t, violations, v.GetViolations(model.CoreSchema))

	// the configuration marks the reverted violations in the returned slice, which is the recorded one
	violations[0].MarkAsReverted()
	assert.True(t, v.GetViolations(model.CoreSchema)[0].WasReverted())

	// callers of GetViolations get their own slice and can't change the recorded one
	sysprobeSlice := v.GetViolations(model.SystemProbeSchema)
	sysprobeSlice[0] = nil
	assert.NotNil(t, v.GetViolations(model.SystemProbeSchema)[0])

	// a new validation replaces the recorded violations
	_, err = v.ValidateConfig(valid)
	require.NoError(t, err)
	assert.Empty(t, v.GetViolations(model.CoreSchema))
}

func TestGetViolationsAfterError(t *testing.T) {
	v := newTestValidator(t)
	_, err := v.ValidateConfig(fakeReader{schemaID: model.CoreSchema, settings: map[string]interface{}{"api_key": 1234}})
	require.NoError(t, err)
	require.NotEmpty(t, v.GetViolations(model.CoreSchema))

	// a failed validation doesn't keep reporting the violations of a previous one
	schemaGetter = func(model.SchemaID) (*jsonschema.Schema, error) { return nil, errors.New("no schema") }
	_, err = v.ValidateConfig(fakeReader{schemaID: model.CoreSchema, settings: map[string]interface{}{"api_key": 1234}})
	require.Error(t, err)
	assert.Nil(t, v.GetViolations(model.CoreSchema))
}
