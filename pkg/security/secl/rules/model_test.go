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
	"github.com/stretchr/testify/require"
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

func TestSetDefinitionJSONNumbers(t *testing.T) {
	tests := []struct {
		name                 string
		json                 string
		expectedValue        interface{}
		expectedDefaultValue interface{}
	}{
		{
			name:          "integer",
			json:          `{"name":"var","value":123}`,
			expectedValue: 123,
		},
		{
			name:                 "negative default value",
			json:                 `{"name":"var","expression":"process.pid","default_value":-1}`,
			expectedDefaultValue: -1,
		},
		{
			name:                 "arrays",
			json:                 `{"name":"var","value":[1,2],"default_value":[0]}`,
			expectedValue:        []interface{}{1, 2},
			expectedDefaultValue: []interface{}{0},
		},
		{
			name:          "not an integer",
			json:          `{"name":"var","value":1.5}`,
			expectedValue: 1.5,
		},
		{
			name:          "out of the int range",
			json:          `{"name":"var","value":1e20}`,
			expectedValue: 1e20,
		},
		{
			name:          "string",
			json:          `{"name":"var","value":"123"}`,
			expectedValue: "123",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var setDef SetDefinition
			require.NoError(t, json.Unmarshal([]byte(test.json), &setDef))
			require.NoError(t, setDef.PreCheck(PolicyLoaderOpts{}))
			assert.Equal(t, test.expectedValue, setDef.Value)
			assert.Equal(t, test.expectedDefaultValue, setDef.DefaultValue)
		})
	}
}

func TestSetDefinitionCapture(t *testing.T) {
	t.Run("yaml round trip", func(t *testing.T) {
		setDef := SetDefinition{
			Name:      "ssm_command_id",
			Field:     "open.file.path",
			Capture:   "/orchestration/([^/]+)/",
			Scope:     ScopeProcess,
			Inherited: true,
		}
		bytes, err := yaml.Marshal(setDef)
		assert.NoError(t, err)
		var deserialized SetDefinition
		err = yaml.Unmarshal(bytes, &deserialized)
		assert.NoError(t, err)
		assert.True(t, reflect.DeepEqual(setDef, deserialized))
	})

	t.Run("json round trip", func(t *testing.T) {
		setDef := SetDefinition{
			Name:    "imds_iam_role",
			Field:   "imds.url",
			Capture: "/security-credentials/([^/?]+)",
		}
		bytes, err := json.Marshal(setDef)
		assert.NoError(t, err)
		var deserialized SetDefinition
		err = json.Unmarshal(bytes, &deserialized)
		assert.NoError(t, err)
		assert.True(t, reflect.DeepEqual(setDef, deserialized))
	})

	t.Run("with field", func(t *testing.T) {
		setDef := SetDefinition{
			Name:    "ssm_command_id",
			Field:   "open.file.path",
			Capture: "/orchestration/([^/]+)/",
		}
		assert.NoError(t, setDef.PreCheck(PolicyLoaderOpts{}))
	})

	// the error strings are asserted rather than just the presence of an error: several of
	// the cases below are rejected by the pre-existing 'value'/'field'/'expression'
	// exclusivity check rather than by the 'capture' check, and a bare assert.Error cannot
	// tell the two apart
	t.Run("with value", func(t *testing.T) {
		setDef := SetDefinition{
			Name:    "ssm_command_id",
			Value:   "some_value",
			Capture: "/orchestration/([^/]+)/",
		}
		assert.EqualError(t, setDef.PreCheck(PolicyLoaderOpts{}), "'capture' can only be used along with 'field'")
	})

	t.Run("with expression", func(t *testing.T) {
		setDef := SetDefinition{
			Name:         "ssm_command_id",
			Expression:   "open.file.path",
			DefaultValue: "",
			Capture:      "/orchestration/([^/]+)/",
		}
		assert.EqualError(t, setDef.PreCheck(PolicyLoaderOpts{}), "'capture' can only be used along with 'field'")
	})

	// the two cases below never reach the 'capture' check: with nothing else set, and
	// with 'field' set alongside 'value', the pre-existing exclusivity check rejects them
	// first. The two mechanisms together are what make 'capture' reachable only through
	// 'field'
	t.Run("without field, value or expression", func(t *testing.T) {
		setDef := SetDefinition{
			Name:    "ssm_command_id",
			Capture: "/orchestration/([^/]+)/",
		}
		assert.EqualError(t, setDef.PreCheck(PolicyLoaderOpts{}), "either 'value', 'field' or 'expression' must be specified")
	})

	t.Run("with field and value", func(t *testing.T) {
		setDef := SetDefinition{
			Name:    "ssm_command_id",
			Field:   "open.file.path",
			Value:   "some_value",
			Capture: "/orchestration/([^/]+)/",
		}
		assert.EqualError(t, setDef.PreCheck(PolicyLoaderOpts{}), "either 'value', 'field' or 'expression' must be specified")
	})
}

func TestNetworkFilterDefinitionEmptyScopeDefault(t *testing.T) {
	n := &NetworkFilterDefinition{
		BPFFilter: "tcp port 80",
		Policy:    NetworkFilterPolicyDrop,
	}
	opts := PolicyLoaderOpts{
		ValidateBPFFilter: func(_ string) error { return nil },
	}
	err := n.PreCheck(opts)
	assert.NoError(t, err)
	assert.Equal(t, "process", n.Scope)
}

func TestNetworkFilterDefinitionEmptyScopeDefaultOnRuleLoad(t *testing.T) {
	actionDef := &ActionDefinition{
		NetworkFilter: &NetworkFilterDefinition{
			BPFFilter: "tcp port 80",
			Policy:    NetworkFilterPolicyDrop,
		},
	}

	opts := PolicyLoaderOpts{
		ValidateBPFFilter: func(_ string) error { return nil },
	}
	err := actionDef.PreCheck(opts)
	assert.NoError(t, err)
	assert.Equal(t, "process", actionDef.NetworkFilter.Scope)
}
