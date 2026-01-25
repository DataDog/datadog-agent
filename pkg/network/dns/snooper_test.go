// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dns

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/google/gopacket/layers"
	mdns "github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/testdns"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func checkSnooping(t *testing.T, destIP string, destName string, reverseDNS *dnsMonitor) {
	destAddr := util.AddressFromString(destIP)
	srcIP := "127.0.0.1"
	srcAddr := util.AddressFromString(srcIP)

	require.Eventually(t, func() bool {
		return reverseDNS.cache.Len() >= 1
	}, 1*time.Second, 10*time.Millisecond)

	// Verify that the IP from the connections above maps to the right name
	payload := map[util.Address]struct{}{srcAddr: {}, destAddr: {}}
	names := reverseDNS.Resolve(payload)
	require.Len(t, names, 1)
	assert.Contains(t, names[destAddr], ToHostname(destName))

	// Verify telemetry
	assert.True(t, cacheTelemetry.length.Load() >= 1)
	lookups := cacheTelemetry.lookups.Load()
	if srcIP != destIP {
		assert.Equal(t, int64(2), lookups)
	} else {
		assert.Equal(t, int64(1), lookups)
	}
	assert.Equal(t, int64(1), cacheTelemetry.resolved.Load())
}

func TestDNSOverUDPSnooping(t *testing.T) {
	cacheTelemetry.length.Set(0)
	cacheTelemetry.lookups.Delete()
	cacheTelemetry.resolved.Delete()
	reverseDNS := initDNSTestsWithDomainCollection(t, false)
	defer reverseDNS.Close()

	// Connect to golang.org. This will result in a DNS lookup which will be captured by socketFilterSnooper
	_, _, reps := testdns.SendDNSQueriesAndCheckError(t, []string{"golang.org"}, testdns.GetServerIP(t), "udp")
	rep := reps[0]
	require.NotNil(t, rep)
	require.Equal(t, rep.Rcode, mdns.RcodeSuccess)

	for _, r := range rep.Answer {
		aRecord, ok := r.(*mdns.A)
		require.True(t, ok)
		require.True(t, mdns.NumField(aRecord) >= 1)
		destIP := mdns.Field(aRecord, 1)
		checkSnooping(t, destIP, "golang.org", reverseDNS)
	}
}

func TestDNSOverTCPSnooping(t *testing.T) {
	cacheTelemetry.length.Set(0)
	cacheTelemetry.lookups.Delete()
	cacheTelemetry.resolved.Delete()
	reverseDNS := initDNSTestsWithDomainCollection(t, false)
	defer reverseDNS.Close()

	_, _, reps := testdns.SendDNSQueriesAndCheckError(t, []string{"golang.org"}, testdns.GetServerIP(t), "tcp")
	rep := reps[0]
	require.NotNil(t, rep)
	require.Equal(t, rep.Rcode, mdns.RcodeSuccess)

	for _, r := range rep.Answer {
		aRecord, ok := r.(*mdns.A)
		require.True(t, ok)
		require.True(t, mdns.NumField(aRecord) >= 1)
		destIP := mdns.Field(aRecord, 1)
		checkSnooping(t, destIP, "golang.org", reverseDNS)
	}
}

const (
	localhost = "127.0.0.1"
)

func initDNSTestsWithDomainCollection(t *testing.T, localDNS bool) *dnsMonitor {
	return initDNSTests(t, localDNS, true)
}

func initDNSTests(t *testing.T, localDNS bool, collectDomain bool) *dnsMonitor {
	cfg := testConfig()
	cfg.CollectDNSStats = true
	cfg.CollectLocalDNS = localDNS
	cfg.DNSTimeout = 1 * time.Second
	cfg.CollectDNSDomains = collectDomain

	rdns, err := NewReverseDNS(cfg, nil)
	require.NoError(t, err)

	err = rdns.Start()
	require.NoError(t, err)

	return rdns.(*dnsMonitor)
}

func getKey(
	qIP string,
	qPort int,
	sIP string,
	protocol uint8,
) Key {
	return Key{
		ClientIP:   util.AddressFromString(qIP),
		ClientPort: uint16(qPort),
		ServerIP:   util.AddressFromString(sIP),
		Protocol:   protocol,
	}
}

func hasDomains(stats map[Hostname]map[QueryType]Stats, domains ...string) bool {
	for _, domain := range domains {
		if _, ok := stats[ToHostname(domain)]; !ok {
			return false
		}
	}

	return true
}

func countDNSResponses(statsByDomain map[Hostname]map[QueryType]Stats) int {
	total := 0
	for _, statsByType := range statsByDomain {
		for _, s := range statsByType {
			total += int(s.Timeouts)
			for _, count := range s.CountByRcode {
				total += int(count)
			}
		}
	}
	return total
}

func TestDNSOverTCPSuccessfulResponseCountWithoutDomain(t *testing.T) {
	reverseDNS := initDNSTests(t, true, false)
	defer reverseDNS.Close()
	statKeeper := reverseDNS.statKeeper
	domains := []string{
		"golang.org",
		"google.com",
		"acm.org",
	}
	queryIP, queryPort, reps := testdns.SendDNSQueriesAndCheckError(t, domains, testdns.GetServerIP(t), "tcp")

	// Check that all the queries succeeded
	for _, rep := range reps {
		require.NotNil(t, rep)
		require.Equal(t, rep.Rcode, mdns.RcodeSuccess)
	}

	key := getKey(queryIP, queryPort, testdns.GetServerIP(t).String(), syscall.IPPROTO_TCP)
	var allStats StatsByKeyByNameByType
	require.Eventuallyf(t, func() bool {
		allStats = statKeeper.Snapshot()
		return allStats[key] != nil && countDNSResponses(allStats[key]) >= len(domains)
	}, 3*time.Second, 10*time.Millisecond, "not enough DNS responses")

	// Exactly one rcode (0, success) is expected
	stats := allStats[key][ToHostname("")][TypeA]
	require.Equal(t, 1, len(stats.CountByRcode))
	assert.Equal(t, uint32(3), stats.CountByRcode[uint32(layers.DNSResponseCodeNoErr)])
	assert.True(t, stats.SuccessLatencySum >= uint64(1))
	assert.Equal(t, uint32(0), stats.Timeouts)
	assert.Equal(t, uint64(0), stats.FailureLatencySum)
}

func TestDNSOverTCPSuccessfulResponseCount(t *testing.T) {
	reverseDNS := initDNSTestsWithDomainCollection(t, true)
	defer reverseDNS.Close()
	statKeeper := reverseDNS.statKeeper
	domains := []string{
		"golang.org",
		"google.com",
		"acm.org",
	}
	serverIP := testdns.GetServerIP(t)
	queryIP, queryPort, reps := testdns.SendDNSQueriesAndCheckError(t, domains, serverIP, "tcp")

	// Check that all the queries succeeded
	for _, rep := range reps {
		require.NotNil(t, rep)
		require.Equal(t, rep.Rcode, mdns.RcodeSuccess)
	}

	var allStats StatsByKeyByNameByType
	key := getKey(queryIP, queryPort, serverIP.String(), syscall.IPPROTO_TCP)
	require.Eventually(t, func() bool {
		allStats = statKeeper.Snapshot()
		return hasDomains(allStats[key], domains...)
	}, 3*time.Second, 10*time.Millisecond, "missing DNS data for domains %+v", domains)

	// Exactly one rcode (0, success) is expected
	for _, d := range domains {
		stats := allStats[key][ToHostname(d)][TypeA]
		require.Equal(t, 1, len(stats.CountByRcode))
		assert.Equal(t, uint32(1), stats.CountByRcode[uint32(layers.DNSResponseCodeNoErr)])
		assert.True(t, stats.SuccessLatencySum >= uint64(1))
		assert.Equal(t, uint32(0), stats.Timeouts)
		assert.Equal(t, uint64(0), stats.FailureLatencySum)
	}
}

func TestDNSFailedResponseCount(t *testing.T) {
	reverseDNS := initDNSTestsWithDomainCollection(t, true)
	defer reverseDNS.Close()
	statKeeper := reverseDNS.statKeeper

	domains := []string{
		"nonexistenent.net.com",
		"missingdomain.com",
	}
	queryIP, queryPort, reps, _ := testdns.SendDNSQueries(domains, testdns.GetServerIP(t), "tcp")
	for _, rep := range reps {
		require.NotNil(t, rep)
		require.Equal(t, rep.Rcode, mdns.RcodeNameError) // All the queries should have failed
	}

	var allStats StatsByKeyByNameByType
	// First check the one sent over TCP. Expected error type: NXDomain
	key1 := getKey(queryIP, queryPort, testdns.GetServerIP(t).String(), syscall.IPPROTO_TCP)
	require.Eventually(t, func() bool {
		allStats = statKeeper.Snapshot()
		return hasDomains(allStats[key1], domains...)
	}, 3*time.Second, 10*time.Millisecond, "missing DNS data for TCP requests")
	for _, d := range domains {
		require.Equal(t, 1, len(allStats[key1][ToHostname(d)][TypeA].CountByRcode))
		assert.Equal(t, uint32(1), allStats[key1][ToHostname(d)][TypeA].CountByRcode[uint32(layers.DNSResponseCodeNXDomain)], "expected one NXDOMAIN for %s, got %v", d, allStats[key1][ToHostname(d)])
	}

	domains = []string{
		"failedserver.com",
		"failedservertoo.com",
	}
	queryIP, queryPort, reps = testdns.SendDNSQueriesAndCheckError(t, domains, testdns.GetServerIP(t), "udp")
	for _, rep := range reps {
		require.NotNil(t, rep)
		require.Equal(t, rep.Rcode, mdns.RcodeServerFailure) // All the queries should have failed
	}

	// Next check the one sent over UDP. Expected error type: ServFail
	key2 := getKey(queryIP, queryPort, testdns.GetServerIP(t).String(), syscall.IPPROTO_UDP)
	require.Eventually(t, func() bool {
		allStats = statKeeper.Snapshot()
		return hasDomains(allStats[key2], domains...)
	}, 3*time.Second, 10*time.Millisecond, "missing DNS data for UDP requests")
	for _, d := range domains {
		require.Equal(t, 1, len(allStats[key2][ToHostname(d)][TypeA].CountByRcode))
		assert.Equal(t, uint32(1), allStats[key2][ToHostname(d)][TypeA].CountByRcode[uint32(layers.DNSResponseCodeServFail)])
	}
}

func TestDNSOverNonPort53(t *testing.T) {
	reverseDNS := initDNSTestsWithDomainCollection(t, true)
	defer reverseDNS.Close()
	statKeeper := reverseDNS.statKeeper

	domains := []string{
		"nonexistent.net.com",
	}
	shutdown, port := newTestServer(t, localhost, "udp")
	defer shutdown()

	queryIP, queryPort, reps, err := testdns.SendDNSQueriesOnPort(domains, net.ParseIP(localhost), strconv.Itoa(int(port)), "udp")
	require.NoError(t, err)
	require.NotNil(t, reps[0])

	// we only pick up on configured DNS ports (default 53), so we shouldn't ever get stats
	key := getKey(queryIP, queryPort, localhost, syscall.IPPROTO_UDP)
	var allStats StatsByKeyByNameByType
	require.Never(t, func() bool {
		allStats = statKeeper.Snapshot()
		return allStats[key] != nil
	}, 3*time.Second, 10*time.Millisecond, "found DNS data for key %v when it should be missing", key)
}

func TestDNSOverCustomPort(t *testing.T) {
	cfg := testConfig()
	cfg.CollectDNSStats = true
	cfg.CollectLocalDNS = true
	cfg.DNSTimeout = 1 * time.Second
	// Add custom port 5353
	cfg.DNSMonitoringPortList = []int{53, 5353}

	rdns, err := NewReverseDNS(cfg, nil)
	require.NoError(t, err)
	err = rdns.Start()
	require.NoError(t, err)
	reverseDNS := rdns.(*dnsMonitor)
	defer reverseDNS.Close()

	statKeeper := reverseDNS.statKeeper
	domains := []string{"golang.org"}
	shutdown, port := newTestServerOnPort(t, localhost, "udp", 5353)
	defer shutdown()

	queryIP, queryPort, reps, err := testdns.SendDNSQueriesOnPort(domains, net.ParseIP(localhost), strconv.Itoa(int(port)), "udp")
	require.NoError(t, err)
	require.NotNil(t, reps[0])

	key := getKey(queryIP, queryPort, localhost, syscall.IPPROTO_UDP)
	var allStats StatsByKeyByNameByType
	require.Eventually(t, func() bool {
		allStats = statKeeper.Snapshot()
		if len(allStats) > 0 {
			fmt.Printf("allStats: %+v\n", allStats)
		}
		return allStats[key] != nil
	}, 3*time.Second, 10*time.Millisecond, "missing DNS data for key %+v", key)
}

func newTestServer(t *testing.T, ip string, protocol string) (func(), uint16) {
	return newTestServerOnPort(t, ip, protocol, 0)
}

func newTestServerOnPort(t *testing.T, ip string, protocol string, port int) (func(), uint16) {
	t.Helper()
	addr := net.JoinHostPort(ip, strconv.Itoa(port))
	srv := &mdns.Server{
		Addr: addr,
		Net:  protocol,
		Handler: mdns.HandlerFunc(func(w mdns.ResponseWriter, r *mdns.Msg) {
			msg := mdns.Msg{}
			msg.SetReply(r)
			msg.SetRcode(r, mdns.RcodeServerFailure)
			_ = w.WriteMsg(&msg)
		}),
	}

	initChan := make(chan error, 1)
	srv.NotifyStartedFunc = func() {
		initChan <- nil
	}
	go func() {
		initChan <- srv.ListenAndServe()
		close(initChan)
	}()

	if err := <-initChan; err != nil {
		t.Errorf("could not initialize DNS server: %s", err)
		return func() {}, uint16(0)
	}

	return func() {
		_ = srv.Shutdown()
	}, uint16(srv.PacketConn.LocalAddr().(*net.UDPAddr).Port)
}

func TestDNSOverUDPTimeoutCount(t *testing.T) {
	reverseDNS := initDNSTestsWithDomainCollection(t, false)
	defer reverseDNS.Close()
	statKeeper := reverseDNS.statKeeper

	invalidServerIP := "8.8.8.90"
	domainQueried := "agafsdfsdasdfsd"
	queryIP, queryPort, reps, err := testdns.SendDNSQueries([]string{domainQueried}, net.ParseIP(invalidServerIP), "udp")
	require.ErrorIs(t, err, os.ErrDeadlineExceeded, "error should be i/o timeout")
	require.Len(t, reps, 1)
	require.Nil(t, reps[0])

	var allStats StatsByKeyByNameByType
	key := getKey(queryIP, queryPort, invalidServerIP, syscall.IPPROTO_UDP)
	require.Eventually(t, func() bool {
		allStats = statKeeper.Snapshot()
		return allStats[key] != nil
	}, 3*time.Second, 10*time.Millisecond, "missing DNS data for key %v", key)
	assert.Equal(t, 0, len(allStats[key][ToHostname(domainQueried)][TypeA].CountByRcode))
	assert.Equal(t, uint32(1), allStats[key][ToHostname(domainQueried)][TypeA].Timeouts)
	assert.Equal(t, uint64(0), allStats[key][ToHostname(domainQueried)][TypeA].SuccessLatencySum)
	assert.Equal(t, uint64(0), allStats[key][ToHostname(domainQueried)][TypeA].FailureLatencySum)
}

func TestDNSOverUDPTimeoutCountWithoutDomain(t *testing.T) {
	reverseDNS := initDNSTests(t, false, false)
	defer reverseDNS.Close()
	statKeeper := reverseDNS.statKeeper

	invalidServerIP := "8.8.8.90"
	domainQueried := "agafsdfsdasdfsd"
	queryIP, queryPort, reps, err := testdns.SendDNSQueries([]string{domainQueried}, net.ParseIP(invalidServerIP), "udp")
	require.ErrorIs(t, err, os.ErrDeadlineExceeded, "error should be i/o timeout")
	require.Len(t, reps, 1)
	require.Nil(t, reps[0])

	key := getKey(queryIP, queryPort, invalidServerIP, syscall.IPPROTO_UDP)
	var allStats StatsByKeyByNameByType
	require.Eventuallyf(t, func() bool {
		allStats = statKeeper.Snapshot()
		return allStats[key] != nil
	}, 3*time.Second, 10*time.Millisecond, "missing DNS data for key %v", key)

	assert.Equal(t, 0, len(allStats[key][ToHostname("")][TypeA].CountByRcode))
	assert.Equal(t, uint32(1), allStats[key][ToHostname("")][TypeA].Timeouts)
	assert.Equal(t, uint64(0), allStats[key][ToHostname("")][TypeA].SuccessLatencySum)
	assert.Equal(t, uint64(0), allStats[key][ToHostname("")][TypeA].FailureLatencySum)
}

func TestParsingError(t *testing.T) {
	cacheTelemetry.length.Set(0)
	snooperTelemetry.decodingErrors.Delete()
	cfg := testConfig()
	cfg.CollectDNSStats = false
	cfg.CollectLocalDNS = false
	cfg.CollectDNSDomains = false
	cfg.DNSTimeout = 15 * time.Second
	rdns, err := NewReverseDNS(cfg, nil)
	require.NoError(t, err)
	err = rdns.Start()
	require.NoError(t, err)
	defer rdns.Close()

	reverseDNS := rdns.(*dnsMonitor)
	// Pass a byte array of size 1 which should result in parsing error
	err = reverseDNS.processPacket(make([]byte, 1), 0, time.Now())
	require.NoError(t, err)
	assert.True(t, cacheTelemetry.length.Load() == 0)
	assert.True(t, snooperTelemetry.decodingErrors.Load() == 1)
}

func TestDNSOverIPv6(t *testing.T) {
	reverseDNS := initDNSTestsWithDomainCollection(t, true)
	defer reverseDNS.Close()
	statKeeper := reverseDNS.statKeeper
	domain := "missingdomain.com"
	serverIP := testdns.GetServerIP(t)

	queryIP, queryPort, reps := testdns.SendDNSQueriesAndCheckError(t, []string{domain}, serverIP, "udp")
	require.NotNil(t, reps[0])

	key := getKey(queryIP, queryPort, serverIP.String(), syscall.IPPROTO_UDP)
	var allStats StatsByKeyByNameByType
	require.Eventually(t, func() bool {
		allStats = statKeeper.Snapshot()
		return allStats[key] != nil
	}, 3*time.Second, 10*time.Millisecond, "missing DNS data for key %v", key)

	stats := allStats[key][ToHostname(domain)][TypeA]
	assert.Equal(t, 1, len(stats.CountByRcode))
	assert.Equal(t, uint32(1), stats.CountByRcode[uint32(layers.DNSResponseCodeNXDomain)])
}

func TestDNSNestedCNAME(t *testing.T) {
	cacheTelemetry.length.Set(0)
	cacheTelemetry.lookups.Delete()
	cacheTelemetry.resolved.Delete()
	reverseDNS := initDNSTestsWithDomainCollection(t, true)
	defer reverseDNS.Close()
	statKeeper := reverseDNS.statKeeper

	domain := "nestedcname.com"

	serverIP := testdns.GetServerIP(t)

	queryIP, queryPort, reps := testdns.SendDNSQueriesAndCheckError(t, []string{domain}, serverIP, "udp")
	require.NotNil(t, reps[0])

	key := getKey(queryIP, queryPort, serverIP.String(), syscall.IPPROTO_UDP)

	var allStats StatsByKeyByNameByType
	require.Eventually(t, func() bool {
		allStats = statKeeper.Snapshot()
		return allStats[key] != nil
	}, 3*time.Second, 10*time.Millisecond, "missing DNS data for key %v", key)

	stats := allStats[key][ToHostname(domain)][TypeA]
	assert.Equal(t, 1, len(stats.CountByRcode))
	assert.Equal(t, uint32(1), stats.CountByRcode[uint32(layers.DNSResponseCodeNoErr)])

	checkSnooping(t, serverIP.String(), domain, reverseDNS)
}

func TestDNSPortReconfiguration(t *testing.T) {
	// First configuration with port 5300
	cfg := testConfig()
	cfg.CollectDNSStats = true
	cfg.CollectLocalDNS = true
	cfg.DNSTimeout = 1 * time.Second
	cfg.CollectDNSDomains = true
	cfg.DNSMonitoringPortList = []int{5300}

	rdns, err := NewReverseDNS(cfg, nil)
	require.NoError(t, err)
	err = rdns.Start()
	require.NoError(t, err)
	reverseDNS := rdns.(*dnsMonitor)

	statKeeper := reverseDNS.statKeeper
	domains := []string{"golang.org"}

	// Start test servers on both ports
	shutdown5300, port5300 := newTestServerOnPort(t, localhost, "udp", 5300)
	defer shutdown5300()
	shutdown5301, port5301 := newTestServerOnPort(t, localhost, "udp", 5301)
	defer shutdown5301()

	// Send queries to port 5300 (should be captured)
	queryIP5300, queryPort5300, reps, err := testdns.SendDNSQueriesOnPort(domains, net.ParseIP(localhost), strconv.Itoa(int(port5300)), "udp")
	require.NoError(t, err)
	require.NotNil(t, reps[0])

	// Send queries to port 5301 (should NOT be captured)
	queryIP5301, queryPort5301, reps, err := testdns.SendDNSQueriesOnPort(domains, net.ParseIP(localhost), strconv.Itoa(int(port5301)), "udp")
	require.NoError(t, err)
	require.NotNil(t, reps[0])

	// Verify port 5300 is captured
	key5300 := getKey(queryIP5300, queryPort5300, localhost, syscall.IPPROTO_UDP)
	require.Eventually(t, func() bool {
		allStats := statKeeper.Snapshot()
		return allStats[key5300] != nil
	}, 3*time.Second, 10*time.Millisecond, "missing DNS data for port 5300")

	// Verify port 5301 is NOT captured
	key5301 := getKey(queryIP5301, queryPort5301, localhost, syscall.IPPROTO_UDP)
	require.Never(t, func() bool {
		allStats := statKeeper.Snapshot()
		return allStats[key5301] != nil
	}, 500*time.Millisecond, 10*time.Millisecond, "found DNS data for port 5301 when it should be missing")

	// Close the first DNS monitor
	reverseDNS.Close()

	// Second configuration with port 5301
	cfg2 := testConfig()
	cfg2.CollectDNSStats = true
	cfg2.CollectLocalDNS = true
	cfg2.DNSTimeout = 1 * time.Second
	cfg2.CollectDNSDomains = true
	cfg2.DNSMonitoringPortList = []int{5301}

	rdns2, err := NewReverseDNS(cfg2, nil)
	require.NoError(t, err)
	err = rdns2.Start()
	require.NoError(t, err)
	reverseDNS2 := rdns2.(*dnsMonitor)
	defer reverseDNS2.Close()

	statKeeper2 := reverseDNS2.statKeeper

	// Send queries to port 5300 (should NOT be captured this time)
	queryIP5300_2, queryPort5300_2, reps, err := testdns.SendDNSQueriesOnPort(domains, net.ParseIP(localhost), strconv.Itoa(int(port5300)), "udp")
	require.NoError(t, err)
	require.NotNil(t, reps[0])

	// Send queries to port 5301 (should be captured this time)
	queryIP5301_2, queryPort5301_2, reps, err := testdns.SendDNSQueriesOnPort(domains, net.ParseIP(localhost), strconv.Itoa(int(port5301)), "udp")
	require.NoError(t, err)
	require.NotNil(t, reps[0])

	// Verify port 5300 is NOT captured
	key5300_2 := getKey(queryIP5300_2, queryPort5300_2, localhost, syscall.IPPROTO_UDP)
	require.Never(t, func() bool {
		allStats := statKeeper2.Snapshot()
		return allStats[key5300_2] != nil
	}, 500*time.Millisecond, 10*time.Millisecond, "found DNS data for port 5300 when it should be missing")

	// Verify port 5301 is captured
	key5301_2 := getKey(queryIP5301_2, queryPort5301_2, localhost, syscall.IPPROTO_UDP)
	require.Eventually(t, func() bool {
		allStats := statKeeper2.Snapshot()
		return allStats[key5301_2] != nil
	}, 3*time.Second, 10*time.Millisecond, "missing DNS data for port 5301")
}

func testConfig() *config.Config {
	return config.New()
}
