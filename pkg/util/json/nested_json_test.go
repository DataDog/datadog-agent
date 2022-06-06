package json

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetNestedValueExists(t *testing.T) {
	rawJSON := []byte(`{"key":"val"}`)
	jsonMap := make(map[string]interface{})
	err := json.Unmarshal(rawJSON, &jsonMap)
	assert.Nil(t, err)

	assert.Equal(t, "val", GetNestedValue(jsonMap, "key"))
}

func TestGetNestedValueExistsNested(t *testing.T) {
	rawJSON := []byte(`{"key":"val", "key2": {"key3": {"key4": "val2"}}}`)
	jsonMap := make(map[string]interface{})
	err := json.Unmarshal(rawJSON, &jsonMap)
	assert.Nil(t, err)

	assert.Equal(t, "val2", GetNestedValue(jsonMap, []string{"key2", "key3", "key4"}...))
}

func TestGetNestedValueExistsStruct(t *testing.T) {
	rawJSON := []byte(`{"key":"val", "key2": {"key3": {"key4": "val2"}}}`)
	jsonMap := make(map[string]interface{})
	err := json.Unmarshal(rawJSON, &jsonMap)
	assert.Nil(t, err)

	assert.Equal(t, map[string]interface{}{
		"key4": "val2",
	}, GetNestedValue(jsonMap, []string{"key2", "key3"}...))
}

func TestGetNestedValueDoesntExist(t *testing.T) {
	rawJSON := []byte(`{"key":"val", "key5": {"key3": {"key4": "val2"}}}`)
	jsonMap := make(map[string]interface{})
	err := json.Unmarshal(rawJSON, &jsonMap)
	assert.Nil(t, err)

	assert.Equal(t, nil, GetNestedValue(jsonMap, []string{"doesnt_exist", "key3"}...))
}

func TestGetNestedValueDoesntExistNested(t *testing.T) {
	rawJSON := []byte(`{"key":"val", "key5": {"key3": {"key4": "val2"}}}`)
	jsonMap := make(map[string]interface{})
	err := json.Unmarshal(rawJSON, &jsonMap)
	assert.Nil(t, err)

	assert.Equal(t, nil, GetNestedValue(jsonMap, []string{"key5", "doesnt_exist"}...))
}
