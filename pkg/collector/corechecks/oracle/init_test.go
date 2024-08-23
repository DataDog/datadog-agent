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

func TestAgain(t *testing.T) {
	assert.True(t, true)
}

func TestMain(m *testing.M) {
	print("setup")
	// This is a bit of a hack to get a db connection
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

	code := m.Run()
	print("shutdown")
	os.Exit(code)
}
