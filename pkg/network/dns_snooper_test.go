// +build linux_bpf

package network

import (
	"net"
	"testing"
	"time"

	bpflib "github.com/iovisor/gobpf/elf"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	mdns "github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getSnooper(t *testing.T, m *bpflib.Module) *SocketFilterSnooper {
	// Load socket filter
	cfg := NewDefaultConfig()
	err := m.Load(SectionsFromConfig(cfg, true))
	require.NoError(t, err)

	filter := m.SocketFilter("socket/dns_filter")
	require.NotNil(t, filter)

	reverseDNS, err := NewSocketFilterSnooper("/proc", filter)
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

func TestDNSOverTCPSnooping(t *testing.T) {
	m, err := readBPFModule(false)
	require.NoError(t, err)
	defer m.Close()

	reverseDNS := getSnooper(t, m)
	defer reverseDNS.Close()

	// Create a DNS query message
	msg := new(mdns.Msg)
	msg.SetQuestion(mdns.Fqdn("golang.org"), mdns.TypeA)
	msg.RecursionDesired = true

	config, err := mdns.ClientConfigFromFile("/etc/resolv.conf")
	require.NoError(t, err)
	dnsHost := net.JoinHostPort(config.Servers[0], config.Port)

	dnsClient := mdns.Client{Net: "tcp"}
	rep, _, _ := dnsClient.Exchange(msg, dnsHost)
	require.NotNil(t, rep)
	require.Equal(t, rep.Rcode, mdns.RcodeSuccess)

	for _, r := range rep.Answer {
		aRecord, ok := r.(*mdns.A)
		require.True(t, ok)
		require.True(t, mdns.NumField(aRecord) >= 1)
		destIP := mdns.Field(aRecord, 1)
		checkSnooping(t, destIP, reverseDNS)
	}
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
