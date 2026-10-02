// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package identity

import (
	"encoding/json"
	"net/netip"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	softwareimpl "github.com/DataDog/datadog-agent/comp/softwareinventory/impl"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

const run = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const capturedUser = "user-12345678901234567890123456789012"

func TestConnectionDNSUsesEndpointScopesAndRunSharedDomains(t *testing.T) {
	localDomain := "domain-11111111111111111111111111111111.invalid"
	remoteDomain := "domain-22222222222222222222222222222222.invalid"
	encoder := model.NewV2DNSEncoder()
	database, offsets, err := encoder.EncodeDomainDatabase([]string{localDomain, remoteDomain})
	if err != nil {
		t.Fatal(err)
	}
	lookups, err := encoder.EncodeMapped(map[string]*model.DNSDatabaseEntry{
		"10.1.2.3": {NameOffsets: []int32{0}}, "10.4.5.6": {NameOffsets: []int32{1}},
	}, offsets)
	if err != nil {
		t.Fatal(err)
	}
	build := func() *telemetry.Sample {
		connections := &model.CollectorConnections{HostName: "capture-host", EncodedDomainDatabase: database, EncodedDnsLookups: lookups}
		for _, connection := range []struct {
			local, remote string
			intra         bool
		}{
			{"10.1.2.3", "10.4.5.6", false}, {"10.4.5.6", "10.1.2.3", false}, {"10.1.2.3", "10.4.5.6", true},
		} {
			connections.Connections = append(connections.Connections, &model.Connection{Laddr: &model.Addr{Ip: connection.local}, Raddr: &model.Addr{Ip: connection.remote}, IntraHost: connection.intra, LastBytesSent: 12345})
		}
		return &telemetry.Sample{Connections: connections}
	}
	first := New(run, 7, "clients", 0)
	second := New(run, 7, "clients", 1)
	otherRun := New("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 7, "clients", 0)
	for _, identity := range []*Map{first, second, otherRun} {
		sample := build()
		if err := identity.Apply(sample, schema.GroupDef{}, nil); err != nil {
			t.Fatal(err)
		}
		for i, connection := range sample.Connections.Connections {
			wantLocal, wantRemote := localDomain, remoteDomain
			if i == 1 {
				wantLocal, wantRemote = remoteDomain, localDomain
			}
			for _, endpoint := range []struct {
				address *model.Addr
				domain  string
			}{{connection.Laddr, wantLocal}, {connection.Raddr, wantRemote}} {
				var names []string
				err := sample.Connections.IterateDNS(endpoint.address, func(_, _ int, name string) bool { names = append(names, name); return true })
				if err != nil || !slices.Equal(names, []string{identity.replaceTokens(endpoint.domain)}) {
					t.Fatal("DNS lookup no longer matches rewritten endpoint")
				}
			}
			if connection.LastBytesSent != 12345 {
				t.Fatal("DNS rewriting changed traffic counters")
			}
		}
		if sample.Connections.Connections[0].Raddr.Ip == sample.Connections.Connections[2].Raddr.Ip {
			t.Fatal("remote and intrahost endpoints shared a scope")
		}
	}
	if first.replaceTokens(remoteDomain) != second.replaceTokens(remoteDomain) || first.replaceTokens(remoteDomain) == otherRun.replaceTokens(remoteDomain) {
		t.Fatal("domains were not shared within one isolated run")
	}
	if err := telemetry.ValidateConnectionDNSV2(database, lookups); err != nil {
		t.Fatal("rewriting mutated captured DNS buffers")
	}
}

func TestConnectionDNSRejectsMalformedLookupBeforeAddressRewrite(t *testing.T) {
	identity := New(run, 7, "clients", 0)
	sample := &telemetry.Sample{Connections: &model.CollectorConnections{EncodedDomainDatabase: []byte{1, 0, 1, 'a'}, EncodedDnsLookups: []byte{2, 1}, Connections: []*model.Connection{{Laddr: &model.Addr{Ip: "10.1.2.3"}, Raddr: &model.Addr{Ip: "10.4.5.6"}}}}}
	if err := identity.Apply(sample, schema.GroupDef{}, nil); err == nil {
		t.Fatal("malformed DNS was silently discarded")
	}
	if sample.Connections.Connections[0].Laddr.Ip != "10.1.2.3" {
		t.Fatal("DNS validation occurred after address rewriting")
	}
}

func TestDeclaredWirelessAssociationIsSharedAndRunScoped(t *testing.T) {
	group := schema.GroupDef{OS: "windows", BSSID: "02:11:22:33:44:55", SSID: "declared-network"}
	for ordinal := range 2 {
		m := New(run, 7, "clients", ordinal)
		sample := fixture()
		if err := m.Apply(sample, group, nil); err != nil {
			t.Fatal(err)
		}
		tags := sample.Metrics[0].Tags.UnsafeToReadOnlySliceString()
		if !slices.Contains(tags, "bssid:"+RadioMAC(run, "", "", group.BSSID)) || !slices.Contains(tags, "ssid:"+SSID(run, group.SSID)) || !slices.Contains(tags, "mac_address:"+m.ClientMAC) {
			t.Fatal("declared shared network or unique client identity was ignored")
		}
		if slices.Contains(tags, "bssid:"+group.BSSID) || slices.Contains(tags, "ssid:"+group.SSID) {
			t.Fatal("literal network identity escaped run isolation")
		}
	}
}

func fixture() *telemetry.Sample {
	return &telemetry.Sample{
		Metrics:      []*metrics.Serie{{Name: "system.wlan.rssi", Host: "capture-host", Points: []metrics.Point{{Ts: 15, Value: -55}}, Tags: tagset.CompositeTagsFromSlice([]string{"bssid:02:00:00:00:00:01", "mac_address:02:00:00:00:00:02", "ssid:captured-network", "scenario:private-scenario", "affected:true", "infra_mode:end_user_device"})}},
		HostMetadata: &telemetry.HostMetadata{Hostname: "capture-host", UUID: "captured-uuid", OS: "win32", HostTags: map[string][]string{"system": {"os_name:windows", "total_memory_gb:16"}}, Gohai: `{"platform":{"hostname":"capture-host","hardware_uuid":"captured-uuid","serial_number":"serial_number-12345678901234567890123456789012"},"network":{"ipaddress":"10.1.2.3","macaddress":"02:00:00:00:00:02"}}`},
		Inventory: &tc.Inventory{Hostname: "capture-host", UUID: "captured-uuid", Host: &tc.HostInventoryMetadata{
			OS: "Windows", AgentVersion: "7.85.0", MemoryTotalKb: 16 << 20, IPAddress: "10.1.2.3", MacAddress: "02:00:00:00:00:02",
		}},
		Processes: &model.CollectorProc{HostName: "capture-host", NetworkId: "captured-network", GroupId: 3, GroupSize: 2, Info: &model.SystemInfo{Uuid: "captured-uuid", TotalMemory: 16 << 30}, Processes: []*model.Process{
			{Pid: 100, NsPid: 100, CreateTime: -5000, User: &model.ProcessUser{Name: capturedUser}, Command: &model.Command{Comm: "SentinelAgent.exe", Exe: `C:\Program Files\SentinelOne\24.1\SentinelAgent.exe`, Args: []string{`C:\Program Files\SentinelOne\24.1\SentinelAgent.exe`}, Ppid: 101, Pgroup: 101}, Cpu: &model.CPUStat{TotalPct: 8}, Memory: &model.MemoryStat{Rss: 64 << 20}},
			{Pid: 101, Command: &model.Command{Comm: "process-12345678901234567890123456789012", Exe: `C:\capture\bin\process-12345678901234567890123456789012`, Cwd: `C:\capture`, Root: `C:\`}},
		}},
		Connections: &model.CollectorConnections{HostName: "capture-host", GroupId: 8, GroupSize: 1, NetworkId: "captured-network", Connections: []*model.Connection{{Pid: 100, Laddr: &model.Addr{Ip: "10.1.2.3", Port: 1234}, Raddr: &model.Addr{Ip: "10.4.5.6", Port: 443}, Rtt: 25000, LastRetransmits: 2}}},
		Software:    &softwareimpl.Payload{Hostname: "capture-host", Metadata: softwareimpl.HostSoftware{Software: []software.Entry{{DisplayName: "SentinelOne", Version: "24.1", ProductCode: "product-12345678901234567890123456789012", UserSID: capturedUser, InstallPaths: []string{`C:\capture\software\path-12345678901234567890123456789012`}}}}},
	}
}

func TestIdentityCorrelatesStreamsAndPreservesEvidence(t *testing.T) {
	m := New(run, 19, "private-cohort", 7)
	s := fixture()
	group := schema.GroupDef{Group: "private-cohort", OS: "windows", Tags: []string{"env:staging"}}
	if err := m.Apply(s, group, nil); err != nil {
		t.Fatal(err)
	}
	for _, hostname := range []string{s.Metrics[0].Host, s.HostMetadata.Hostname, s.Inventory.Hostname, s.Processes.HostName, s.Connections.HostName, s.Software.Hostname} {
		if hostname != m.Hostname {
			t.Fatal("stream host identities diverged")
		}
	}
	if s.Processes.Info.Uuid != m.UUID || s.HostMetadata.UUID != m.UUID || s.Inventory.UUID != m.UUID || s.Processes.NetworkId != m.NetworkID || s.Connections.NetworkId != m.NetworkID || s.HostMetadata.Network["network-id"] != m.NetworkID {
		t.Fatal("host UUID or network identities diverged")
	}
	var gohai map[string]map[string]string
	if err := json.Unmarshal([]byte(s.HostMetadata.Gohai), &gohai); err != nil {
		t.Fatal(err)
	}
	if gohai["platform"]["hardware_uuid"] != m.UUID || gohai["network"]["ipaddress"] != s.Connections.Connections[0].Laddr.Ip || gohai["network"]["macaddress"] != m.ClientMAC {
		t.Fatal("hardware or local network identities diverged")
	}
	if s.Inventory.Host.IPAddress != s.Connections.Connections[0].Laddr.Ip || s.Inventory.Host.MacAddress != m.ClientMAC || s.Inventory.Host.MemoryTotalKb != 16<<20 {
		t.Fatal("host inventory lost cross-stream network identity or hardware evidence")
	}
	p := s.Processes.Processes[0]
	if p.Pid != s.Connections.Connections[0].Pid || p.Pid != p.NsPid || p.Command.Ppid != s.Processes.Processes[1].Pid || p.Command.Pgroup != p.Command.Ppid {
		t.Fatal("process, parent, and connection PID identities diverged")
	}
	if p.User.Name != s.Software.Metadata.Software[0].UserSID || p.User.Name == capturedUser {
		t.Fatal("process and software user identities diverged")
	}
	if p.Command.Exe != fixture().Processes.Processes[0].Command.Exe || p.Command.Comm != "SentinelAgent.exe" || s.Software.Metadata.Software[0].Version != "24.1" || p.Cpu.TotalPct != 8 || p.Memory.Rss != 64<<20 || p.CreateTime != -5000 || s.Metrics[0].Points[0].Ts != 15 || s.Processes.GroupId != 3 || s.Processes.GroupSize != 2 || s.Connections.GroupId != 8 {
		t.Fatal("identity rewriting changed evidence, version paths, cadence or grouping")
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"capture-host", `C:\\capture`, "private-cohort", "private-scenario", "affected:true", capturedUser} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("captured or local-only identity survived: %s", forbidden)
		}
	}
}

func TestDeterministicIdentityIsSafeAcrossConcurrentStreams(t *testing.T) {
	m := New(run, 9, "fleet", 2)
	group := schema.GroupDef{OS: "windows"}
	want := fixture()
	if err := m.Apply(want, group, nil); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := fixture()
			if err := m.Apply(got, group, nil); err != nil {
				t.Error(err)
				return
			}
			if !reflect.DeepEqual(want, got) {
				t.Error("worker ordering changed identity")
			}
		}()
	}
	wg.Wait()
	other := fixture()
	second := New(run, 9, "fleet", 3)
	if err := second.Apply(other, group, nil); err != nil {
		t.Fatal(err)
	}
	if second.Hostname == m.Hostname || second.UUID == m.UUID || second.ClientMAC == m.ClientMAC || want.Processes.Processes[0].Pid == other.Processes.Processes[0].Pid || want.Connections.Connections[0].Laddr.Ip == other.Connections.Connections[0].Laddr.Ip {
		t.Fatal("distinct devices share host identities")
	}
	if want.Connections.Connections[0].Raddr.Ip != other.Connections.Connections[0].Raddr.Ip || want.Processes.Processes[1].Command.Comm != other.Processes.Processes[1].Command.Comm {
		t.Fatal("shared remote endpoints or background application identity diverged")
	}
}

func TestRadioSharingAndRunIsolation(t *testing.T) {
	configured := "AA:BB:CC:DD:EE:FF"
	wireless := &Wireless{BSSID: RadioMAC(run, "ap", "radio", configured), SSID: SSID(run, "Corp-WiFi")}
	if wireless.BSSID != RadioMAC(run, "ap", "radio", "aa-bb-cc-dd-ee-ff") || wireless.BSSID == configured {
		t.Fatal("configured BSSID was not normalized and scoped")
	}
	secondRun := strings.Repeat("b", 32)
	if wireless.BSSID == RadioMAC(secondRun, "ap", "radio", configured) || wireless.SSID == SSID(secondRun, "Corp-WiFi") || Namespace(run) == Namespace(secondRun) {
		t.Fatal("concurrent run resources overlap")
	}
	for ordinal := range 2 {
		s := fixture()
		m := New(run, 2, "fleet", ordinal)
		if err := m.Apply(s, schema.GroupDef{}, wireless); err != nil {
			t.Fatal(err)
		}
		tags := s.Metrics[0].Tags.UnsafeToReadOnlySliceString()
		joined := strings.Join(tags, " ")
		for _, want := range []string{"bssid:" + wireless.BSSID, "ssid:" + wireless.SSID, "mac_address:" + m.ClientMAC, m.RunTag} {
			if !strings.Contains(joined, want) {
				t.Fatalf("client lacks shared radio identity %s", want)
			}
		}
	}
}

func TestIPv6IntraHostAndInvalidIdentity(t *testing.T) {
	s := fixture()
	c := s.Connections.Connections[0]
	c.Laddr.Ip, c.Raddr.Ip, c.IntraHost = "2001:db8::1", "2001:0db8:0::1", true
	m := New(run, 1, "fleet", 1)
	if err := m.Apply(s, schema.GroupDef{}, nil); err != nil {
		t.Fatal(err)
	}
	if c.Laddr.Ip != c.Raddr.Ip || c.Laddr.HostName != c.Raddr.HostName {
		t.Fatal("intra-host connection lost locality")
	}
	if addr, err := netip.ParseAddr(c.Laddr.Ip); err != nil || !addr.Is6() {
		t.Fatal("IPv6 family changed")
	}
	if err := New("scenario-name", 1, "fleet", 1).Apply(fixture(), schema.GroupDef{}, nil); err == nil {
		t.Fatal("nonopaque run identity accepted")
	}
	if err := m.Apply(fixture(), schema.GroupDef{}, &Wireless{BSSID: "invalid", SSID: "ssid"}); err == nil {
		t.Fatal("invalid wireless identity accepted")
	}
}

func TestHardwareAndSoftwareVersionPseudonymsCorrelateWithinRun(t *testing.T) {
	modelToken := "cpu_model-" + strings.Repeat("1", 32)
	vendorToken := "cpu_vendor-" + strings.Repeat("2", 32)
	versionToken := "software_version-" + strings.Repeat("3", 32)
	var previous *telemetry.Sample
	for ordinal := range 2 {
		sample := fixture()
		sample.HostMetadata.Gohai = `{"cpu":{"model_name":"` + modelToken + `","vendor_id":"` + vendorToken + `"}}`
		sample.Inventory.Host.CPUModel, sample.Inventory.Host.CPUVendor = modelToken, vendorToken
		sample.Software.Metadata.Software[0].Version = versionToken
		if err := New(run, 7, "devices", ordinal).Apply(sample, schema.GroupDef{OS: "macos"}, nil); err != nil {
			t.Fatal(err)
		}
		var gohai map[string]map[string]string
		if err := json.Unmarshal([]byte(sample.HostMetadata.Gohai), &gohai); err != nil {
			t.Fatal(err)
		}
		if gohai["cpu"]["model_name"] != sample.Inventory.Host.CPUModel || gohai["cpu"]["vendor_id"] != sample.Inventory.Host.CPUVendor || strings.Contains(sample.HostMetadata.Gohai, modelToken) || sample.Software.Metadata.Software[0].Version == versionToken {
			t.Fatal("hardware/software identities lost correlation or run isolation")
		}
		if previous != nil && (previous.Inventory.Host.CPUModel != sample.Inventory.Host.CPUModel || previous.Software.Metadata.Software[0].Version != sample.Software.Metadata.Software[0].Version) {
			t.Fatal("shared profile fields diverged across devices")
		}
		previous = sample
	}
}
