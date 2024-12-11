// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

package eventdatafilter

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

// TestRegularUnmarshalSchema tests that we can unmarshal a valid schema.
func TestRegularUnmarshalSchema(t *testing.T) {
	config := `
schema_version: 1.0
eventid:
  - 1000
  - 2000
`
	filter, err := unmarshalEventdataFilterSchema([]byte(config))
	if assert.NoError(t, err) {
		assert.Equal(t, []int{1000, 2000}, filter.EventIDs)
	}
}

// TestBumpMinorVersion tests that we can unmarshal a valid schema with a bumped minor version.
func TestBumpMinorVersion(t *testing.T) {
	config := `
schema_version: 1.1
eventid:
  - 1000
  - 2000
`
	filter, err := unmarshalEventdataFilterSchema([]byte(config))
	if assert.NoError(t, err) {
		assert.Equal(t, []int{1000, 2000}, filter.EventIDs)
	}
}

// TestBumpMajorVersionReturnsUnsupported tests that we return an error when the schema version is unsupported.
func TestBumpMajorVersionReturnsUnsupported(t *testing.T) {
	config := `
schema_version: 2.0
eventid:
  - 1000
  - 2000
`
	filter, err := unmarshalEventdataFilterSchema([]byte(config))
	assert.ErrorContains(t, err, "unsupported schema version 2.0")
	assert.ErrorContains(t, err, "please use a version compatible with version 1")
	assert.Nil(t, filter)
}

// TestMissingSchemaVersion tests that we return an error when the schema version is missing.
func TestMissingSchemaVersion(t *testing.T) {
	config := `
eventid:
  - 1000
  - 2000
`
	filter, err := unmarshalEventdataFilterSchema([]byte(config))
	assert.ErrorContains(t, err, "schema_version is required but is missing or empty")
	assert.Nil(t, filter)
}

// TestInvalidEventIDType tests that we return an error when the eventid is not an array.
func TestInvalidEventIDType(t *testing.T) {
	config := `
schema_version: 1.0
eventid: 1000
`
	filter, err := unmarshalEventdataFilterSchema([]byte(config))
	assert.ErrorContains(t, err, "cannot unmarshal !!int `1000` into []int")
	assert.Nil(t, filter)
	config = `
schema_version: 1.0
eventid:
  - toast
`
	filter, err = unmarshalEventdataFilterSchema([]byte(config))
	assert.ErrorContains(t, err, "cannot unmarshal !!str `toast` into int")
	assert.Nil(t, filter)
}

// TestEmptyEventIDList tests that we can unmarshal a schema with an empty eventid list.
func TestEmptyEventIDList(t *testing.T) {
	config := `
schema_version: 1.0
eventid:
`
	filter, err := unmarshalEventdataFilterSchema([]byte(config))
	if assert.NoError(t, err) {
		assert.Empty(t, filter.EventIDs)
	}
}

// TestNoErrorWithExtraKeys tests that extra keys in the config are ignored, in case
// we want to add more fields in the future.
func TestNoErrorWithExtraKeys(t *testing.T) {
	config := `
schema_version: 1.1
eventid:
  - 1000
  - 2000
extra_key: "value"
`
	filter, err := unmarshalEventdataFilterSchema([]byte(config))
	if assert.NoError(t, err) {
		assert.Equal(t, []int{1000, 2000}, filter.EventIDs)
	}
}
