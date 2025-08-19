// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"fmt"
	"os"
	"strings"
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
	defer func() {
		code := m.Run()
		os.Exit(code)
	}()

	// Set
	// "go.testEnvVars": {
	// 	"SKIP_TEST_MAIN": "1"
	//   }
	// in your settings.json to skip integration test setup
	if os.Getenv("SKIP_TEST_MAIN") == "1" {
		return
	}

	print("Running initdb.d sql files...")
	// This is a bit of a hack to get a db connection without a testing.T
	// Ideally we should pull the connection logic out
	// to make it more accessible for testing
	sysCheck, _ := newSysCheck(nil, "", "")
	sysCheck.Run()
	_, err := sysCheck.db.Exec("SELECT 1 FROM dual")
	if err != nil {
		fmt.Printf("Error executing select check: %s\n", err)
		os.Exit(1)
	}

	initDbPath := "./compose/initdb.d"
	files, _ := os.ReadDir(initDbPath)
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		filename := file.Name()
		bytes, err := os.ReadFile(initDbPath + "/" + filename)
		if err != nil {
			fmt.Printf("Error reading file %s: %s\n", filename, err)
			os.Exit(1)
		}
		fmt.Printf("Executing %s\n", filename)
		sql := string(bytes)
		if strings.HasSuffix(filename, ".nosplit.sql") {
			// For some inits we need to run functions without splitting
			_, err = sysCheck.db.Exec(sql)
			if err != nil {
				fmt.Printf("Error executing as literal \n%s\n %s\n", sql, err)
			}
		} else {
			// Oracle can't handle multiple SQL statements in a single exec
			lines := strings.Split(sql, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "--") {
					continue
				}
				// It also hates semicolons
				trimmed := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ";"))
				if trimmed == "" {
					continue
				}
				fmt.Printf("Executing %s\n", trimmed)
				_, err = sysCheck.db.Exec(trimmed)
				if err != nil {
					fmt.Printf("Error executing \n%s\n %s\n", trimmed, err)
				}
			}
		}
	}
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
