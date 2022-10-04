// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package state

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestASMData(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  ASMDataConfig
		raw  []byte
	}{
		{
			name: "null",
			cfg:  ASMDataConfig{},
			raw:  []byte(`{"rules_data":null}`),
		},
		{
			name: "empty",
			cfg:  ASMDataConfig{Config: ASMDataRulesData{RulesData: []ASMDataRuleData{}}},
			raw:  []byte(`{"rules_data":[]}`),
		},
		{
			name: "single-entry-float",
			cfg: ASMDataConfig{
				Config: ASMDataRulesData{
					RulesData: []ASMDataRuleData{
						{
							ID:   "1",
							Type: "test1",
							Data: []interface{}{float64(1)},
						},
					},
				},
			},
			raw: []byte(`{"rules_data":[{"id":"1","type":"test1","data":[1]}]}`),
		},
		{
			name: "single-entry-string",
			cfg: ASMDataConfig{
				Config: ASMDataRulesData{
					RulesData: []ASMDataRuleData{
						{
							ID:   "1",
							Type: "test1",
							Data: []interface{}{"1"},
						},
					},
				},
			},
			raw: []byte(`{"rules_data":[{"id":"1","type":"test1","data":["1"]}]}`),
		},
		{
			name: "multiple-entries-float",
			cfg: ASMDataConfig{
				Config: ASMDataRulesData{
					RulesData: []ASMDataRuleData{
						{
							ID:   "empty",
							Type: "",
							Data: []interface{}{},
						},
						{
							ID:   "1",
							Type: "test1",
							Data: []interface{}{float64(1)},
						},
						{
							ID:   "2",
							Type: "test2",
							Data: []interface{}{float64(1), float64(2), float64(3)},
						},
					},
				},
			},
			raw: []byte(`{"rules_data":[{"id":"empty","type":"","data":[]},{"id":"1","type":"test1","data":[1]},{"id":"2","type":"test2","data":[1,2,3]}]}`),
		},
		{
			name: "multiple-entries-string",
			cfg: ASMDataConfig{
				Config: ASMDataRulesData{
					RulesData: []ASMDataRuleData{
						{
							ID:   "empty",
							Type: "",
							Data: []interface{}{},
						},
						{
							ID:   "1",
							Type: "test1",
							Data: []interface{}{"1"},
						},
						{
							ID:   "2",
							Type: "test2",
							Data: []interface{}{"1", "2", "3"},
						},
					},
				},
			},
			raw: []byte(`{"rules_data":[{"id":"empty","type":"","data":[]},{"id":"1","type":"test1","data":["1"]},{"id":"2","type":"test2","data":["1","2","3"]}]}`),
		},
	} {
		t.Run("marshall-"+tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.cfg.Config)
			require.NoError(t, err)
			require.Equal(t, tc.raw, raw)
		})

		t.Run("unmarshall-"+tc.name, func(t *testing.T) {
			var cfg ASMDataConfig
			err := json.Unmarshal(tc.raw, &cfg.Config)
			require.NoError(t, err)
			require.Equal(t, tc.cfg, cfg)
		})

		t.Run("parse-"+tc.name, func(t *testing.T) {
			cfg, err := parseConfigASMData(tc.raw, Metadata{})
			require.NoError(t, err)
			require.Equal(t, tc.cfg, cfg)
		})
	}
}
