// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTags(t *testing.T) {
	c, _ := newDefaultCheck(t, `tags:
  - foo1:bar1
  - foo2:bar2`, "")
	defer c.Teardown()
	err := c.Run()
	require.NoError(t, err)
	assert.True(t, c.initialized, "Check not initialized")
	assert.Contains(t, c.tags, dbmsTag, "Static tag not merged")
	assert.Contains(t, c.tags, "foo1:bar1", "Config tag not in tags")
}

// This test is just used for debugging database init issues
// To use, set the assert to always fail then run the test
func TestNoop(t *testing.T) {
	assert.True(t, true)
}

func TestMain(m *testing.M) {
	if os.Getenv("SKIP_TEST_MAIN") != "1" {
		if err := setupTestDatabase(); err != nil {
			fmt.Fprintf(os.Stderr, "Oracle test setup failed: %s\n", err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

func TestCreateDatabaseIdentifier(t *testing.T) {
	tests := []struct {
		name           string
		config         func(c *Check)
		expectedResult string
	}{
		{
			name: "Basic configuration",
			config: func(c *Check) {
				c.config.Server = "test-server"
				c.config.Port = 1521
				c.config.ServiceName = "test-service"
				c.config.DatabaseIdentifier.Template = "$resolved_hostname:$server:$port:$cdb_name:$service_name"
				c.dbResolvedHostname = "test-hostname"
				c.cdbName = "test-cdb"
			},
			expectedResult: "test-hostname:test-server:1521:test-cdb:test-service",
		},
		{
			name: "Missing hostname",
			config: func(c *Check) {
				c.config.Server = "test-server"
				c.config.Port = 1521
				c.config.ServiceName = "test-service"
				c.config.DatabaseIdentifier.Template = "$resolved_hostname:$server:$port:$cdb_name:$service_name"
				c.dbResolvedHostname = ""
				c.cdbName = "test-cdb"
			},
			expectedResult: "$resolved_hostname:test-server:1521:test-cdb:test-service",
		},
		{
			name: "Custom template",
			config: func(c *Check) {
				c.config.Server = "custom-server"
				c.config.Port = 3306
				c.config.ServiceName = "custom-service"
				c.config.DatabaseIdentifier.Template = "$server-$port-$service_name"
				c.dbResolvedHostname = "custom-hostname"
				c.cdbName = "custom-cdb"
			},
			expectedResult: "custom-server-3306-custom-service",
		},
		{
			name: "Empty template",
			config: func(c *Check) {
				c.config.Server = "empty-server"
				c.config.Port = 5432
				c.config.ServiceName = "empty-service"
				c.config.DatabaseIdentifier.Template = ""
				c.dbResolvedHostname = "empty-hostname"
				c.cdbName = "empty-cdb"
			},
			expectedResult: "",
		},
		{
			name: "Template with missing variables",
			config: func(c *Check) {
				c.config.Server = "partial-server"
				c.config.Port = 1521
				c.config.ServiceName = "partial-service"
				c.config.DatabaseIdentifier.Template = "$server:$port:$missing_variable"
				c.dbResolvedHostname = "partial-hostname"
				c.cdbName = "partial-cdb"
			},
			expectedResult: "partial-server:1521:$missing_variable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newDbDoesNotExistCheck(t, "", "")
			defer c.Teardown()

			// Apply test-specific configuration
			tt.config(&c)

			// Generate the database identifier
			identifier := c.createDatabaseIdentifier()

			// Assertions
			assert.Equal(t, tt.expectedResult, identifier, "Database identifier does not match expected value")
		})
	}
}
