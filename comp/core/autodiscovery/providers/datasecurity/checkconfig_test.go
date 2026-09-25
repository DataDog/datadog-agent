// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datasecurity

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v3"
)

const toYAMLRule = `{
  "id": "rule-1",
  "license": "Datadog Confidential",
  "pattern": "(?i)\\b\\d{6}\"x\\b",
  "precedence": "Specific",
  "proximity_keywords": {
    "look_ahead_character_count": 30,
    "included_keywords": ["id", "ssn", "numéro"],
    "excluded_keywords": []
  },
  "validator": {
    "type": "JwtClaimsValidator",
    "config": {"required_claims": {"iss": {"type": "RegexMatch", "config": "^\\w{1,4}:\\d{1,}$"}}}
  }
}`

const toYAMLWant = `min_collection_interval: 0
scan_data:
    - connection:
        dbname: app
        host: db-host
        password: secret
        port: 5432
        username: datadog
      entity:
        database: app
        database_cluster_name: cluster
        database_host_name: db-host
        database_instance_name: instance
        platform: postgres
        schema: public
        table: users
      query: SELECT * FROM users
      sub_task_id: sub-1
      timeout_seconds: 30
scanning_rules:
    - id: rule-1
      license: Datadog Confidential
      pattern: (?i)\b\d{6}"x\b
      precedence: Specific
      proximity_keywords:
        excluded_keywords: []
        included_keywords:
            - id
            - ssn
            - numéro
        look_ahead_character_count: 30
      validator:
        config:
            required_claims:
                iss:
                    config: ^\w{1,4}:\d{1,}$
                    type: RegexMatch
        type: JwtClaimsValidator
task_id: task-1
`

func TestCheckInstanceToYAML(t *testing.T) {
	inst := checkInstance{
		TaskID:        "task-1",
		ScanningRules: []json.RawMessage{json.RawMessage(toYAMLRule)},
		ScanData: []checkSubTask{{
			subTask: subTask{
				SubTaskID: "sub-1",
				Entity: entity{
					Platform:             "postgres",
					DatabaseClusterName:  "cluster",
					DatabaseInstanceName: "instance",
					DatabaseHostName:     "db-host",
					Database:             "app",
					Schema:               "public",
					Table:                "users",
				},
				Query:          "SELECT * FROM users",
				TimeoutSeconds: 30,
			},
			Connection: connection{Host: "db-host", Port: 5432, DBName: "app", Username: "datadog", Password: "secret"},
		}},
	}

	out, err := inst.toYAML()
	require.NoError(t, err)
	assert.Equal(t, toYAMLWant, string(out))

	var got struct {
		ScanningRules []any `yaml:"scanning_rules"`
	}
	require.NoError(t, yaml.Unmarshal(out, &got))
	require.Len(t, got.ScanningRules, 1)
	gotRule, err := json.Marshal(got.ScanningRules[0])
	require.NoError(t, err)
	assert.JSONEq(t, toYAMLRule, string(gotRule))
}
