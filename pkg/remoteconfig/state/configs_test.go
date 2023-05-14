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
			name: "single-entry",
			cfg: ASMDataConfig{
				Config: ASMDataRulesData{
					RulesData: []ASMDataRuleData{
						{
							ID:   "1",
							Type: "test1",
							Data: []ASMDataRuleDataEntry{
								{
									Value:      "1",
									Expiration: 1234,
								},
							},
						},
					},
				},
			},
			raw: []byte(`{"rules_data":[{"id":"1","type":"test1","data":[{"expiration":1234,"value":"1"}]}]}`),
		},
		{
			name: "multiple-entries",
			cfg: ASMDataConfig{
				Config: ASMDataRulesData{
					RulesData: []ASMDataRuleData{
						{
							ID:   "empty",
							Type: "",
							Data: []ASMDataRuleDataEntry{},
						},
						{
							ID:   "1",
							Type: "test1",
							Data: []ASMDataRuleDataEntry{
								{
									Value:      "127.0.0.1",
									Expiration: 1234,
								},
							},
						},
						{
							ID:   "2",
							Type: "test2",
							Data: []ASMDataRuleDataEntry{
								{
									Value:      "value1",
									Expiration: 1234,
								},
								{
									Value:      "value2",
									Expiration: 1234,
								},
								{
									Value:      "value3",
									Expiration: 1234,
								},
							},
						},
					},
				},
			},
			raw: []byte(`{"rules_data":[{"id":"empty","type":"","data":[]},{"id":"1","type":"test1","data":[{"expiration":1234,"value":"127.0.0.1"}]},{"id":"2","type":"test2","data":[{"expiration":1234,"value":"value1"},{"expiration":1234,"value":"value2"},{"expiration":1234,"value":"value3"}]}]}`),
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
