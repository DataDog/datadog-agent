// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usm

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

// repoRoot returns the absolute path to the datadog-agent repository root.
func repoRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
}

// deployWindowsBinaries copies locally-built system-probe.exe and process-agent.exe
// to the remote Windows host, replacing the installed versions, then restarts the agent services.
func deployWindowsBinaries(t *testing.T, host *components.RemoteHost) {
	t.Helper()
	root := repoRoot()

	binaries := []struct {
		localPath  string
		remotePath string
	}{
		{
			localPath:  filepath.Join(root, "bin", "system-probe", "system-probe.exe"),
			remotePath: `C:\Program Files\Datadog\Datadog Agent\bin\agent\system-probe.exe`,
		},
		{
			localPath:  filepath.Join(root, "bin", "process-agent", "process-agent.exe"),
			remotePath: `C:\Program Files\Datadog\Datadog Agent\bin\agent\process-agent.exe`,
		},
	}

	host.MustExecute("Stop-Service datadogagent -Force")
	time.Sleep(5 * time.Second)

	for _, bin := range binaries {
		content, err := os.ReadFile(bin.localPath)
		require.NoError(t, err, "failed to read local binary %s", bin.localPath)
		t.Logf("deploying %s (%d bytes) -> %s", bin.localPath, len(content), bin.remotePath)
		_, err = host.WriteFile(bin.remotePath, content)
		require.NoError(t, err, "failed to write binary to %s", bin.remotePath)
	}

	host.MustExecute("Start-Service datadogagent")
	time.Sleep(15 * time.Second)

	out, err := host.Execute("Get-Service datadog-system-probe | Select-Object -ExpandProperty Status")
	t.Logf("system-probe service status: %s (err=%v)", strings.TrimSpace(out), err)
	require.NoError(t, err, "failed to query system-probe service status")
	require.Contains(t, strings.TrimSpace(out), "Running", "system-probe service should be running after deploy")

	out, _ = host.Execute("Get-Service datadog* | Format-Table Name,Status -AutoSize")
	t.Logf("datadog services:\n%s", out)
}

// deployLinuxBinaries copies locally-built system-probe and process-agent
// to the remote Linux host, replacing the installed versions, then restarts the agent services.
func deployLinuxBinaries(t *testing.T, host *components.RemoteHost) {
	t.Helper()
	home, err := os.UserHomeDir()
	require.NoError(t, err, "failed to get user home directory")

	binaries := []struct {
		localPath  string
		remotePath string
	}{
		{
			localPath:  filepath.Join(home, "files", "system-probe"),
			remotePath: "/opt/datadog-agent/embedded/bin/system-probe",
		},
		{
			localPath:  filepath.Join(home, "files", "process-agent"),
			remotePath: "/opt/datadog-agent/embedded/bin/process-agent",
		},
	}

	host.MustExecute("sudo systemctl stop datadog-agent")
	time.Sleep(5 * time.Second)

	for _, bin := range binaries {
		content, err := os.ReadFile(bin.localPath)
		require.NoError(t, err, "failed to read local binary %s", bin.localPath)
		t.Logf("deploying %s (%d bytes) -> %s", bin.localPath, len(content), bin.remotePath)
		tmpPath := "/tmp/" + filepath.Base(bin.remotePath)
		_, err = host.WriteFile(tmpPath, content)
		require.NoError(t, err, "failed to write binary to %s", tmpPath)
		host.MustExecute(fmt.Sprintf("sudo cp %s %s && sudo chmod 755 %s", tmpPath, bin.remotePath, bin.remotePath))
	}

	host.MustExecute("sudo systemctl start datadog-agent")
	// Wait for system-probe eBPF probes and process-agent process table to fully initialize.
	time.Sleep(30 * time.Second)

	out, err := host.Execute("sudo systemctl is-active datadog-agent-sysprobe")
	t.Logf("system-probe service status: %s (err=%v)", strings.TrimSpace(out), err)
	require.NoError(t, err, "failed to query system-probe service status")
	require.Equal(t, "active", strings.TrimSpace(out), "system-probe service should be active after deploy")

	out, _ = host.Execute("sudo systemctl list-units 'datadog*' --no-pager")
	t.Logf("datadog services:\n%s", out)
}

// sendPythonHTTPRequests sends requestsPerPort keep-alive HTTP GET requests to each
// of ports 8081 and 8082, then holds the process alive for 20 seconds so the agent
// has time to capture all connections before they are cleaned up at process exit.
func sendPythonHTTPRequests(host *components.RemoteHost, pythonCmd string, requestsPerPort int) {
	host.MustExecute(fmt.Sprintf(`%s -c "
import http.client, time

conns = []
for port in [8081, 8082]:
    for i in range(%d):
        c = http.client.HTTPConnection('127.0.0.1', port)
        c.request('GET', '/')
        c.getresponse()
        conns.append(c)

time.sleep(20)
print('done')
"`, pythonCmd, requestsPerPort))
}

// sendWindowsKeepAliveRequests opens requestsPerPort keep-alive connections to
// localhost:8081 and localhost:8082, holds them open for holdSeconds, then closes them.
// Windows network driver flushes closed connections, so keep-alive is needed for reliable capture.
func sendWindowsKeepAliveRequests(host *components.RemoteHost, requestsPerPort, holdSeconds int) {
	// DefaultConnectionLimit must exceed total connections (2*requestsPerPort) plus headroom
	// to prevent .NET from queuing requests on shared connections.
	connLimit := requestsPerPort*2 + 100
	host.MustExecute(fmt.Sprintf(
		`[System.Net.ServicePointManager]::DefaultConnectionLimit = %d; `+
			`$resps = [System.Collections.ArrayList]::new(); `+
			`1..%d | ForEach-Object { `+
			`$r = [System.Net.HttpWebRequest]::Create("http://localhost:8081/"); `+
			`$r.KeepAlive = $true; `+
			`$r.ConnectionGroupName = [guid]::NewGuid().ToString(); `+
			`[void]$resps.Add($r.GetResponse()) }; `+
			`1..%d | ForEach-Object { `+
			`$r = [System.Net.HttpWebRequest]::Create("http://localhost:8082/"); `+
			`$r.KeepAlive = $true; `+
			`$r.ConnectionGroupName = [guid]::NewGuid().ToString(); `+
			`[void]$resps.Add($r.GetResponse()) }; `+
			`Start-Sleep -Seconds %d; `+
			`$resps | ForEach-Object { $_.Close() }`,
		connLimit, requestsPerPort, requestsPerPort, holdSeconds))
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
func fetchAndAssertTaggedConnections(t *testing.T, host *components.RemoteHost, fi *fi.Client, label string, minPerPort int) {
	t.Helper()

	// On Linux, query system-probe directly to see how many connections it captured,
	// before waiting for process-agent to forward them to fakeintake.
	if host != nil {
		spOut, err := host.Execute(`sudo curl -s -H 'Accept: application/json' --unix-socket /opt/datadog-agent/run/sysprobe.sock -o /tmp/sp_conns.json http://unix/connections?client_id=e2e-debug && python3 -c "
raw = open('/tmp/sp_conns.json','rb').read()
print(f'response size={len(raw)} first_bytes={raw[:64]!r}')
import json
data = json.loads(raw[raw.index(b'{'):])
conns = data.get('conns', [])
port_counts = {}
for c in conns:
    rport = c.get('raddr', {}).get('port', 0)
    if rport in (8081, 8082):
        port_counts[rport] = port_counts.get(rport, 0) + 1
print(f'system-probe: total_conns={len(conns)} port8081={port_counts.get(8081,0)} port8082={port_counts.get(8082,0)}')
"`)
		if err != nil {
			t.Logf("%s: failed to query system-probe connections: %v", label, err)
		} else {
			t.Logf("%s: %s", label, strings.TrimSpace(spOut))
		}
	}

	time.Sleep(60 * time.Second)

	cnx, err := fi.GetConnections()
	require.NoError(t, err, "GetConnections() error")
	require.NotNil(t, cnx, "GetConnections() returned nil")

	stats := getConnectionStats(t, cnx, "process_context:")
	assertTaggedConnections(t, stats, label, minPerPort)
}

// connectionStats holds the results of counting connections on test ports from FakeIntake.
type connectionStats struct {
	connsByPort    map[int32]int
	untagged       int
	missingByTag   map[string]int
	tagsByPort     map[int32]map[string]bool
}

// getConnectionStats fetches connections from FakeIntake and counts connections
// on ports 8081/8082. For each connection it checks whether all requiredTagPrefixes
// are present in the remote service tags, counting how many connections are missing each.
func getConnectionStats(t *testing.T, cnx *aggregator.ConnectionsAggregator, requiredTagPrefixes ...string) connectionStats {
	t.Helper()
	stats := connectionStats{
		connsByPort:  make(map[int32]int),
		missingByTag: make(map[string]int),
		tagsByPort:   make(map[int32]map[string]bool),
	}

	cnx.ForeachConnection(func(conn *agentmodel.Connection, cc *agentmodel.CollectorConnections, hostname string) {
		if conn.Raddr.Port != 8081 && conn.Raddr.Port != 8082 {
			return
		}
		stats.connsByPort[conn.Raddr.Port]++
		if conn.RemoteServiceTagsIdx < 0 {
			stats.untagged++
			for _, prefix := range requiredTagPrefixes {
				stats.missingByTag[prefix]++
			}
			return
		}
		remoteTags := cc.GetTags(int(conn.RemoteServiceTagsIdx))
		if stats.tagsByPort[conn.Raddr.Port] == nil {
			stats.tagsByPort[conn.Raddr.Port] = make(map[string]bool)
		}
		found := make(map[string]bool)
		for _, tag := range remoteTags {
			stats.tagsByPort[conn.Raddr.Port][tag] = true
			for _, prefix := range requiredTagPrefixes {
				if strings.HasPrefix(tag, prefix) {
					found[prefix] = true
				}
			}
		}
		for _, prefix := range requiredTagPrefixes {
			if !found[prefix] {
				stats.missingByTag[prefix]++
			}
		}
	})

	return stats
}

// assertTaggedConnections asserts that at least minPerPort connections were captured
// on each test port, that none are untagged, and that no connections are missing any
// required tag prefix.
func assertTaggedConnections(t *testing.T, stats connectionStats, label string, minPerPort int) {
	t.Helper()

	t.Logf("%s: port8081=%d port8082=%d untagged=%d missingByTag=%v tags8081=%v tags8082=%v",
		label, stats.connsByPort[8081], stats.connsByPort[8082], stats.untagged,
		stats.missingByTag, stats.tagsByPort[8081], stats.tagsByPort[8082])

	assert.GreaterOrEqual(t, stats.connsByPort[8081], minPerPort,
		"%s: port 8081 should have at least %d connections", label, minPerPort)
	assert.GreaterOrEqual(t, stats.connsByPort[8082], minPerPort,
		"%s: port 8082 should have at least %d connections", label, minPerPort)
	assert.Equal(t, 0, stats.untagged,
		"%s: all connections to test ports should have remote service tags", label)
	// Allow a small number of connections to be missing required tags due to race
	// conditions at agent startup (process context may not be resolved yet for the
	// first few connections after a restart).
	total := stats.connsByPort[8081] + stats.connsByPort[8082]
	maxMissing := total / 100 // allow up to 1% missing
	if maxMissing < 5 {
		maxMissing = 5
	}
	for prefix, count := range stats.missingByTag {
		assert.LessOrEqual(t, count, maxMissing,
			"%s: too many connections (%d/%d) missing required tag prefix %q", label, count, total, prefix)
	}
}

// assertTaggedConnectionsOnPort asserts that at least minConns connections were captured
// on a specific port, that none are untagged, and that no connections are missing any
// required tag prefix.
func assertTaggedConnectionsOnPort(t *testing.T, stats connectionStats, label string, port int32, minConns int) {
	t.Helper()

	t.Logf("%s: port%d=%d untagged=%d missingByTag=%v tags=%v",
		label, port, stats.connsByPort[port], stats.untagged,
		stats.missingByTag, stats.tagsByPort[port])

	assert.GreaterOrEqual(t, stats.connsByPort[port], minConns,
		"%s: port %d should have at least %d connections", label, port, minConns)
	assert.Equal(t, 0, stats.untagged,
		"%s: all connections to test ports should have remote service tags", label)
	total := stats.connsByPort[port]
	maxMissing := total / 100 // allow up to 1% missing
	if maxMissing < 5 {
		maxMissing = 5
	}
	for prefix, count := range stats.missingByTag {
		assert.LessOrEqual(t, count, maxMissing,
			"%s: too many connections (%d/%d) missing required tag prefix %q", label, count, total, prefix)
	}
}