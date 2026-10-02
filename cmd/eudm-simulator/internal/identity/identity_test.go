// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package identity

import (
	"bytes"
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

func fixture() *telemetry.Sample {
	return &telemetry.Sample{
		Metrics: []*metrics.Serie{{Name: "system.wlan.rssi", Host: "native-laptop", Device: "en0", Points: []metrics.Point{{Ts: 15, Value: -55}}, Unit: "decibel", SourceTypeName: "system", NoIndex: true,
			Resources: []metrics.Resource{{Type: "host", Name: "native-laptop"}, {Type: "device", Name: "en0"}},
			Tags:      tagset.CompositeTagsFromSlice([]string{"bssid:02:00:00:00:00:01", "mac_address:02:00:00:00:00:02", "ssid:Corp-WiFi", "interface:en0", "scenario:native-label", "unkeyed", "infra_mode:end_user_device"})}},
		HostMetadata: &telemetry.HostMetadata{Hostname: "native-laptop", UUID: "native-uuid", OS: "darwin", Network: map[string]string{"network-id": "native-network"},
			HostTags: map[string][]string{"system": {"os_name:darwin", "total_memory_gb:16"}, "custom": {"owner:alice", "unkeyed"}},
			Gohai:    `{"platform":{"hostname":"native-laptop","hardware_uuid":"native-uuid","serial_number":"native-serial"},"cpu":{"model_name":"Native CPU Model","cache_size":9007199254740993},"network":{"ipaddress":"10.1.2.3","macaddress":"02:00:00:00:00:02"}}`},
		Inventory: &tc.Inventory{Hostname: "native-laptop", UUID: "native-uuid", Host: &tc.HostInventoryMetadata{OS: "Darwin", AgentVersion: "7.85.0", MemoryTotalKb: 16 << 20, IPAddress: "10.1.2.3", MacAddress: "02:00:00:00:00:02", CPUModel: "Native CPU Model", CPUVendor: "Native Vendor",
			Interfaces: `[{"name":"en0","ipv4":["10.1.2.3"],"ipv6":["2001:db8::1"],"macaddress":"02:00:00:00:00:02","mtu":1500}]`}},
		Processes: &model.CollectorProc{HostName: "native-laptop", NetworkId: "native-network", GroupId: 3, GroupSize: 2, Info: &model.SystemInfo{Uuid: "native-uuid", TotalMemory: 16 << 30}, Processes: []*model.Process{
			{Pid: 100, NsPid: 100, CreateTime: -5000, User: &model.ProcessUser{Name: "alice"}, Tags: []string{"team:engineering", "native"}, Command: &model.Command{Comm: "DeveloperApp", Exe: "/Users/alice/Applications/DeveloperApp", Args: []string{"/Users/alice/Applications/DeveloperApp", "--project=/Users/alice/project"}, Cwd: "/Users/alice/project", Root: "/", Ppid: 101, Pgroup: 101}, Cpu: &model.CPUStat{TotalPct: 8}, Memory: &model.MemoryStat{Rss: 64 << 20}},
			{Pid: 101, Command: &model.Command{Comm: "process-12345678901234567890123456789012", Exe: "/capture/bin/legitimate-name", Cwd: "/capture", Root: "/"}},
		}},
		Connections: &model.CollectorConnections{HostName: "native-laptop", GroupId: 8, GroupSize: 1, NetworkId: "native-network", Connections: []*model.Connection{{Pid: 100, Laddr: &model.Addr{Ip: "10.1.2.3", Port: 1234, HostName: "native-laptop"}, Raddr: &model.Addr{Ip: "1.1.1.1", Port: 443, HostName: "resolver.example"}, Rtt: 25000, LastRetransmits: 2}}},
		Software:    &softwareimpl.Payload{Hostname: "native-laptop", Metadata: softwareimpl.HostSoftware{Software: []software.Entry{{DisplayName: "DeveloperApp", Publisher: "Independent Developer", Version: "build custom.42", ProductCode: "com.example.developerapp", UserSID: "alice", InstallDate: "2025-01-02T03:04:05Z", InstallPaths: []string{"/Users/alice/Applications/DeveloperApp"}}}}},
	}
}

func TestIdentityChangesOnlyDeviceIdentity(t *testing.T) {
	baseline, sample := fixture(), fixture()
	m := New(run, 19, "cohort", 7, baseline)
	group := schema.GroupDef{Group: "cohort", OS: "macos", Tags: []string{"env:staging"}}
	if err := m.Apply(sample, group, nil); err != nil {
		t.Fatal(err)
	}
	for _, hostname := range []string{sample.Metrics[0].Host, sample.HostMetadata.Hostname, sample.Inventory.Hostname, sample.Processes.HostName, sample.Connections.HostName, sample.Software.Hostname} {
		if hostname != m.Hostname {
			t.Fatal("cross-stream host identity differs")
		}
	}
	if sample.Processes.Info.Uuid != m.UUID || sample.HostMetadata.UUID != m.UUID || sample.Inventory.UUID != m.UUID {
		t.Fatal("cross-stream UUID differs")
	}
	if sample.Inventory.Host.IPAddress == baseline.Inventory.Host.IPAddress || sample.Inventory.Host.IPAddress != sample.Connections.Connections[0].Laddr.Ip || sample.Inventory.Host.MacAddress != m.ClientMAC {
		t.Fatal("local addresses lost correlation")
	}
	var gohai map[string]map[string]json.RawMessage
	if err := json.Unmarshal([]byte(sample.HostMetadata.Gohai), &gohai); err != nil {
		t.Fatal(err)
	}
	var address string
	if json.Unmarshal(gohai["network"]["ipaddress"], &address) != nil || address != sample.Inventory.Host.IPAddress {
		t.Fatal("legacy metadata IP differs")
	}
	if string(gohai["cpu"]["cache_size"]) != "9007199254740993" || string(gohai["cpu"]["model_name"]) != `"Native CPU Model"` {
		t.Fatal("unrelated metadata was changed")
	}
	if !strings.Contains(sample.Inventory.Host.Interfaces, sample.Inventory.Host.IPAddress) || !strings.Contains(sample.Inventory.Host.Interfaces, `"name":"en0"`) {
		t.Fatal("interface identity or native name was lost")
	}
	p := sample.Processes.Processes[0]
	if p.Pid != sample.Connections.Connections[0].Pid || p.Pid != p.NsPid || p.Command.Ppid != sample.Processes.Processes[1].Pid || p.Command.Pgroup != p.Command.Ppid {
		t.Fatal("PID references diverged")
	}
	if !reflect.DeepEqual(p.User, baseline.Processes.Processes[0].User) || p.Command.Comm != "DeveloperApp" || p.Command.Exe != baseline.Processes.Processes[0].Command.Exe || !slices.Equal(p.Command.Args, baseline.Processes.Processes[0].Command.Args) || p.Command.Cwd != "/Users/alice/project" || p.Command.Root != "/" {
		t.Fatal("process names, users, paths or arguments were redacted")
	}
	if !reflect.DeepEqual(sample.Processes.Processes[1].Command, baseline.Processes.Processes[1].Command) {
		t.Fatal("literal placeholder-like text was rewritten")
	}
	if !reflect.DeepEqual(sample.Software.Metadata, baseline.Software.Metadata) {
		t.Fatal("native application metadata or historical installation date changed")
	}
	if p.Cpu.TotalPct != 8 || p.Memory.Rss != 64<<20 || p.CreateTime != -5000 || sample.Processes.GroupId != 3 || sample.Connections.GroupId != 8 {
		t.Fatal("resource or temporal evidence changed")
	}
	if !reflect.DeepEqual(sample.Connections.Connections[0].Raddr, baseline.Connections.Connections[0].Raddr) || sample.Processes.NetworkId != "native-network" || sample.Connections.NetworkId != "native-network" || sample.HostMetadata.Network["network-id"] != "native-network" {
		t.Fatal("remote endpoint or network attributes changed")
	}
	for _, tag := range baseline.Metrics[0].Tags.UnsafeToReadOnlySliceString() {
		if strings.HasPrefix(tag, "mac_address:") {
			continue
		}
		if !slices.Contains(sample.Metrics[0].Tags.UnsafeToReadOnlySliceString(), tag) {
			t.Fatalf("native tag lost: %s", tag)
		}
	}
	if !slices.Contains(sample.HostMetadata.HostTags["custom"], "owner:alice") || !slices.Contains(sample.HostMetadata.HostTags["custom"], "unkeyed") {
		t.Fatal("non-system host tags lost")
	}
	if sample.Metrics[0].Device != "en0" || sample.Metrics[0].Unit != "decibel" || !sample.Metrics[0].NoIndex || sample.Metrics[0].Resources[0].Name != m.Hostname || sample.Metrics[0].Resources[1].Name != "en0" {
		t.Fatal("metric properties or host resource rewriting differs")
	}
}

func TestLocalIdentityMapIsImmutableAcrossStreamsAndDevices(t *testing.T) {
	baseline := fixture()
	observed := NewBaseline()
	observed.Observe(baseline)
	m := NewWithBaseline(run, 9, "fleet", 2, observed)
	want := fixture()
	if err := m.Apply(want, schema.GroupDef{}, nil); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 24 {
		workers.Go(func() {
			got := fixture()
			if err := m.Apply(got, schema.GroupDef{}, nil); err != nil {
				t.Error(err)
				return
			}
			if !reflect.DeepEqual(want, got) {
				t.Error("concurrent identity mapping differs")
			}
		})
	}
	workers.Wait()
	if !reflect.DeepEqual(baseline, fixture()) {
		t.Fatal("baseline mutated")
	}
	other := fixture()
	second := NewWithBaseline(run, 9, "fleet", 3, observed)
	if err := second.Apply(other, schema.GroupDef{}, nil); err != nil {
		t.Fatal(err)
	}
	if second.Hostname == m.Hostname || second.UUID == m.UUID || want.Connections.Connections[0].Laddr.Ip == other.Connections.Connections[0].Laddr.Ip {
		t.Fatal("device identities overlap")
	}
	if want.Connections.Connections[0].Raddr.Ip != other.Connections.Connections[0].Raddr.Ip || !reflect.DeepEqual(want.Software.Metadata, other.Software.Metadata) {
		t.Fatal("shared services or applications diverged")
	}
}

func TestExpandedDeviceIdentitiesStayConsistent(t *testing.T) {
	sample := fixture()
	sample.Metrics[0].Resources[0].Name = "native-resource-host"
	sample.HostMetadata.Network["public_ipv4"] = "203.0.113.25"
	sample.HostMetadata.Meta = map[string]json.RawMessage{
		"instance-id": json.RawMessage(`"i-native"`),
		"ccrid":       json.RawMessage(`"aws/region/account/i-native"`),
	}
	sample.HostMetadata.Gohai = `{"platform":{"hostname":"native-laptop","hardware_uuid":"native-uuid","serial_number":"native-serial","kernel_version":"Darwin Kernel Version 25.0.0: root:xnu-123.1~1/RELEASE_ARM64"},"filesystem":[{"name":"/dev/disk1","mountpoint":"/Users/alice"}]}`
	sample.Inventory.SystemInfo = &tc.HostSystemInfoMetadata{SerialNumber: "native-serial"}
	h := sample.Inventory.Host
	h.HypervisorGuestUUID, h.DmiProductUUID = "native-uuid", "native-uuid"
	h.CloudProviderHostID, h.CanonicalCloudResourceID = "i-native", "aws/region/account/i-native"
	h.CloudProviderAccountID, h.DmiBoardVendor = "native-account", "Native Vendor"
	sample.Connections.Connections[0].Raddr.Ip = "203.0.113.25"
	m := New(run, 1, "fleet", 0, sample)
	if err := m.Apply(sample, schema.GroupDef{}, nil); err != nil {
		t.Fatal(err)
	}
	if sample.Metrics[0].Resources[0].Name != m.Hostname || h.HypervisorGuestUUID != m.UUID || h.DmiProductUUID != m.UUID {
		t.Fatal("alternate host resource or hardware UUID identity diverged")
	}
	if sample.HostMetadata.Network["public_ipv4"] == "203.0.113.25" || sample.Connections.Connections[0].Raddr.Ip != "203.0.113.25" {
		t.Fatal("advertised local address was not scoped independently of remote destination")
	}
	var instance, resource string
	if json.Unmarshal(sample.HostMetadata.Meta["instance-id"], &instance) != nil || json.Unmarshal(sample.HostMetadata.Meta["ccrid"], &resource) != nil || instance != h.CloudProviderHostID || resource != h.CanonicalCloudResourceID || !strings.HasSuffix(resource, "/"+instance) {
		t.Fatal("cloud device identity diverged across metadata streams")
	}
	if h.CloudProviderAccountID != "native-account" || h.DmiBoardVendor != "Native Vendor" || !strings.Contains(sample.HostMetadata.Gohai, `"kernel_version":"Darwin Kernel Version 25.0.0: root:xnu-123.1~1/RELEASE_ARM64"`) || !strings.Contains(sample.HostMetadata.Gohai, `"mountpoint":"/Users/alice"`) || !strings.Contains(sample.HostMetadata.Gohai, sample.Inventory.SystemInfo.SerialNumber) {
		t.Fatal("native hardware details changed or serial correlation was lost")
	}
	// A canonical resource ID need not be accompanied by a provider host ID.
	singleton := &telemetry.Sample{Inventory: &tc.Inventory{Hostname: "native-laptop", Host: &tc.HostInventoryMetadata{CanonicalCloudResourceID: "aws/region/account/i-native"}}}
	if err := m.Apply(singleton, schema.GroupDef{}, nil); err != nil || singleton.Inventory.Host.CanonicalCloudResourceID != resource {
		t.Fatal("standalone cloud device resource ID was not scoped consistently")
	}
}

func TestConnectionLookupTablesFollowLocalIdentity(t *testing.T) {
	sample := fixture()
	c := sample.Connections
	c.ContainerForPid = map[int32]string{100: "native-container", 999: "other-chunk-container", -1: "sentinel"}
	local := &model.Host{Name: "native-laptop", TagIndex: 7, AllTags: []string{"native:tag"}}
	remote := &model.Host{Name: "resolver.example", TagIndex: 8}
	c.ResolvedHostsByName = map[string]*model.Host{"native-laptop": local, "resolver.example": remote, "unreferenced.example": {Name: "unreferenced.example", TagIndex: 9}}
	sample.Processes.Host = &model.Host{Name: "native-laptop", NumCpus: 8}
	m := New(run, 1, "fleet", 0, sample)
	if err := m.Apply(sample, schema.GroupDef{}, nil); err != nil {
		t.Fatal(err)
	}
	if c.ContainerForPid[c.Connections[0].Pid] != "native-container" || c.ContainerForPid[m.pid(999)] != "other-chunk-container" || c.ContainerForPid[-1] != "sentinel" || len(c.ContainerForPid) != 3 {
		t.Fatal("PID-indexed container associations were lost")
	}
	resolved := c.ResolvedHostsByName[c.Connections[0].Laddr.HostName]
	if resolved == nil || resolved.Name != m.Hostname || resolved.TagIndex != 7 || !slices.Equal(resolved.AllTags, local.AllTags) || c.ResolvedHostsByName[c.Connections[0].Raddr.HostName] != remote || c.ResolvedHostsByName["unreferenced.example"].TagIndex != 9 {
		t.Fatal("local host resolution or unchanged remote entries diverged")
	}
	if _, ok := c.ResolvedHostsByName["native-laptop"]; ok || local.Name != "native-laptop" || sample.Processes.Host.Name != m.Hostname || sample.Processes.Host.NumCpus != 8 {
		t.Fatal("old local resolution survived or original table value was mutated")
	}
}

func TestWirelessNativeAndExplicitTopology(t *testing.T) {
	for _, group := range []schema.GroupDef{{}, {BSSID: "02:11:22:33:44:55", SSID: "Declared WiFi"}} {
		sample := fixture()
		m := New(run, 1, "fleet", 0, fixture())
		if err := m.Apply(sample, group, nil); err != nil {
			t.Fatal(err)
		}
		bssid, ssid := "02:00:00:00:00:01", "Corp-WiFi"
		if group.BSSID != "" {
			bssid, ssid = group.BSSID, group.SSID
		}
		tags := sample.Metrics[0].Tags.UnsafeToReadOnlySliceString()
		if !slices.Contains(tags, "bssid:"+bssid) || !slices.Contains(tags, "ssid:"+ssid) {
			t.Fatal("native or configured wireless attributes changed")
		}
	}
	configured := "AA:BB:CC:DD:EE:FF"
	wireless := &Wireless{BSSID: RadioMAC(run, "ap", "radio", configured), SSID: SSID(run, "Corp-WiFi")}
	if wireless.SSID != "Corp-WiFi" || wireless.BSSID != RadioMAC(run, "ap", "radio", "aa-bb-cc-dd-ee-ff") || wireless.BSSID == RadioMAC(strings.Repeat("b", 32), "ap", "radio", configured) {
		t.Fatal("AP identity isolation changed the SSID or lost scoped BSSID")
	}
	for ordinal := range 2 {
		sample := fixture()
		m := New(run, 1, "fleet", ordinal, sample)
		if err := m.Apply(sample, schema.GroupDef{}, wireless); err != nil {
			t.Fatal(err)
		}
		tags := sample.Metrics[0].Tags.UnsafeToReadOnlySliceString()
		if !slices.Contains(tags, "ssid:Corp-WiFi") || !slices.Contains(tags, "bssid:"+wireless.BSSID) || !slices.Contains(tags, "mac_address:"+m.ClientMAC) {
			t.Fatal("AP client and radio identity differ")
		}
	}
}

func TestLoopbackAndRemoteAddressesRemainNative(t *testing.T) {
	baseline := fixture()
	baseline.Inventory.Host.IPv6Address = "2001:db8::1"
	sample := &telemetry.Sample{Connections: &model.CollectorConnections{HostName: "native-laptop", Connections: []*model.Connection{
		{Laddr: &model.Addr{Ip: "127.0.0.1"}, Raddr: &model.Addr{Ip: "127.0.0.1"}, IntraHost: true},
		{Laddr: &model.Addr{Ip: "::1"}, Raddr: &model.Addr{Ip: "::1"}, IntraHost: true},
		{Laddr: &model.Addr{Ip: "0.0.0.0"}, Raddr: &model.Addr{Ip: "1.1.1.1"}},
		{Laddr: &model.Addr{Ip: "2001:db8::1"}, Raddr: &model.Addr{Ip: "2001:0db8:0::1"}, IntraHost: true},
		{Laddr: &model.Addr{Ip: "10.1.2.3"}, Raddr: &model.Addr{Ip: "10.1.2.3"}},
	}}}
	m := New(run, 1, "fleet", 1, baseline)
	if err := m.Apply(sample, schema.GroupDef{}, nil); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"127.0.0.1", "::1", "0.0.0.0"} {
		if sample.Connections.Connections[i].Laddr.Ip != want {
			t.Fatal("loopback or unspecified address changed")
		}
	}
	intra := sample.Connections.Connections[3]
	if intra.Laddr.Ip != intra.Raddr.Ip {
		t.Fatal("intrahost address correlation lost")
	}
	if addr, err := netip.ParseAddr(intra.Laddr.Ip); err != nil || !addr.Is6() {
		t.Fatal("IPv6 family changed")
	}
	if sample.Connections.Connections[4].Raddr.Ip != "10.1.2.3" {
		t.Fatal("remote address mistaken for local identity")
	}
	if err := New("scenario-name", 1, "fleet", 1).Apply(fixture(), schema.GroupDef{}, nil); err == nil {
		t.Fatal("nonopaque run ID accepted")
	}
}

func TestDNSKeepsAllNamesStatsAndDatabaseOffsets(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			baseline := fixture()
			m := New(run, 1, "fleet", 0, baseline)
			c := baseline.Connections
			c.Domains = []string{"native-query.example"}
			c.Connections[0].DnsStatsByDomain = map[int32]*model.DNSStats{0: {DnsTimeouts: 7}}
			c.Connections[0].DnsStatsByDomainByQueryType = map[int32]*model.DNSStatsByQueryType{0: {DnsStatsByQueryType: map[int32]*model.DNSStats{1: {DnsTimeouts: 9}}}}
			c.Connections[0].DnsStatsByDomainOffsetByQueryType = map[int32]*model.DNSStatsByQueryType{}
			names := []string{"native-laptop.example", "cloudflare-dns.com", "stats-only.example", "unreferenced.example"}
			entries := map[string]*model.DNSEntry{"10.1.2.3": {Names: names[:1]}, "1.1.1.1": {Names: names[1:2]}, "203.0.113.47": {Names: names[3:]}}
			var err error
			if version == 1 {
				c.EncodedDNS, err = model.NewV1DNSEncoder().Encode(entries)
			} else {
				encoder := model.NewV2DNSEncoder()
				var offsets []int32
				c.EncodedDomainDatabase, offsets, err = encoder.EncodeDomainDatabase(names)
				if err == nil {
					c.EncodedDnsLookups, err = encoder.EncodeMapped(map[string]*model.DNSDatabaseEntry{"10.1.2.3": {NameOffsets: []int32{0}}, "1.1.1.1": {NameOffsets: []int32{1}}, "203.0.113.47": {NameOffsets: []int32{3}}}, offsets)
				}
				c.Connections[0].DnsStatsByDomainOffsetByQueryType[offsets[2]] = &model.DNSStatsByQueryType{DnsStatsByQueryType: map[int32]*model.DNSStats{1: {DnsTimeouts: 11}}}
			}
			if err != nil {
				t.Fatal(err)
			}
			database := slices.Clone(c.EncodedDomainDatabase)
			stats, _ := json.Marshal(c.Connections[0].DnsStatsByDomainOffsetByQueryType)
			if err := m.Apply(baseline, schema.GroupDef{}, nil); err != nil {
				t.Fatal(err)
			}
			for address, entry := range entries {
				if address == "10.1.2.3" {
					address = m.localIP(address)
				}
				var actual []string
				if err := c.IterateDNS(&model.Addr{Ip: address}, func(_, _ int, name string) bool { actual = append(actual, name); return true }); err != nil || !slices.Equal(actual, entry.Names) {
					t.Fatalf("DNS names lost for %s: %v", address, err)
				}
			}
			after, _ := json.Marshal(c.Connections[0].DnsStatsByDomainOffsetByQueryType)
			if !bytes.Equal(database, c.EncodedDomainDatabase) || !bytes.Equal(stats, after) || !slices.Equal(c.Domains, []string{"native-query.example"}) || c.Connections[0].DnsStatsByDomain[0].DnsTimeouts != 7 || c.Connections[0].DnsStatsByDomainByQueryType[0].DnsStatsByQueryType[1].DnsTimeouts != 9 {
				t.Fatal("DNS tables or query statistics changed")
			}
		})
	}
}

func TestMalformedDNSFailsBeforeEndpointMutation(t *testing.T) {
	sample := fixture()
	sample.Connections.EncodedDomainDatabase = []byte{1, 0, 1, 'a'}
	sample.Connections.EncodedDnsLookups = []byte{2, 1}
	m := New(run, 1, "fleet", 0, sample)
	if err := m.Apply(sample, schema.GroupDef{}, nil); err == nil {
		t.Fatal("invalid DNS accepted")
	}
	if sample.Connections.Connections[0].Laddr.Ip != "10.1.2.3" {
		t.Fatal("address changed before DNS validation")
	}
}
