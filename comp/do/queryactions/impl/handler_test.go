// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package doqueryactionsimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v2"
)

func TestIsPostgresIntegration(t *testing.T) {
	assert.True(t, isPostgresIntegration("postgres"))
	assert.False(t, isPostgresIntegration("mysql"))
	assert.False(t, isPostgresIntegration("redis"))
	assert.False(t, isPostgresIntegration(""))
}

func TestMatchesDBName(t *testing.T) {
	t.Run("empty RC dbname matches any instance", func(t *testing.T) {
		instance := map[string]any{"dbname": "mydb"}
		dbID := &DBIdentifier{DBName: ""}
		assert.True(t, matchesDBName(instance, dbID))
	})

	t.Run("empty instance dbname matches any RC", func(t *testing.T) {
		instance := map[string]any{}
		dbID := &DBIdentifier{DBName: "mydb"}
		assert.True(t, matchesDBName(instance, dbID))
	})

	t.Run("matching dbnames", func(t *testing.T) {
		instance := map[string]any{"dbname": "mydb"}
		dbID := &DBIdentifier{DBName: "mydb"}
		assert.True(t, matchesDBName(instance, dbID))
	})

	t.Run("mismatching dbnames", func(t *testing.T) {
		instance := map[string]any{"dbname": "otherdb"}
		dbID := &DBIdentifier{DBName: "mydb"}
		assert.False(t, matchesDBName(instance, dbID))
	})
}

func TestMatchesIdentifier_SelfHosted(t *testing.T) {
	instance := map[string]any{
		"host": "localhost",
		"port": 5432,
	}
	dbID := &DBIdentifier{Type: "self-hosted", Host: "localhost", Port: 5432}
	assert.True(t, matchesIdentifier(instance, dbID))

	dbID = &DBIdentifier{Type: "self-hosted", Host: "localhost", Port: 5433}
	assert.False(t, matchesIdentifier(instance, dbID))

	dbID = &DBIdentifier{Type: "self-hosted", Host: "otherhost", Port: 5432}
	assert.False(t, matchesIdentifier(instance, dbID))
}

func TestMatchesIdentifier_SelfHosted_WithDBName(t *testing.T) {
	instance := map[string]any{
		"host":   "localhost",
		"port":   5432,
		"dbname": "production",
	}

	t.Run("matching dbname", func(t *testing.T) {
		dbID := &DBIdentifier{Type: "self-hosted", Host: "localhost", Port: 5432, DBName: "production"}
		assert.True(t, matchesIdentifier(instance, dbID))
	})

	t.Run("mismatching dbname", func(t *testing.T) {
		dbID := &DBIdentifier{Type: "self-hosted", Host: "localhost", Port: 5432, DBName: "staging"}
		assert.False(t, matchesIdentifier(instance, dbID))
	})

	t.Run("no dbname in RC matches any", func(t *testing.T) {
		dbID := &DBIdentifier{Type: "self-hosted", Host: "localhost", Port: 5432}
		assert.True(t, matchesIdentifier(instance, dbID))
	})
}

func TestMatchesIdentifier_RDS_HostPort(t *testing.T) {
	instance := map[string]any{
		"host": "mydb.123456.us-east-1.rds.amazonaws.com",
		"port": 5432,
	}
	dbID := &DBIdentifier{
		Type: "rds",
		Host: "mydb.123456.us-east-1.rds.amazonaws.com",
		Port: 5432,
	}
	assert.True(t, matchesIdentifier(instance, dbID))
}

func TestMatchesIdentifier_RDS_DBInstanceIdentifier(t *testing.T) {
	instance := map[string]any{
		"host":                 "mydb.123456.us-east-1.rds.amazonaws.com",
		"port":                 5432,
		"dbinstanceidentifier": "my-rds-instance",
	}
	dbID := &DBIdentifier{
		Type:                 "rds",
		DBInstanceIdentifier: "my-rds-instance",
	}
	assert.True(t, matchesIdentifier(instance, dbID))
}

func TestMatchesIdentifier_RDS_Tags(t *testing.T) {
	instance := map[string]any{
		"host": "mydb.123456.us-east-1.rds.amazonaws.com",
		"port": 5432,
		"tags": []any{"env:prod", "dbinstanceidentifier:my-rds-instance"},
	}
	dbID := &DBIdentifier{
		Type:                 "rds",
		DBInstanceIdentifier: "my-rds-instance",
	}
	assert.True(t, matchesIdentifier(instance, dbID))
}

func TestMatchesIdentifier_RDS_AWSInstanceEndpoint(t *testing.T) {
	// YAML nested maps are map[any]any
	instance := map[string]any{
		"host": "mydb.123456.us-east-1.rds.amazonaws.com",
		"port": 5432,
		"aws": map[any]any{
			"instance_endpoint": "my-rds-instance",
		},
	}
	dbID := &DBIdentifier{
		Type:                 "rds",
		DBInstanceIdentifier: "my-rds-instance",
	}
	assert.True(t, matchesIdentifier(instance, dbID))
}

func TestMatchesIdentifier_RDS_WithDBName(t *testing.T) {
	instance := map[string]any{
		"host":                 "mydb.123456.us-east-1.rds.amazonaws.com",
		"port":                 5432,
		"dbname":               "production",
		"dbinstanceidentifier": "my-rds-instance",
	}

	t.Run("matching dbname via dbinstanceidentifier", func(t *testing.T) {
		dbID := &DBIdentifier{
			Type:                 "rds",
			DBInstanceIdentifier: "my-rds-instance",
			DBName:               "production",
		}
		assert.True(t, matchesIdentifier(instance, dbID))
	})

	t.Run("mismatching dbname via dbinstanceidentifier", func(t *testing.T) {
		dbID := &DBIdentifier{
			Type:                 "rds",
			DBInstanceIdentifier: "my-rds-instance",
			DBName:               "staging",
		}
		assert.False(t, matchesIdentifier(instance, dbID))
	})
}

func TestMatchesIdentifier_UnknownType(t *testing.T) {
	instance := map[string]any{
		"host": "localhost",
		"port": 5432,
	}
	dbID := &DBIdentifier{Type: "unknown", Host: "localhost", Port: 5432}
	assert.False(t, matchesIdentifier(instance, dbID))
}

func TestGetPort(t *testing.T) {
	assert.Equal(t, 5432, getPort(map[string]any{"port": 5432}))
	assert.Equal(t, 5432, getPort(map[string]any{"port": float64(5432)}))
	assert.Equal(t, 5432, getPort(map[string]any{"port": "5432"}))
	assert.Equal(t, 0, getPort(map[string]any{"port": nil}))
	assert.Equal(t, 0, getPort(map[string]any{}))
}

func TestExtractDBAuthFromInstanceData(t *testing.T) {
	instanceYAML := `
host: localhost
port: 5432
username: datadog
password: secret
dbname: testdb
ssl_mode: require
extra_field: should_not_appear
`
	auth := extractDBAuthFromInstanceData(integration.Data(instanceYAML))

	require.Equal(t, "localhost", auth["host"])
	require.Equal(t, 5432, auth["port"])
	require.Equal(t, "datadog", auth["username"])
	require.Equal(t, "secret", auth["password"])
	require.Equal(t, "testdb", auth["dbname"])
	require.Equal(t, "require", auth["ssl_mode"])
	_, ok := auth["extra_field"]
	assert.False(t, ok, "extra_field should not be in allowlist output")
}

func TestExtractDBAuthFromInstanceData_NestedMap(t *testing.T) {
	instanceYAML := `
host: mydb.rds.amazonaws.com
port: 5432
username: datadog
password: secret
aws:
  instance_endpoint: my-rds-instance
  region: us-east-1
`
	auth := extractDBAuthFromInstanceData(integration.Data(instanceYAML))

	require.Equal(t, "mydb.rds.amazonaws.com", auth["host"])
	awsMap, ok := auth["aws"].(map[string]interface{})
	require.True(t, ok, "aws should be converted to map[string]interface{}")
	assert.Equal(t, "my-rds-instance", awsMap["instance_endpoint"])
	assert.Equal(t, "us-east-1", awsMap["region"])
}

func TestExtractDBAuthFromInstanceData_InvalidYAML(t *testing.T) {
	auth := extractDBAuthFromInstanceData(integration.Data("not: [valid: yaml"))
	assert.Empty(t, auth)
}

func TestBuildCheckConfig_MultipleQueries(t *testing.T) {
	c := &component{
		log: logmock.New(t),
	}

	payload := &DOQueryPayload{
		ConfigID: "test-config-1",
		Queries: []QuerySpec{
			{
				MonitorID:       100,
				Query:           "SELECT count(*) FROM orders",
				IntervalSeconds: 60,
				TimeoutSeconds:  10,
				Entity: EntityMetadata{
					Platform: "postgres",
					Database: "shop",
					Schema:   "public",
					Table:    "orders",
				},
			},
			{
				MonitorID:       200,
				Query:           "SELECT avg(price) FROM products",
				IntervalSeconds: 300,
				TimeoutSeconds:  30,
				Entity: EntityMetadata{
					Platform: "postgres",
					Database: "shop",
					Schema:   "public",
					Table:    "products",
				},
			},
		},
	}

	baseCfg := &integration.Config{
		Name:     "postgres",
		Provider: "file",
		NodeName: "node1",
	}

	instanceData := integration.Data(`
host: localhost
port: 5432
username: datadog
password: secret
`)

	checkCfg := c.buildCheckConfig(payload, baseCfg, instanceData, "rc-id-1")

	assert.Equal(t, "do_query_actions", checkCfg.Name)
	assert.Equal(t, "file", checkCfg.Provider)
	assert.Equal(t, "node1", checkCfg.NodeName)
	require.Len(t, checkCfg.Instances, 1)

	// Parse the generated YAML to verify structure
	var instance map[string]any
	err := yaml.Unmarshal(checkCfg.Instances[0], &instance)
	require.NoError(t, err)

	assert.Equal(t, "rc-id-1", instance["remote_config_id"])
	assert.Equal(t, "postgres", instance["db_type"])
	assert.Equal(t, "localhost", instance["host"])
	assert.Equal(t, 5432, instance["port"])
	assert.Equal(t, "datadog", instance["username"])
	assert.Equal(t, "secret", instance["password"])

	// Verify queries list
	queries, ok := instance["queries"].([]interface{})
	require.True(t, ok, "queries should be a list")
	require.Len(t, queries, 2)

	q1, ok := queries[0].(map[interface{}]interface{})
	require.True(t, ok)
	assert.Equal(t, 100, q1["monitor_id"])
	assert.Equal(t, "SELECT count(*) FROM orders", q1["query"])
	assert.Equal(t, 60, q1["interval_seconds"])
	assert.Equal(t, 10, q1["timeout_seconds"])

	entity1, ok := q1["entity"].(map[interface{}]interface{})
	require.True(t, ok)
	assert.Equal(t, "postgres", entity1["platform"])
	assert.Equal(t, "shop", entity1["database"])
	assert.Equal(t, "public", entity1["schema"])
	assert.Equal(t, "orders", entity1["table"])

	q2, ok := queries[1].(map[interface{}]interface{})
	require.True(t, ok)
	assert.Equal(t, 200, q2["monitor_id"])
	assert.Equal(t, "SELECT avg(price) FROM products", q2["query"])

	// Verify run_once is NOT present
	_, hasRunOnce := instance["run_once"]
	assert.False(t, hasRunOnce, "run_once should not be present in declarative model")
}
