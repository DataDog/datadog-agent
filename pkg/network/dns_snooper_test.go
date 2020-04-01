// +build linux_bpf

package network

import (
	"math/rand"
	"net"
	"testing"
	"time"

	bpflib "github.com/iovisor/gobpf/elf"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	mdns "github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getSnooper(t *testing.T, m *bpflib.Module, collectStats, collectLocalDNS bool) *SocketFilterSnooper {
	// Load socket filter
	params := map[string]bpflib.SectionParams{
		"socket/dns_filter":      {},
		"maps/conn_stats":        {MapMaxEntries: 1024},
		"maps/tcp_stats":         {MapMaxEntries: 1024},
		"maps/tcp_close_events":  {MapMaxEntries: 1024},
		"maps/port_bindings":     {MapMaxEntries: 1024},
		"maps/udp_port_bindings": {MapMaxEntries: 1024},
	}
	err := m.Load(params)
	require.NoError(t, err)

	filter := m.SocketFilter("socket/dns_filter")
	require.NotNil(t, filter)

	reverseDNS, err := NewSocketFilterSnooper(
		"/proc",
		filter,
		collectStats,
		collectLocalDNS,
	)
	require.NoError(t, err)
	return reverseDNS
}

func checkSnooping(t *testing.T, destIP string, reverseDNS *SocketFilterSnooper) {
	destAddr := util.AddressFromString(destIP)
	srcAddr := util.AddressFromString("127.0.0.1")

	timeout := time.After(1 * time.Second)
Loop:
	// Wait until DNS entry becomes available (with a timeout)
	for {
		select {
		case <-timeout:
			break Loop
		default:
			if reverseDNS.cache.Len() >= 1 {
				break Loop
			}
		}
	}

	// Verify that the IP from the connections above maps to the right name
	payload := []ConnectionStats{{Source: srcAddr, Dest: destAddr}}
	names := reverseDNS.Resolve(payload)
	require.Len(t, names, 1)
	assert.Contains(t, names[destAddr], "golang.org")

	// Verify telemetry
	stats := reverseDNS.GetStats()
	assert.True(t, stats["ips"] >= 1)
	assert.Equal(t, int64(2), stats["lookups"])
	assert.Equal(t, int64(1), stats["resolved"])
}

func TestDNSOverUDPSnooping(t *testing.T) {
	m, err := bytecode.ReadBPFModule(false)
	require.NoError(t, err)
	defer m.Close()

	reverseDNS := getSnooper(t, m, false, false)
	defer reverseDNS.Close()

	// Connect to golang.org. This will result in a DNS lookup which will be captured by SocketFilterSnooper
	conn, err := net.DialTimeout("tcp", "golang.org:80", 1*time.Second)
	require.NoError(t, err)

	// Get destination IP to compare against snooped DNS
	destIP, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	conn.Close()
	require.NoError(t, err)

	checkSnooping(t, destIP, reverseDNS)
}

// Get the preferred outbound IP of this machine
func getOutboundIP(t *testing.T, serverIP string) net.IP {
	conn, err := net.Dial("udp", serverIP+":80")
	require.NoError(t, err)
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP
}

func testDNSOverTCPSnooping(t *testing.T, domain string, shouldSucceed bool) {
	m, err := bytecode.ReadBPFModule(false)
	require.NoError(t, err)
	defer m.Close()

	reverseDNS := getSnooper(t, m, true, false)
	defer reverseDNS.Close()

	// Create a DNS query message
	msg := new(mdns.Msg)
	msg.SetQuestion(mdns.Fqdn(domain), mdns.TypeA)
	msg.RecursionDesired = true

	require.NoError(t, err)
	serverIP := "8.8.8.8"
	dnsHost := net.JoinHostPort(serverIP, "53")

	queryIP := getOutboundIP(t, serverIP).String()
	rand.Seed(time.Now().UnixNano())
	queryPort := rand.Intn(20000) + 10000
	dnsClientAddr := &net.TCPAddr{IP: net.ParseIP(queryIP), Port: queryPort}
	localAddrDialer := &net.Dialer{
		LocalAddr: dnsClientAddr,
	}

	dnsClient := mdns.Client{Net: "tcp", Dialer: localAddrDialer}

	rep, _, _ := dnsClient.Exchange(msg, dnsHost)
	require.NotNil(t, rep)

	if shouldSucceed {
		require.Equal(t, rep.Rcode, mdns.RcodeSuccess)
	} else {
		require.NotEqual(t, rep.Rcode, mdns.RcodeSuccess)
	}

	for _, r := range rep.Answer {
		aRecord, ok := r.(*mdns.A)
		require.True(t, ok)
		require.True(t, mdns.NumField(aRecord) >= 1)
		destIP := mdns.Field(aRecord, 1)
		checkSnooping(t, destIP, reverseDNS)
	}

	key := dnsKey{
		clientPort: uint16(queryPort),
		clientIP:   util.AddressFromString(queryIP),
		serverIP:   util.AddressFromString(serverIP),
		protocol:   TCP,
	}

	var allStats = map[dnsKey]dnsStats{}

	timeout := time.After(1 * time.Second)
Loop:
	// Wait until DNS stats becomes available
	for {
		select {
		case <-timeout:
			break Loop
		default:
			allStats = reverseDNS.GetDNSStats()
			if len(allStats) >= 1 {
				break Loop
			}
		}
	}

	require.Equal(t, 1, len(allStats))

	if shouldSucceed {
		assert.Equal(t, uint32(1), allStats[key].successfulResponses)
		assert.Equal(t, uint32(0), allStats[key].failedResponses)
	} else {
		assert.Equal(t, uint32(0), allStats[key].successfulResponses)
		assert.Equal(t, uint32(1), allStats[key].failedResponses)
	}
}

func TestDNSOverTCPSnoopingWithSuccessfulResposne(t *testing.T) {
	testDNSOverTCPSnooping(t, "golang.org", true)
}

func TestDNSOverTCPSnoopingWithFailedResponse(t *testing.T) {
	testDNSOverTCPSnooping(t, "abcdefghi", false)
}

func TestParsingError(t *testing.T) {
	m, err := bytecode.ReadBPFModule(false)
	require.NoError(t, err)
	defer m.Close()

	reverseDNS := getSnooper(t, m, false, false)
	defer reverseDNS.Close()

	// Pass a byte array of size 1 which should result in parsing error
	reverseDNS.processPacket(make([]byte, 1))
	stats := reverseDNS.GetStats()
	assert.True(t, stats["ips"] == 0)
	assert.True(t, stats["decoding_errors"] == 1)
}
