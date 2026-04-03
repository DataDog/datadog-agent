// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v3"
)

func TestDuration(t *testing.T) {
	t.Run("json marshaling", func(t *testing.T) {
		setDef := SetDefinition{
			Name:  "set_name",
			Value: float64(123),
			Scope: "container",
			TTL: &HumanReadableDuration{
				Duration: 1 * time.Second,
			},
		}
		bytes, err := json.Marshal(setDef)
		assert.NoError(t, err)
		var deserialized SetDefinition
		err = json.Unmarshal(bytes, &deserialized)
		assert.NoError(t, err)
		assert.Equal(t, setDef.Value, deserialized.Value)
		assert.True(t, reflect.DeepEqual(setDef, deserialized))
	})

	t.Run("json marshaling with 0 duration", func(t *testing.T) {
		setDef := SetDefinition{
			Name:  "set_name",
			Value: float64(123),
			Scope: "container",
			TTL: &HumanReadableDuration{
				Duration: 0,
			},
		}
		bytes, err := json.Marshal(setDef)
		assert.NoError(t, err)
		var deserialized SetDefinition
		err = json.Unmarshal(bytes, &deserialized)
		assert.NoError(t, err)
		assert.Equal(t, setDef.Value, deserialized.Value)
		assert.True(t, reflect.DeepEqual(setDef, deserialized))
	})

	t.Run("json unmarshalling", func(t *testing.T) {
		setDef := SetDefinition{
			Name:  "set_name",
			Value: float64(123),
			Scope: "container",
			TTL: &HumanReadableDuration{
				Duration: 1 * time.Second,
			},
		}
		var deserialized SetDefinition
		err := json.Unmarshal([]byte(`{"name":"set_name","value":123,"scope":"container","ttl":"1s"}`), &deserialized)
		assert.NoError(t, err)
		assert.True(t, reflect.DeepEqual(setDef, deserialized))
		err = json.Unmarshal([]byte(`{"name":"set_name","value":123,"scope":"container","ttl":1000000000}`), &deserialized)
		assert.NoError(t, err)
		assert.True(t, reflect.DeepEqual(setDef, deserialized))
	})

	t.Run("json unmarshalling 0 duration", func(t *testing.T) {
		setDef := SetDefinition{
			Name:  "set_name",
			Value: float64(123),
			Scope: "container",
			TTL: &HumanReadableDuration{
				Duration: 0 * time.Second,
			},
		}
		var deserialized SetDefinition
		err := json.Unmarshal([]byte(`{"name":"set_name","value":123,"scope":"container","ttl":"0s"}`), &deserialized)
		assert.NoError(t, err)
		assert.True(t, reflect.DeepEqual(setDef, deserialized))
		err = json.Unmarshal([]byte(`{"name":"set_name","value":123,"scope":"container","ttl":0}`), &deserialized)
		assert.NoError(t, err)
		assert.True(t, reflect.DeepEqual(setDef, deserialized))
	})

	t.Run("yaml marshaling", func(t *testing.T) {
		setDef := SetDefinition{
			Name:  "set_name",
			Value: 123,
			Scope: "container",
			TTL: &HumanReadableDuration{
				Duration: 1 * time.Second,
			},
		}
		bytes, err := yaml.Marshal(setDef)
		assert.NoError(t, err)
		var deserialized SetDefinition
		err = yaml.Unmarshal(bytes, &deserialized)
		assert.NoError(t, err)
		assert.True(t, reflect.DeepEqual(setDef, deserialized))
	})

	t.Run("yaml unmarshalling", func(t *testing.T) {
		setDef := SetDefinition{
			Name:  "set_name",
			Value: 123,
			Scope: "container",
			TTL: &HumanReadableDuration{
				Duration: 1 * time.Second,
			},
		}
		var deserialized SetDefinition
		serialized := "name: set_name\nvalue: 123\nscope: container\nttl: 1s\n"
		err := yaml.Unmarshal([]byte(serialized), &deserialized)
		assert.NoError(t, err)
		assert.True(t, reflect.DeepEqual(setDef, deserialized))
	})
}
