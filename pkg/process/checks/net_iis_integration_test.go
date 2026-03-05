// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && npm

package checks

import (
	"fmt"
	"io"
	nethttp "net/http"
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/hosttags"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	"github.com/DataDog/datadog-agent/pkg/network/usm"
	iistestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/metadata/parser"
	tracetestutil "github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

// iisSiteConfig holds the configuration for a test IIS site.
type iisSiteConfig struct {
	name    string
	port    int
	content string
}

// TestRemoteServiceTagsFullFlowIIS is an integration test that exercises the full pipeline:
// IIS ETW events -> IIS tags cache -> driver connections -> batchConnections -> RemoteServiceTags.
//
// It uses the real network Tracer to capture connections from the Windows kernel driver,
// paired with real IIS ETW cache entries — exactly as the agent does in production.
// The Tracer manages the driver, USM monitor (ETW consumer), and connection state.
//
// Requests are sent in two phases to work within the 1024-entry IIS tags cache limit.
// Phase 1 fills the cache, gets verified, then we wait for the 2-minute TTL to expire
// before phase 2 sends the remaining requests.
//
// This test requires administrator privileges and will install IIS if not already present.
func TestRemoteServiceTagsFullFlowIIS(t *testing.T) {
	// Step 1: Initialize driver and create Tracer with HTTP monitoring
	if err := driver.Init(); err != nil {
		t.Skipf("driver initialization failed (may require admin privileges): %v", err)
	}

	cfg := usm.NewUSMEmptyConfig()
	cfg.EnableHTTPMonitoring = true

	tr, err := tracer.NewTracer(cfg, nil, nil)
	require.NoError(t, err)
	t.Cleanup(tr.Stop)

	// Register client and drain initial state so GetActiveConnections returns only new connections
	const clientID = "iis-integration-test"
	err = tr.RegisterClient(clientID)
	require.NoError(t, err)
	_, drainCleanup, err := tr.GetActiveConnections(clientID)
	require.NoError(t, err)
	drainCleanup()

	// Step 2: Set up two IIS sites on dynamic ports
	const testPath = "/index.html"

	iisManager := iistestutil.NewIISManager(t)
	iisManager.EnsureIISInstalled()

	sites := []iisSiteConfig{
		{name: "DatadogTestSiteA", port: tracetestutil.FreeTCPPort(t), content: "Hello from site A"},
		{name: "DatadogTestSiteB", port: tracetestutil.FreeTCPPort(t), content: "Hello from site B"},
	}

	serverPortSet := make(map[uint16]bool, len(sites))
	for _, site := range sites {
		err := iisManager.SetupIISSite(site.name, site.port, site.content)
		require.NoError(t, err, "failed to set up IIS site %s", site.name)
		serverPortSet[uint16(site.port)] = true
		t.Logf("Set up IIS site %s at http://localhost:%d", site.name, site.port)
		siteName := site.name
		t.Cleanup(func() {
			_ = iisManager.CleanupIISSite(siteName)
		})
	}

	client := &nethttp.Client{
		Timeout: 10 * time.Second,
		Transport: &nethttp.Transport{
			DisableKeepAlives: true, // force unique TCP connection per request
		},
	}

	addrs := make([]string, len(sites))
	for i, site := range sites {
		addrs[i] = fmt.Sprintf("http://localhost:%d%s", site.port, testPath)
	}

	// sendRequests sends n requests per site sequentially in round-robin order.
	sendRequests := func(n int) {
		for i := 0; i < n; i++ {
			for _, addr := range addrs {
				resp, reqErr := client.Get(addr)
				require.NoError(t, reqErr, "request to %s failed", addr)
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				require.Equal(t, 200, resp.StatusCode, "unexpected status from %s", addr)
			}
		}
	}

	// collectAndVerify polls until driver connections align with the IIS cache,
	// then calls batchConnections and verifies all connections get remote service tags.
	// Returns the number of tagged connections and the site names seen.
	collectAndVerify := func(phaseLabel string, expectedTotal int) (int, map[string]bool) {
		var driverConns []*model.Connection
		connSeen := make(map[string]bool)

		require.Eventually(t, func() bool {
			connections, connCleanup, connErr := tr.GetActiveConnections(clientID)
			if connErr != nil {
				t.Logf("%s: GetActiveConnections error: %v", phaseLabel, connErr)
				return false
			}
			for _, cs := range connections.Conns {
				if !cs.IntraHost || !serverPortSet[cs.DPort] {
					continue
				}
				key := fmt.Sprintf("%d-%d", cs.SPort, cs.DPort)
				if connSeen[key] {
					continue
				}
				connSeen[key] = true
				conn := makeConnection(int32(cs.Pid))
				conn.RouteIdx = -1
				conn.IntraHost = true
				conn.Laddr.ContainerId = ""
				conn.Laddr.Port = int32(cs.SPort)
				conn.Raddr.Port = int32(cs.DPort)
				driverConns = append(driverConns, conn)
			}
			connCleanup()

			if len(driverConns) == 0 {
				return false
			}

			iisTags := http.GetIISTagsCache()
			missing := 0
			for _, c := range driverConns {
				iisKey := fmt.Sprintf("%d-%d", c.Raddr.Port, c.Laddr.Port)
				if _, ok := iisTags[iisKey]; !ok {
					missing++
				}
			}
			if missing > 0 {
				t.Logf("%s: %d/%d connections, %d missing from cache (%d entries)",
					phaseLabel, len(driverConns), expectedTotal, missing, len(iisTags))
				return false
			}

			t.Logf("%s: %d connections, all have cache entries (%d entries)",
				phaseLabel, len(driverConns), len(iisTags))
			return true
		}, 60*time.Second, 1*time.Second, "%s: timed out waiting for connections and cache to align", phaseLabel)

		iisTags := http.GetIISTagsCache()
		ex := parser.NewServiceExtractor(false, false, false)
		hostTagsProvider := hosttags.NewHostTagProvider()
		chunks := batchConnections(&HostInfo{}, hostTagsProvider, nil, nil, len(driverConns)+1, 0,
			driverConns, nil, "nid", nil, nil,
			model.KernelHeaderFetchResult_FetchNotAttempted, nil, nil, nil, nil, nil, nil,
			ex, nil, iisTags, nil, nil)

		require.Len(t, chunks, 1)
		cc := chunks[0].(*model.CollectorConnections)

		siteNames := make(map[string]bool)
		var tagged int
		for _, c := range cc.Connections {
			if c.RemoteServiceTagsIdx >= 0 {
				remoteTags := cc.GetTags(int(c.RemoteServiceTagsIdx))
				assertContainsTagPrefix(t, remoteTags, "http.iis.site:")
				assertContainsTagPrefix(t, remoteTags, "http.iis.sitename:")
				assertContainsTagPrefix(t, remoteTags, "http.iis.app_pool:")
				tagged++
				for _, tag := range remoteTags {
					if strings.HasPrefix(tag, "http.iis.sitename:") {
						siteNames[tag] = true
					}
				}
			}
		}

		require.Equal(t, len(driverConns), tagged,
			"%s: expected all %d connections tagged, but only %d were", phaseLabel, len(driverConns), tagged)
		t.Logf("%s: %d/%d connections tagged", phaseLabel, tagged, len(driverConns))
		return tagged, siteNames
	}

	// Step 3: Two-phase approach to work within the 1024-entry IIS tags cache limit.
	// Phase 1: 500 per site = 1000 total (under 1024 cache limit)
	const (
		phase1PerSite = 500
		phase2PerSite = 100
	)

	t.Logf("Phase 1: sending %d requests (%d per site)...", phase1PerSite*len(sites), phase1PerSite)
	sendRequests(phase1PerSite)
	t.Logf("Phase 1: all %d requests completed", phase1PerSite*len(sites))

	tagged1, sites1 := collectAndVerify("Phase 1", phase1PerSite*len(sites))

	// Wait for phase 1 cache entries to expire (TTL = 2 minutes).
	// This frees space in the 1024-entry cache for phase 2.
	// Keep the tracer client alive during the wait by polling periodically,
	// otherwise the client state expires and phase 2 loses connection history.
	t.Logf("Waiting 125s for phase 1 cache entries to expire (TTL=2m)...")
	waitDeadline := time.Now().Add(125 * time.Second)
	for time.Now().Before(waitDeadline) {
		_, keepAliveCleanup, _ := tr.GetActiveConnections(clientID)
		if keepAliveCleanup != nil {
			keepAliveCleanup()
		}
		time.Sleep(10 * time.Second)
	}

	// Phase 2: 100 per site = 200 total
	t.Logf("Phase 2: sending %d requests (%d per site)...", phase2PerSite*len(sites), phase2PerSite)
	sendRequests(phase2PerSite)
	t.Logf("Phase 2: all %d requests completed", phase2PerSite*len(sites))

	tagged2, sites2 := collectAndVerify("Phase 2", phase2PerSite*len(sites))

	// Final summary
	totalTagged := tagged1 + tagged2
	totalExpected := (phase1PerSite + phase2PerSite) * len(sites)
	t.Logf("Total: %d/%d connections tagged across both phases", totalTagged, totalExpected)
	require.Equal(t, totalExpected, totalTagged)

	// Verify both sites appeared in both phases
	allSites := make(map[string]bool)
	for k, v := range sites1 {
		allSites[k] = v
	}
	for k, v := range sites2 {
		allSites[k] = v
	}
	require.Len(t, allSites, len(sites),
		"expected tags from %d distinct IIS sites, got: %v", len(sites), allSites)
	t.Logf("Distinct site names: %v", allSites)
}

// assertContainsTagPrefix asserts that at least one tag in the slice starts
// with the given prefix (e.g., "http.iis.site:").
func assertContainsTagPrefix(t *testing.T, tags []string, prefix string) {
	t.Helper()
	for _, tag := range tags {
		if strings.HasPrefix(tag, prefix) {
			return
		}
	}
	assert.Failf(t, "missing tag prefix", "expected at least one tag with prefix %q in %v", prefix, tags)
}
