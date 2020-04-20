// +build linux_bpf

package ebpf

import (
	"math/rand"
	"net"
	"strings"
	"testing"
	"time"

	bpflib "github.com/iovisor/gobpf/elf"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	mdns "github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getSnooper(t *testing.T, m *bpflib.Module) *SocketFilterSnooper {
	cfg := NewDefaultConfig()
	return getSnooperWithConfig(t, m, cfg)
}

func getSnooperWithConfig(t *testing.T, m *bpflib.Module, cfg *Config) *SocketFilterSnooper {
	// Load socket filter
	err := m.Load(SectionsFromConfig(cfg, true))
	require.NoError(t, err)

	filter := m.SocketFilter("socket/dns_filter")
	require.NotNil(t, filter)

	reverseDNS, err := NewSocketFilterSnooper(
		"/proc",
		filter,
		cfg.CollectDNSStats,
		cfg.CollectLocalDNS,
		cfg.DNSTimeout,
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
	m, err := readBPFModule(false)
	require.NoError(t, err)
	defer m.Close()

	reverseDNS := getSnooper(t, m)
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

const (
	validDNSServerIP = "8.8.8.8"
)

func initDNSTests(t *testing.T) (*bpflib.Module, *SocketFilterSnooper) {
	m, err := readBPFModule(false)
	require.NoError(t, err)

	cfg := NewDefaultConfig()
	cfg.CollectDNSStats = true
	cfg.DNSTimeout = 1 * time.Second
	reverseDNS := getSnooperWithConfig(t, m, cfg)
	return m, reverseDNS
}

func sendDNSQuery(
	t *testing.T,
	domain string,
	serverIP string,
	protocol ConnectionType,
) (string, int, *mdns.Msg) {
	// Create a DNS query message
	msg := new(mdns.Msg)
	msg.SetQuestion(mdns.Fqdn(domain), mdns.TypeA)
	msg.RecursionDesired = true
	queryIP := getOutboundIP(t, serverIP).String()

	rand.Seed(time.Now().UnixNano())
	queryPort := rand.Intn(20000) + 10000

	var dnsClientAddr net.Addr
	if protocol == TCP {
		dnsClientAddr = &net.TCPAddr{IP: net.ParseIP(queryIP), Port: queryPort}
	} else {
		dnsClientAddr = &net.UDPAddr{IP: net.ParseIP(queryIP), Port: queryPort}
	}

	localAddrDialer := &net.Dialer{
		LocalAddr: dnsClientAddr,
		Timeout:   5 * time.Second,
	}

	dnsClient := mdns.Client{Net: strings.ToLower(protocol.String()), Dialer: localAddrDialer}

	dnsHost := net.JoinHostPort(serverIP, "53")
	rep, _, _ := dnsClient.Exchange(msg, dnsHost)
	return queryIP, queryPort, rep
}

func getStats(
	qIP string,
	qPort int,
	sIP string,
	snooper *SocketFilterSnooper,
	protocol ConnectionType,
) (dnsKey, map[dnsKey]dnsStats) {
	key := dnsKey{
		clientIP:   util.AddressFromString(qIP),
		clientPort: uint16(qPort),
		serverIP:   util.AddressFromString(sIP),
		protocol:   protocol,
	}

	var allStats = map[dnsKey]dnsStats{}

	timeout := time.After(30 * time.Second)
Loop:
	// Wait until DNS stats becomes available
	for {
		select {
		case <-timeout:
			break Loop
		default:
			allStats = snooper.GetDNSStats()
			if len(allStats) >= 1 {
				break Loop
			}
		}

	}
	return key, allStats
}

func TestDNSOverTCPSnoopingWithSuccessfulResposne(t *testing.T) {
	m, reverseDNS := initDNSTests(t)
	defer m.Close()
	defer reverseDNS.Close()

	queryIP, queryPort, rep := sendDNSQuery(t, "golang.org", validDNSServerIP, TCP)
	require.NotNil(t, rep)
	require.Equal(t, rep.Rcode, mdns.RcodeSuccess)

	for _, r := range rep.Answer {
		aRecord, ok := r.(*mdns.A)
		require.True(t, ok)
		require.True(t, mdns.NumField(aRecord) >= 1)
		destIP := mdns.Field(aRecord, 1)
		checkSnooping(t, destIP, reverseDNS)
	}

	key, allStats := getStats(queryIP, queryPort, validDNSServerIP, reverseDNS, TCP)
	require.Equal(t, 1, len(allStats))
	assert.Equal(t, uint32(1), allStats[key].successfulResponses)
	assert.Equal(t, uint32(0), allStats[key].failedResponses)
	assert.Equal(t, uint32(0), allStats[key].timeouts)
	assert.True(t, allStats[key].successLatency >= uint64(1))
	assert.Equal(t, uint64(0), allStats[key].failureLatency)
}

func TestDNSOverTCPSnoopingWithFailedResponse(t *testing.T) {
	m, reverseDNS := initDNSTests(t)
	defer m.Close()
	defer reverseDNS.Close()

	queryIP, queryPort, rep := sendDNSQuery(t, "agafsdfsdasdfsd", validDNSServerIP, TCP)
	require.NotNil(t, rep)
	require.NotEqual(t, rep.Rcode, mdns.RcodeSuccess)

	key, allStats := getStats(queryIP, queryPort, validDNSServerIP, reverseDNS, TCP)
	require.Equal(t, 1, len(allStats))
	assert.Equal(t, uint32(0), allStats[key].successfulResponses)
	assert.Equal(t, uint32(1), allStats[key].failedResponses)
	assert.Equal(t, uint32(0), allStats[key].timeouts)
	assert.Equal(t, uint64(0), allStats[key].successLatency)
	assert.True(t, allStats[key].failureLatency > uint64(0))
}

func TestDNSOverUDPSnoopingWithTimedOutResponse(t *testing.T) {
	m, reverseDNS := initDNSTests(t)
	defer m.Close()
	defer reverseDNS.Close()

	invalidServerIP := "8.8.8.90"
	queryIP, queryPort, rep := sendDNSQuery(t, "agafsdfsdasdfsd", invalidServerIP, UDP)
	require.Nil(t, rep)

	key, allStats := getStats(queryIP, queryPort, invalidServerIP, reverseDNS, UDP)
	require.Equal(t, 1, len(allStats))
	assert.Equal(t, uint32(0), allStats[key].successfulResponses)
	assert.Equal(t, uint32(0), allStats[key].failedResponses)
	assert.Equal(t, uint32(1), allStats[key].timeouts)
	assert.Equal(t, uint64(0), allStats[key].successLatency)
	assert.Equal(t, uint64(0), allStats[key].failureLatency)
}

func TestParsingError(t *testing.T) {
	m, err := readBPFModule(false)
	require.NoError(t, err)
	defer m.Close()

	reverseDNS := getSnooper(t, m)
	defer reverseDNS.Close()

	// Pass a byte array of size 1 which should result in parsing error
	reverseDNS.processPacket(make([]byte, 1))
	stats := reverseDNS.GetStats()
	assert.True(t, stats["ips"] == 0)
	assert.True(t, stats["decoding_errors"] == 1)
}
