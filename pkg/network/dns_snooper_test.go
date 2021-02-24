// +build linux_bpf

package network

import (
	"math"
	"math/rand"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	filterpkg "github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"github.com/google/gopacket/layers"
	"github.com/miekg/dns"
	mdns "github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func getSnooper(
	t *testing.T,
	buf bytecode.AssetReader,
	collectStats bool,
	collectLocalDNS bool,
	dnsTimeout time.Duration,
	collectDNSDomains bool,
) (*manager.Manager, *SocketFilterSnooper) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	pre410Kernel := currKernelVersion < kernel.VersionCode(4, 1, 0)
	if pre410Kernel {
		t.Skip("DNS feature not available on pre 4.1.0 kernels")
		return nil, nil
	}

	mgr := netebpf.NewManager(ddebpf.NewPerfHandler(1), ddebpf.NewPerfHandler(1), false)
	mgrOptions := manager.Options{
		MapSpecEditors: map[string]manager.MapSpecEditor{
			// These maps are unrelated to DNS but are getting set because the eBPF library loads all of them
			string(probes.ConnMap):            {Type: ebpf.Hash, MaxEntries: 1024, EditorFlag: manager.EditMaxEntries},
			string(probes.TcpStatsMap):        {Type: ebpf.Hash, MaxEntries: 1024, EditorFlag: manager.EditMaxEntries},
			string(probes.PortBindingsMap):    {Type: ebpf.Hash, MaxEntries: 1024, EditorFlag: manager.EditMaxEntries},
			string(probes.UdpPortBindingsMap): {Type: ebpf.Hash, MaxEntries: 1024, EditorFlag: manager.EditMaxEntries},
			string(probes.HttpInFlightMap):    {Type: ebpf.Hash, MaxEntries: 1024, EditorFlag: manager.EditMaxEntries},
		},
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					Section: string(probes.SocketDnsFilter),
				},
			},
		},
	}

	for _, p := range mgr.Probes {
		if p.Section != string(probes.SocketDnsFilter) {
			mgrOptions.ExcludedSections = append(mgrOptions.ExcludedSections, p.Section)
		}
	}

	if collectStats {
		mgrOptions.ConstantEditors = append(mgrOptions.ConstantEditors, manager.ConstantEditor{
			Name:  "dns_stats_enabled",
			Value: uint64(1),
		})
	}
	err = mgr.InitWithOptions(buf, mgrOptions)
	require.NoError(t, err)

	filter, _ := mgr.GetProbe(manager.ProbeIdentificationPair{Section: string(probes.SocketDnsFilter)})
	require.NotNil(t, filter)

	packetSrc, err := filterpkg.NewPacketSource(filter)
	require.NoError(t, err)

	reverseDNS, err := NewSocketFilterSnooper(
		&config.Config{
			Config: ddebpf.Config{
				ProcRoot: "/proc",
			},
			CollectDNSStats:   collectStats,
			CollectLocalDNS:   collectLocalDNS,
			DNSTimeout:        dnsTimeout,
			CollectDNSDomains: collectDNSDomains,
		},
		packetSrc,
	)
	require.NoError(t, err)
	return mgr, reverseDNS
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
	cfg := config.NewDefaultConfig()
	buf, err := netebpf.ReadBPFModule(cfg.BPFDir, false)
	require.NoError(t, err)
	defer buf.Close()

	m, reverseDNS := getSnooper(t, buf, false, false, 15*time.Second, false)
	defer m.Stop(manager.CleanAll)
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
	m, reverseDNS := initDNSTestsWithDomainCollection(t, false)
	defer m.Stop(manager.CleanAll)
	defer reverseDNS.Close()

	_, _, reps := sendDNSQueries(t, []string{"golang.org"}, validDNSServerIP, TCP)
	rep := reps[0]
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

// Get the preferred outbound IP of this machine
func getOutboundIP(t *testing.T, serverIP string) net.IP {
	if parsedIP := net.ParseIP(serverIP); parsedIP.IsLoopback() {
		return parsedIP
	}
	conn, err := net.Dial("udp", serverIP+":80")
	require.NoError(t, err)
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP
}

const (
	localhost        = "127.0.0.1"
	validDNSServerIP = "8.8.8.8"
)

func initDNSTestsWithDomainCollection(t *testing.T, localDNS bool) (*manager.Manager, *SocketFilterSnooper) {
	return initDNSTests(t, localDNS, true)
}

func initDNSTests(t *testing.T, localDNS bool, collectDomain bool) (*manager.Manager, *SocketFilterSnooper) {
	cfg := config.NewDefaultConfig()
	buf, err := netebpf.ReadBPFModule(cfg.BPFDir, false)
	require.NoError(t, err)
	defer buf.Close()
	return getSnooper(t, buf, true, localDNS, 1*time.Second, collectDomain)
}

func sendDNSQueries(
	t *testing.T,
	domains []string,
	serverIP string,
	protocol ConnectionType,
) (string, int, []*mdns.Msg) {
	// Create a DNS query message
	msg := new(mdns.Msg)
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
	var reps []*mdns.Msg

	if protocol == TCP {
		conn, err := dnsClient.Dial(dnsHost)
		require.NoError(t, err)
		for _, domain := range domains {
			msg.SetQuestion(mdns.Fqdn(domain), mdns.TypeA)
			rep, _, _ := dnsClient.ExchangeWithConn(msg, conn)
			reps = append(reps, rep)
		}
	} else { // UDP
		for _, domain := range domains {
			msg.SetQuestion(mdns.Fqdn(domain), mdns.TypeA)
			rep, _, _ := dnsClient.Exchange(msg, dnsHost)
			reps = append(reps, rep)
		}
	}
	return queryIP, queryPort, reps
}

func getKey(
	qIP string,
	qPort int,
	sIP string,
	protocol ConnectionType,
) DNSKey {
	return DNSKey{
		clientIP:   util.AddressFromString(qIP),
		clientPort: uint16(qPort),
		serverIP:   util.AddressFromString(sIP),
		protocol:   protocol,
	}
}

func hasDomains(stats map[string]DNSStats, domains ...string) bool {
	for _, domain := range domains {
		if _, ok := stats[domain]; !ok {
			return false
		}
	}

	return true
}

func countDNSResponses(stats map[DNSKey]map[string]DNSStats) int {
	total := 0
	for _, statsByDomain := range stats {
		for _, s := range statsByDomain {
			total += int(s.DNSTimeouts)
			for _, count := range s.DNSCountByRcode {
				total += int(count)
			}
		}
	}
	return total
}

func TestDNSOverTCPSuccessfulResponseCountWithoutDomain(t *testing.T) {
	m, reverseDNS := initDNSTests(t, false, false)
	defer m.Stop(manager.CleanAll)
	defer reverseDNS.Close()
	statKeeper := reverseDNS.statKeeper
	domains := []string{
		"golang.org",
		"google.com",
		"acm.org",
	}
	queryIP, queryPort, reps := sendDNSQueries(t, domains, validDNSServerIP, TCP)

	// Check that all the queries succeeded
	for _, rep := range reps {
		require.NotNil(t, rep)
		require.Equal(t, rep.Rcode, mdns.RcodeSuccess)
	}

	key := getKey(queryIP, queryPort, validDNSServerIP, TCP)
	var allStats map[DNSKey]map[string]DNSStats
	require.Eventuallyf(t, func() bool {
		allStats = statKeeper.Snapshot()
		return allStats[key] != nil && countDNSResponses(allStats) >= len(domains)
	}, 3*time.Second, 10*time.Millisecond, "not enough DNS responses")

	// Exactly one rcode (0, success) is expected
	stats := allStats[key][""]
	require.Equal(t, 1, len(stats.DNSCountByRcode))
	assert.Equal(t, uint32(3), stats.DNSCountByRcode[uint32(layers.DNSResponseCodeNoErr)])
	assert.True(t, stats.DNSSuccessLatencySum >= uint64(1))
	assert.Equal(t, uint32(0), stats.DNSTimeouts)
	assert.Equal(t, uint64(0), stats.DNSFailureLatencySum)
}

func TestDNSOverTCPSuccessfulResponseCount(t *testing.T) {
	m, reverseDNS := initDNSTestsWithDomainCollection(t, false)
	defer m.Stop(manager.CleanAll)
	defer reverseDNS.Close()
	statKeeper := reverseDNS.statKeeper
	domains := []string{
		"golang.org",
		"google.com",
		"acm.org",
	}
	queryIP, queryPort, reps := sendDNSQueries(t, domains, validDNSServerIP, TCP)

	// Check that all the queries succeeded
	for _, rep := range reps {
		require.NotNil(t, rep)
		require.Equal(t, rep.Rcode, mdns.RcodeSuccess)
	}

	var allStats map[DNSKey]map[string]DNSStats
	key := getKey(queryIP, queryPort, validDNSServerIP, TCP)
	require.Eventually(t, func() bool {
		allStats = statKeeper.Snapshot()
		return hasDomains(allStats[key], domains...)
	}, 3*time.Second, 10*time.Millisecond, "missing DNS data for domains %+v", domains)

	// Exactly one rcode (0, success) is expected
	for _, d := range domains {
		stats := allStats[key][d]
		require.Equal(t, 1, len(stats.DNSCountByRcode))
		assert.Equal(t, uint32(1), stats.DNSCountByRcode[uint32(layers.DNSResponseCodeNoErr)])
		assert.True(t, stats.DNSSuccessLatencySum >= uint64(1))
		assert.Equal(t, uint32(0), stats.DNSTimeouts)
		assert.Equal(t, uint64(0), stats.DNSFailureLatencySum)
	}
}

type handler struct{}

func (this *handler) ServeDNS(w mdns.ResponseWriter, r *mdns.Msg) {
	msg := mdns.Msg{}
	msg.SetReply(r)
	msg.SetRcode(r, mdns.RcodeServerFailure)
	w.WriteMsg(&msg)
}

func TestDNSFailedResponseCount(t *testing.T) {
	m, reverseDNS := initDNSTestsWithDomainCollection(t, true)
	defer m.Stop(manager.CleanAll)
	defer reverseDNS.Close()
	statKeeper := reverseDNS.statKeeper

	domains := []string{
		"nonexistenent.com.net",
		"aabdgdfsgsdafsdafsad",
	}
	queryIP, queryPort, reps := sendDNSQueries(t, domains, validDNSServerIP, TCP)
	for _, rep := range reps {
		require.NotNil(t, rep)
		require.NotEqual(t, rep.Rcode, mdns.RcodeSuccess) // All the queries should have failed
	}
	key1 := getKey(queryIP, queryPort, validDNSServerIP, TCP)

	// Set up a local DNS server to return SERVFAIL
	localServerAddr := &net.UDPAddr{IP: net.ParseIP(localhost), Port: 53}
	localServer := &mdns.Server{Addr: localServerAddr.String(), Net: "udp"}
	localServer.Handler = &handler{}
	waitLock := sync.Mutex{}
	waitLock.Lock()
	localServer.NotifyStartedFunc = waitLock.Unlock
	defer localServer.Shutdown()

	go func() {
		if err := localServer.ListenAndServe(); err != nil {
			t.Fatalf("Failed to set listener %s\n", err.Error())
		}
	}()
	waitLock.Lock()
	queryIP, queryPort, _ = sendDNSQueries(t, domains, localhost, UDP)
	var allStats map[DNSKey]map[string]DNSStats

	// First check the one sent over TCP. Expected error type: NXDomain
	require.Eventually(t, func() bool {
		allStats = statKeeper.Snapshot()
		return hasDomains(allStats[key1], domains...)
	}, 3*time.Second, 10*time.Millisecond, "missing DNS data for TCP requests")
	for _, d := range domains {
		require.Equal(t, 1, len(allStats[key1][d].DNSCountByRcode))
		assert.Equal(t, uint32(1), allStats[key1][d].DNSCountByRcode[uint32(layers.DNSResponseCodeNXDomain)])
	}

	// Next check the one sent over UDP. Expected error type: ServFail
	key2 := getKey(queryIP, queryPort, localhost, UDP)
	require.Eventually(t, func() bool {
		allStats = statKeeper.Snapshot()
		return hasDomains(allStats[key2], domains...)
	}, 3*time.Second, 10*time.Millisecond, "missing DNS data for UDP requests")
	for _, d := range domains {
		require.Equal(t, 1, len(allStats[key2][d].DNSCountByRcode))
		assert.Equal(t, uint32(1), allStats[key2][d].DNSCountByRcode[uint32(layers.DNSResponseCodeServFail)])
	}
}

func TestDNSOverUDPTimeoutCount(t *testing.T) {
	m, reverseDNS := initDNSTestsWithDomainCollection(t, false)
	defer m.Stop(manager.CleanAll)
	defer reverseDNS.Close()
	statKeeper := reverseDNS.statKeeper

	invalidServerIP := "8.8.8.90"
	domainQueried := "agafsdfsdasdfsd"
	queryIP, queryPort, reps := sendDNSQueries(t, []string{domainQueried}, invalidServerIP, UDP)
	require.Nil(t, reps[0])

	var allStats map[DNSKey]map[string]DNSStats
	key := getKey(queryIP, queryPort, invalidServerIP, UDP)
	require.Eventually(t, func() bool {
		allStats = statKeeper.Snapshot()
		return allStats[key] != nil
	}, 3*time.Second, 10*time.Millisecond, "missing DNS data for key %v", key)
	assert.Equal(t, 0, len(allStats[key][domainQueried].DNSCountByRcode))
	assert.Equal(t, uint32(1), allStats[key][domainQueried].DNSTimeouts)
	assert.Equal(t, uint64(0), allStats[key][domainQueried].DNSSuccessLatencySum)
	assert.Equal(t, uint64(0), allStats[key][domainQueried].DNSFailureLatencySum)
}

func TestDNSOverUDPTimeoutCountWithoutDomain(t *testing.T) {
	m, reverseDNS := initDNSTests(t, false, false)
	defer m.Stop(manager.CleanAll)
	defer reverseDNS.Close()
	statKeeper := reverseDNS.statKeeper

	invalidServerIP := "8.8.8.90"
	domainQueried := "agafsdfsdasdfsd"
	queryIP, queryPort, reps := sendDNSQueries(t, []string{domainQueried}, invalidServerIP, UDP)
	require.Nil(t, reps[0])

	key := getKey(queryIP, queryPort, invalidServerIP, UDP)
	var allStats map[DNSKey]map[string]DNSStats
	require.Eventuallyf(t, func() bool {
		allStats = statKeeper.Snapshot()
		return allStats[key] != nil
	}, 3*time.Second, 10*time.Millisecond, "missing DNS data for key %v", key)

	assert.Equal(t, 0, len(allStats[key][""].DNSCountByRcode))
	assert.Equal(t, uint32(1), allStats[key][""].DNSTimeouts)
	assert.Equal(t, uint64(0), allStats[key][""].DNSSuccessLatencySum)
	assert.Equal(t, uint64(0), allStats[key][""].DNSFailureLatencySum)
}

func TestParsingError(t *testing.T) {
	cfg := config.NewDefaultConfig()
	buf, err := netebpf.ReadBPFModule(cfg.BPFDir, false)
	require.NoError(t, err)
	defer buf.Close()

	m, reverseDNS := getSnooper(t, buf, false, false, 15*time.Second, false)
	defer m.Stop(manager.CleanAll)
	defer reverseDNS.Close()

	// Pass a byte array of size 1 which should result in parsing error
	reverseDNS.processPacket(make([]byte, 1), time.Now())
	stats := reverseDNS.GetStats()
	assert.True(t, stats["ips"] == 0)
	assert.True(t, stats["decoding_errors"] == 1)
}

func TestDNSOverIPv6(t *testing.T) {
	m, reverseDNS := initDNSTestsWithDomainCollection(t, true)
	defer m.Stop(manager.CleanAll)
	defer reverseDNS.Close()
	statKeeper := reverseDNS.statKeeper

	// This DNS server is set up so it always returns a NXDOMAIN answer
	serverIP := net.IPv6loopback.String()
	closeFn := newTestServer(t, serverIP, UDP, nxDomainHandler)
	defer closeFn()

	queryIP, queryPort, reps := sendDNSQueries(t, []string{"nxdomain-123.com"}, serverIP, UDP)
	require.NotNil(t, reps[0])

	key := getKey(queryIP, queryPort, serverIP, UDP)
	var allStats map[DNSKey]map[string]DNSStats
	require.Eventually(t, func() bool {
		allStats = statKeeper.Snapshot()
		return allStats[key] != nil
	}, 3*time.Second, 10*time.Millisecond, "missing DNS data for key %v", key)

	stats := allStats[key]["nxdomain-123.com"]
	assert.Equal(t, 1, len(stats.DNSCountByRcode))
	assert.Equal(t, uint32(1), stats.DNSCountByRcode[uint32(layers.DNSResponseCodeNXDomain)])
}

func newTestServer(t *testing.T, ip string, protocol ConnectionType, handler dns.HandlerFunc) func() {
	addr := net.JoinHostPort(ip, "53")
	net := strings.ToLower(protocol.String())
	srv := &dns.Server{Addr: addr, Net: net, Handler: handler}

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
		return func() {}
	}

	return func() {
		srv.Shutdown() //nolint:errcheck
	}
}

// nxDomainHandler returns a NXDOMAIN response for any query
func nxDomainHandler(w dns.ResponseWriter, r *dns.Msg) {
	answer := new(dns.Msg)
	answer.SetReply(r)
	answer.SetRcode(r, dns.RcodeNameError)
	w.WriteMsg(answer) //nolint:errcheck
}
