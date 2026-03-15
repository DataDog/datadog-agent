// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usm

import (
	"fmt"
	"strings"
	"testing"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// httpServerScript is a minimal HTTP server using only the socket module.
// It supports keep-alive connections and is used on both Linux and Windows.
const httpServerScript = `import socket, sys
port = int(sys.argv[1])
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(("0.0.0.0", port))
s.listen(8192)
while True:
    conn, addr = s.accept()
    conn.recv(4096)
    conn.sendall(b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: keep-alive\r\n\r\nok")
    conn.close()
`

// sendPythonHTTPRequests sends requestsPerPort keep-alive HTTP GET requests to each
// of ports 8081 and 8082, then holds the process alive for 20 seconds so the agent
// has time to capture all connections before they are cleaned up at process exit.
func sendPythonHTTPRequests(host *components.RemoteHost, pythonCmd string, requestsPerPort int) {
	host.MustExecute(fmt.Sprintf(`bash -c 'ulimit -n 16384 && %s -c "
import http.client, time

conns = []
for port in [8081, 8082]:
    for i in range(%d):
        c = http.client.HTTPConnection(\"127.0.0.1\", port)
        c.request(\"GET\", \"/\")
        c.getresponse()
        conns.append(c)

time.sleep(90)
print(\"done\")
"'`, pythonCmd, requestsPerPort))
}

// sendWindowsKeepAliveRequestsToPort opens count keep-alive connections to
// localhost:<port>, holds them open for holdSeconds, then closes them.
func sendWindowsKeepAliveRequestsToPort(host *components.RemoteHost, port, count, holdSeconds int) {
	connLimit := count + 100
	host.MustExecute(fmt.Sprintf(
		`[System.Net.ServicePointManager]::DefaultConnectionLimit = %d; `+
			`$resps = [System.Collections.ArrayList]::new(); `+
			`1..%d | ForEach-Object { `+
			`$r = [System.Net.HttpWebRequest]::Create("http://localhost:%d/"); `+
			`$r.KeepAlive = $true; `+
			`$r.ConnectionGroupName = [guid]::NewGuid().ToString(); `+
			`[void]$resps.Add($r.GetResponse()) }; `+
			`Start-Sleep -Seconds %d; `+
			`$resps | ForEach-Object { $_.Close() }`,
		connLimit, count, port, holdSeconds))
}

// fetchAndAssertTaggedConnections waits for the agent to forward connections to
// fakeintake, then asserts that connections on ports 8081/8082 have the expected tags.
func fetchAndAssertTaggedConnections(t *testing.T, fi *fi.Client, label string, minPerPort int) {
	t.Helper()

	time.Sleep(60 * time.Second)

	cnx, err := fi.GetConnections()
	require.NoError(t, err, "GetConnections() error")
	require.NotNil(t, cnx, "GetConnections() returned nil")

	stats := getConnectionStats(t, cnx, "process_context:")
	assertTaggedConnectionsOnPort(t, stats, label, 8081, minPerPort)
	assertTaggedConnectionsOnPort(t, stats, label, 8082, minPerPort)
}

// connectionStats holds the results of counting connections on test ports from FakeIntake.
type connectionStats struct {
	connsByPort      map[int32]int
	untaggedByPort   map[int32]int
	missingByTagPort map[int32]map[string]int
	tagsByPort       map[int32]map[string]bool
}

// getConnectionStats fetches connections from FakeIntake and counts connections
// on ports 8081/8082. For each connection it checks whether all requiredTagPrefixes
// are present in the remote service tags, counting how many connections are missing each.
func getConnectionStats(t *testing.T, cnx *aggregator.ConnectionsAggregator, requiredTagPrefixes ...string) connectionStats {
	t.Helper()
	stats := connectionStats{
		connsByPort:      make(map[int32]int),
		untaggedByPort:   make(map[int32]int),
		missingByTagPort: make(map[int32]map[string]int),
		tagsByPort:       make(map[int32]map[string]bool),
	}

	cnx.ForeachConnection(func(conn *agentmodel.Connection, cc *agentmodel.CollectorConnections, _ string) {
		port := conn.Raddr.Port
		if port != 8081 && port != 8082 {
			return
		}
		stats.connsByPort[port]++
		if stats.missingByTagPort[port] == nil {
			stats.missingByTagPort[port] = make(map[string]int)
		}
		if conn.RemoteServiceTagsIdx < 0 {
			stats.untaggedByPort[port]++
			for _, prefix := range requiredTagPrefixes {
				stats.missingByTagPort[port][prefix]++
			}
			return
		}
		remoteTags := cc.GetTags(int(conn.RemoteServiceTagsIdx))
		if stats.tagsByPort[port] == nil {
			stats.tagsByPort[port] = make(map[string]bool)
		}
		found := make(map[string]bool)
		for _, tag := range remoteTags {
			stats.tagsByPort[port][tag] = true
			for _, prefix := range requiredTagPrefixes {
				if strings.HasPrefix(tag, prefix) {
					found[prefix] = true
				}
			}
		}
		for _, prefix := range requiredTagPrefixes {
			if !found[prefix] {
				stats.missingByTagPort[port][prefix]++
			}
		}
	})

	return stats
}

// assertTaggedConnectionsOnPort asserts that at least minConns connections were captured
// on a specific port, that none are untagged, and that no connections are missing any
// required tag prefix.
func assertTaggedConnectionsOnPort(t *testing.T, stats connectionStats, label string, port int32, minConns int) {
	t.Helper()

	t.Logf("%s: port%d=%d untagged=%d missingByTag=%v tags=%v",
		label, port, stats.connsByPort[port], stats.untaggedByPort[port],
		stats.missingByTagPort[port], stats.tagsByPort[port])

	assert.GreaterOrEqual(t, stats.connsByPort[port], minConns,
		"%s: port %d should have at least %d connections", label, port, minConns)
	assert.Equal(t, 0, stats.untaggedByPort[port],
		"%s: port %d has untagged connections", label, port)
	for prefix, count := range stats.missingByTagPort[port] {
		assert.Equal(t, 0, count,
			"%s: %d/%d connections missing required tag prefix %q", label, count,
			stats.connsByPort[port], prefix)
	}
}
