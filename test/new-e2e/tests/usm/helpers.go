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

// waitForConnectionsPipeline polls fakeintake until at least one connections
// payload arrives, confirming that system-probe and process-agent are working.
// Call this in SetupSuite before sending test traffic to avoid flakiness
// from the agent not being fully initialized yet.
func waitForConnectionsPipeline(t *testing.T, fakeintake *fi.Client) {
	t.Helper()
	require.Eventually(t, func() bool {
		names, err := fakeintake.GetConnectionsNames()
		return err == nil && len(names) > 0
	}, 90*time.Second, 5*time.Second, "no connections data received by fakeintake — connections pipeline may not be running")
}

// waitForHTTPServer polls until the HTTP server on the given port responds.
// checkCmd is the platform-specific command to probe the server (e.g. curl or Invoke-WebRequest).
func waitForHTTPServer(t *testing.T, host *components.RemoteHost, checkCmd string, port int) {
	t.Helper()
	cmd := fmt.Sprintf(checkCmd, port)
	require.Eventually(t, func() bool {
		_, err := host.Execute(cmd)
		return err == nil
	}, 30*time.Second, 2*time.Second, "HTTP server on port %d not responding", port)
}

// httpServerScript is a minimal HTTP/1.1 server built on stdlib
// ThreadingHTTPServer. Threaded accept handles connection bursts in parallel,
// and per-request exceptions are isolated instead of killing the server.
// Used on both Linux and Windows.
const httpServerScript = `import sys
from http.server import ThreadingHTTPServer, BaseHTTPRequestHandler

class H(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    def do_GET(self):
        body = b"ok"
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Connection", "keep-alive")
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *_args, **_kw):
        pass

ThreadingHTTPServer(("0.0.0.0", int(sys.argv[1])), H).serve_forever()
`

// httpListenerScript is a PowerShell HTTP/1.1 server built on
// System.Net.HttpListener. On Windows HttpListener sits on top of HTTP.sys —
// the ETW provider USM hooks for HTTP capture — so connections to this server
// are observable by USM in the same way IIS traffic is.
//
// Usage: powershell.exe -ExecutionPolicy Bypass -File httplistener.ps1 <port>
const httpListenerScript = `param([int]$port)
$listener = New-Object System.Net.HttpListener
$listener.Prefixes.Add("http://+:$port/")
$listener.Start()
while ($listener.IsListening) {
    try {
        $ctx = $listener.GetContext()
        $resp = $ctx.Response
        $bytes = [Text.Encoding]::UTF8.GetBytes("ok")
        $resp.ContentLength64 = $bytes.Length
        $resp.KeepAlive = $true
        $resp.OutputStream.Write($bytes, 0, $bytes.Length)
        $resp.Close()
    } catch {
        # swallow per-connection errors so the listener survives client misbehavior
    }
}
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
        # 10s timeout on connect/read/write — without this, a wedged server
        # (e.g. FD-exhausted accept queue) hangs the script indefinitely
        # and the CI job dies on the 2h limit instead of failing fast.
        c = http.client.HTTPConnection(\"127.0.0.1\", port, timeout=10)
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

// fetchAndAssertTaggedConnections polls fakeintake until connections with the
// expected tags appear on ports 8081/8082, then asserts the results.
func fetchAndAssertTaggedConnections(t *testing.T, fi *fi.Client, label string, portA, portB int32, minPerPort int) {
	t.Helper()

	var stats connectionStats
	require.Eventually(t, func() bool {
		cnx, err := fi.GetConnections()
		if err != nil || cnx == nil {
			return false
		}
		stats = getConnectionStats(t, cnx, []int32{portA, portB}, "process_context:")
		return stats.connsByPort[portA] >= minPerPort && stats.connsByPort[portB] >= minPerPort &&
			stats.untaggedByPort[portA] == 0 && stats.untaggedByPort[portB] == 0
	}, 180*time.Second, 5*time.Second, "%s: timed out waiting for tagged connections on both ports (%d: %d/%d untagged, %d: %d/%d untagged)",
		label, portA, stats.untaggedByPort[portA], stats.connsByPort[portA], portB, stats.untaggedByPort[portB], stats.connsByPort[portB])

	assertTaggedConnectionsOnPort(t, stats, label, portA, minPerPort)
	assertTaggedConnectionsOnPort(t, stats, label, portB, minPerPort)
}

// connectionStats holds the results of counting connections on test ports from FakeIntake.
type connectionStats struct {
	connsByPort      map[int32]int
	untaggedByPort   map[int32]int
	missingByTagPort map[int32]map[string]int
	tagsByPort       map[int32]map[string]bool
}

// getConnectionStats fetches connections from FakeIntake and counts connections
// on the specified ports. For each connection it checks whether all requiredTagPrefixes
// are present in the remote service tags, counting how many connections are missing each.
func getConnectionStats(t *testing.T, cnx *aggregator.ConnectionsAggregator, ports []int32, requiredTagPrefixes ...string) connectionStats {
	t.Helper()
	stats := connectionStats{
		connsByPort:      make(map[int32]int),
		untaggedByPort:   make(map[int32]int),
		missingByTagPort: make(map[int32]map[string]int),
		tagsByPort:       make(map[int32]map[string]bool),
	}

	portSet := make(map[int32]bool, len(ports))
	for _, p := range ports {
		portSet[p] = true
	}

	cnx.ForeachConnection(func(conn *agentmodel.Connection, cc *agentmodel.CollectorConnections, _ string) {
		port := conn.Raddr.Port
		if !portSet[port] {
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
