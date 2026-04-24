// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

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

	// Wait for the Oracle XE service to be fully registered with the TNS
	// listener before running tests. In CI the Oracle sidecar container may
	// report as "ready" (listener accepting TCP connections) before the XE
	// database service is registered, causing ORA-12514 errors.
	fmt.Println("Waiting for Oracle to be ready...")
	var sysCheck Check
	timeout := 5 * time.Minute
	start := time.Now()
	for {
		sysCheck, _ = newSysCheck(nil, "", "")
		if err := sysCheck.Run(); err != nil {
			sysCheck.Teardown()
			if time.Since(start) > timeout {
				fmt.Printf("Oracle failed to become ready within %s: %s\n", timeout, err)
				dumpOracleDiagnostics()
				os.Exit(1)
			}
			fmt.Printf("Oracle not ready yet (%s elapsed): %s\n", time.Since(start).Round(time.Second), err)
			time.Sleep(5 * time.Second)
			continue
		}
		if _, err := sysCheck.db.Exec("SELECT 1 FROM dual"); err != nil {
			sysCheck.Teardown()
			if time.Since(start) > timeout {
				fmt.Printf("Oracle failed to become ready within %s: %s\n", timeout, err)
				dumpOracleDiagnostics()
				os.Exit(1)
			}
			fmt.Printf("Oracle not ready yet (%s elapsed): %s\n", time.Since(start).Round(time.Second), err)
			time.Sleep(5 * time.Second)
			continue
		}
		break
	}
	fmt.Printf("Oracle is ready after %s\n", time.Since(start).Round(time.Second))

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

func dumpOracleDiagnostics() {
	server := os.Getenv("ORACLE_TEST_SERVER")
	if server == "" {
		server = "oracle"
	}

	fmt.Println("=== Oracle Diagnostics ===")

	// Check TCP connectivity to listener
	conn, err := net.DialTimeout("tcp", server+":1521", 5*time.Second)
	if err != nil {
		fmt.Printf("TCP connect to %s:1521: FAILED: %s\n", server, err)
	} else {
		fmt.Printf("TCP connect to %s:1521: OK\n", server)
		conn.Close()
	}

	// DNS resolution
	addrs, err := net.LookupHost(server)
	if err != nil {
		fmt.Printf("DNS lookup %s: FAILED: %s\n", server, err)
	} else {
		fmt.Printf("DNS lookup %s: %v\n", server, addrs)
	}

	// /dev/shm usage — if Oracle allocated SGA, this should be non-zero.
	// 0% used means Oracle never got to SGA allocation.
	if out, err := exec.Command("df", "-h", "/dev/shm").CombinedOutput(); err == nil {
		fmt.Printf("/dev/shm:\n%s", out)
	}
	if out, err := exec.Command("ls", "-la", "/dev/shm/").CombinedOutput(); err == nil {
		fmt.Printf("/dev/shm contents:\n%s", out)
	}

	// cgroup version + memory stats — key theory: on cgroup v2, tmpfs is charged
	// against the container's memory limit. Oracle auto-sizes SGA to ~60-75% of
	// detected memory. If SGA + process RSS exceeds the cgroup limit, mmap fails
	// or OOM kills Oracle, causing it to hang at "Starting Oracle Database instance XE".
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		fmt.Println("cgroup: v2")
		for _, f := range []string{"memory.max", "memory.current", "memory.swap.current", "memory.peak"} {
			if out, err := os.ReadFile("/sys/fs/cgroup/" + f); err == nil {
				fmt.Printf("  %s: %s", f, out)
			}
		}
		// memory.events shows oom/oom_kill counts — critical to confirm OOM theory
		if out, err := os.ReadFile("/sys/fs/cgroup/memory.events"); err == nil {
			fmt.Printf("  memory.events:\n%s", out)
		}
	} else {
		fmt.Println("cgroup: v1")
		if out, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
			fmt.Printf("  memory.limit_in_bytes: %s", out)
		}
		if out, err := os.ReadFile("/sys/fs/cgroup/memory/memory.usage_in_bytes"); err == nil {
			fmt.Printf("  memory.usage_in_bytes: %s", out)
		}
	}

	// Check sibling container cgroups — Oracle's container will be a sibling
	// cgroup under the pod's cgroup. Try to find and read its memory stats.
	// On cgroup v2, pod cgroup is typically the parent of our cgroup.
	if out, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		fmt.Printf("self cgroup: %s", out)
	}
	// Walk up to parent cgroup and list siblings (other containers in the pod)
	if out, err := exec.Command("sh", "-c", "SELF=$(cat /proc/self/cgroup | head -1 | cut -d: -f3); PARENT=$(dirname /sys/fs/cgroup$SELF); echo \"Pod cgroup: $PARENT\"; for d in $PARENT/*/; do echo \"--- $d\"; cat $d/memory.max $d/memory.current $d/memory.events 2>/dev/null | head -20; done").CombinedOutput(); err == nil {
		fmt.Printf("Pod cgroup siblings:\n%s", out)
	}

	// Check if Oracle env vars are visible in this (build) container
	for _, key := range []string{"INIT_SGA_SIZE", "INIT_PGA_SIZE", "ORACLE_PWD", "ALLOCATED_MEMORY", "CI_DEBUG_SERVICES"} {
		val := os.Getenv(key)
		if val == "" {
			fmt.Printf("env %s: <not set>\n", key)
		} else if key == "ORACLE_PWD" {
			fmt.Printf("env %s: <set>\n", key)
		} else {
			fmt.Printf("env %s: %s\n", key, val)
		}
	}

	fmt.Println("=== End Diagnostics ===")
}
