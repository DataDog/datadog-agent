// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package capture

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

const secret = "UNIQUE-CAPTURE-SECRET-8675309"

type rawMetadata []byte

func (r rawMetadata) MarshalJSON() ([]byte, error) { return r, nil }

func noSecret(t *testing.T, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(secret)) {
		t.Fatal("raw identity survived sanitization")
	}
}

func TestMetricAndHostAllowlists(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	raw := &metrics.Serie{Name: "system.wlan.rssi", Host: secret, Device: secret, Points: []metrics.Point{{Ts: 123, Value: -55}}, Tags: tagset.CompositeTagsFromSlice([]string{"bssid:" + secret, "mac_address:" + secret, "ssid:" + secret, "interface:" + secret, "api_key:" + secret, "unknown:" + secret}), Resources: []metrics.Resource{{Type: secret, Name: secret}}, Unit: secret, SourceTypeName: secret}
	before, _ := json.Marshal(raw)
	clean, err := s.Series(NewSeriesSource([]*metrics.Serie{raw, {Name: secret, Host: secret}}))
	if err != nil {
		t.Fatal(err)
	}
	if !clean.MoveNext() {
		t.Fatal("missing allowlisted metric")
	}
	noSecret(t, clean.Current())
	if clean.MoveNext() {
		t.Fatal("unknown metric persisted")
	}
	after, _ := json.Marshal(raw)
	if !bytes.Equal(before, after) {
		t.Fatal("mutated native metric")
	}
	payload := map[string]any{"apiKey": secret, "uuid": secret, "internalHostname": secret, "os": "darwin", "agentVersion": secret, "systemStats": map[string]any{"cpuCores": 8, "machine": "arm64", "username": secret}, "meta": map[string]any{"socket-hostname": secret, "host_aliases": []string{secret}, "instance-id": secret}, "network": map[string]string{"network-id": secret, "public-ipv4": secret}, "host-tags": map[string][]string{"system": {secret}}, "cloud": secret, "container-meta": map[string]string{"id": secret}, "gohai": `{"cpu":{"cpu_cores":8,"unknown":"` + secret + `"},"platform":{"hostname":"` + secret + `","serial_number":"` + secret + `","hardware_uuid":"` + secret + `"},"network":{"ipaddress":"` + secret + `","macaddress":"` + secret + `","unknown":"` + secret + `"}}`}
	encoded, _ := json.Marshal(payload)
	host, err := s.HostMetadata(rawMetadata(encoded))
	if err != nil {
		t.Fatal(err)
	}
	noSecret(t, host)
	if host.(*HostMetadata).Hostname != "capture-host" {
		t.Fatal("host identity mismatch")
	}
}

func TestProcessConnectionAndSoftwareAllowlists(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	proc := &model.CollectorProc{HostName: secret, NetworkId: secret, Info: &model.SystemInfo{Uuid: secret, Os: &model.OSInfo{Name: "windows", Version: "11.0", KernelVersion: secret}, Cpus: []*model.CPUInfo{{Number: 0, Cores: 8, Vendor: secret, PhysicalId: secret}}}, Processes: []*model.Process{{Pid: 912, ContainerId: secret, ByteKey: []byte(secret), Tags: []string{secret}, Command: &model.Command{Comm: "SentinelAgent.exe", Exe: secret, Args: []string{secret}, Cwd: secret, Root: secret, Ppid: 25}, User: &model.ProcessUser{Name: secret}, Cpu: &model.CPUStat{TotalPct: 2, LastCpu: secret}, Memory: &model.MemoryStat{Rss: 4096}}}}
	before, _ := json.Marshal(proc)
	cleanProc := s.Process(proc)
	noSecret(t, cleanProc)
	if cleanProc.Processes[0].Command.Comm != "SentinelAgent.exe" || cleanProc.Processes[0].Memory.Rss != 4096 {
		t.Fatal("lost scenario baseline values")
	}
	conn := &model.CollectorConnections{HostName: secret, NetworkId: secret, EncodedTags: []byte(secret), EncodedDNS: []byte(secret), Domains: []string{secret}, EcsTask: secret, ResolvedHostsByName: map[string]*model.Host{secret: {}}, Connections: []*model.Connection{{Pid: 912, Laddr: &model.Addr{Ip: secret, ContainerId: secret, HostName: secret}, Raddr: &model.Addr{Ip: secret, Port: 443}, RemoteNetworkId: secret, RemoteEcsTask: secret, HttpAggregations: []byte(secret), DatabaseAggregations: []byte(secret), Rtt: 32000, RttVar: 500, LastRetransmits: 2, TcpFailuresByErrCode: map[uint32]uint32{110: 1}}}}
	cleanConn := s.Connections(conn)
	noSecret(t, cleanConn)
	if cleanConn.Connections[0].Pid != cleanProc.Processes[0].Pid || cleanConn.NetworkId != cleanProc.NetworkId {
		t.Fatal("cross-stream identities diverged")
	}
	if cleanConn.Connections[0].Rtt != 32000 || cleanConn.Connections[0].TcpFailuresByErrCode[110] != 1 {
		t.Fatal("lost captured TCP evidence")
	}
	cleanConn.Connections[0].TcpFailuresByErrCode[110] = 50
	if conn.Connections[0].TcpFailuresByErrCode[110] != 1 {
		t.Fatal("connection maps alias native data")
	}
	entries := []software.Entry{{DisplayName: secret, Version: secret, Publisher: secret, ProductCode: secret, UserSID: secret, InstallDate: secret, Source: secret, Status: secret, BrokenReason: secret, PkgID: secret, InstallPath: secret, InstallSource: secret, InstallPaths: []string{secret}}, {DisplayName: "Google Chrome", Version: "125.0.1", Publisher: "Google LLC", Source: "app", Status: "installed"}}
	cleanSoftware := s.Software(entries)
	noSecret(t, cleanSoftware)
	if len(cleanSoftware) != 2 || cleanSoftware[1].DisplayName != "Google Chrome" || cleanSoftware[1].Version != "125.0.1" {
		t.Fatal("lost full snapshot or application identity")
	}
	after, _ := json.Marshal(proc)
	if !bytes.Equal(before, after) {
		t.Fatal("mutated native process")
	}
}

func TestEveryNewProtobufStringDefaultsToDropped(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	// Set every direct string and byte slice, including future protobuf fields.
	fill := func(ptr any) {
		v := reflect.ValueOf(ptr).Elem()
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if !f.CanSet() {
				continue
			}
			if f.Kind() == reflect.String {
				f.SetString(secret)
			}
			if f.Type() == reflect.TypeFor[[]byte]() {
				f.SetBytes([]byte(secret))
			}
		}
	}
	p, c, cmd, user, info, osInfo, cpu, addr := &model.Process{}, &model.Connection{}, &model.Command{}, &model.ProcessUser{}, &model.SystemInfo{}, &model.OSInfo{}, &model.CPUInfo{}, &model.Addr{}
	for _, value := range []any{p, c, cmd, user, info, osInfo, cpu, addr} {
		fill(value)
	}
	p.Command = cmd
	p.User = user
	info.Os = osInfo
	info.Cpus = []*model.CPUInfo{cpu}
	c.Laddr = addr
	c.Raddr = addr
	noSecret(t, s.Process(&model.CollectorProc{Processes: []*model.Process{p}, Info: info}))
	noSecret(t, s.Connections(&model.CollectorConnections{Connections: []*model.Connection{c}}))
}

func TestNativePlatformMetadataSurvivesSanitization(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		os       string
		stats    string
		platform string
	}{
		{"windows", "win32", `{"cpuCores":8,"machine":"amd64","platform":"windows","winV":["Microsoft Windows 11 Pro","10.0.26100"]}`, `{"kernel_name":"Windows","os":"Windows 11 Pro","family":"Domain Joined Workstation","kernel_release":"10.0.26100","machine":"x86_64"}`},
		{"macos", "darwin", `{"cpuCores":8,"machine":"arm64","platform":"darwin","macV":["15.6",["","",""],"arm64"]}`, `{"kernel_name":"Darwin","os":"Darwin","kernel_release":"24.6.0","machine":"arm64","hardware_platform":"arm64"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stats map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.stats), &stats); err != nil {
				t.Fatal(err)
			}
			raw := &HostMetadata{OS: tc.os, Hostname: secret, SystemStats: stats, Gohai: `{"platform":` + tc.platform + `,"cpu":{"cache_size":"8192 KB","cpu_pkgs":"1"},"memory":{"swap_total":"2048kB"}}`}
			clean, err := s.HostMetadata(raw)
			if err != nil {
				t.Fatal(err)
			}
			noSecret(t, clean)
			got := clean.(*HostMetadata)
			if got.OS != tc.os {
				t.Fatalf("native OS changed: got %q, want %q", got.OS, tc.os)
			}
			for key, value := range stats {
				var want, actual any
				if err := json.Unmarshal(value, &want); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(got.SystemStats[key], &actual); err != nil {
					t.Fatalf("missing or invalid systemStats.%s: %v", key, err)
				}
				if !reflect.DeepEqual(actual, want) {
					t.Fatalf("systemStats.%s changed: got %#v, want %#v", key, actual, want)
				}
			}
			var wantPlatform map[string]any
			var gohai map[string]map[string]any
			if err := json.Unmarshal([]byte(tc.platform), &wantPlatform); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(got.Gohai), &gohai); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gohai["platform"], wantPlatform) || gohai["cpu"]["cache_size"] != "8192 KB" || gohai["memory"]["swap_total"] != "2048kB" {
				t.Fatal("lost typed native platform, CPU or memory evidence")
			}
		})
	}
}

func TestNewPlatformFieldsRejectSensitiveValues(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{secret, "Windows 11 Pro " + secret, "15.6 (" + secret + ")"} {
		encoded, err := json.Marshal(map[string]any{
			"os":          value,
			"systemStats": map[string]any{"platform": value, "machine": value, "macV": []any{value, []string{secret}, value}, "winV": []string{value, value}},
			"gohai":       `{"platform":{"kernel_release":"` + value + `","machine":"` + value + `","hardware_platform":"` + value + `","family":"` + value + `","os":"` + value + `"},"cpu":{"cache_size":"` + value + `","cpu_pkgs":"` + value + `"}}`,
		})
		if err != nil {
			t.Fatal(err)
		}
		host, err := s.HostMetadata(rawMetadata(encoded))
		if err != nil {
			t.Fatal(err)
		}
		noSecret(t, host)
		noSecret(t, s.Process(&model.CollectorProc{Info: &model.SystemInfo{Os: &model.OSInfo{Name: value, Platform: value, Family: value, Version: value, KernelVersion: value}}}))
		noSecret(t, s.Software([]software.Entry{{DisplayName: "OS", Source: "os", Version: value}}))
	}
}

func TestNativeProcessAndOSInventoryVersions(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		platform string
		family   string
		version  string
	}{
		{"Windows", "Windows 11 Pro", "Standalone Workstation", "10.0.26100"},
		{"darwin", "macOS", "", "15.6"},
	} {
		p := s.Process(&model.CollectorProc{Info: &model.SystemInfo{Os: &model.OSInfo{Name: tc.name, Platform: tc.platform, Family: tc.family, Version: tc.version}}})
		if p.Info.Os.Platform != tc.platform || p.Info.Os.Family != tc.family || p.Info.Os.Version != tc.version || p.Info.Os.Name == "" {
			t.Fatalf("lost process platform evidence: %+v", p.Info.Os)
		}
	}
	entries := []software.Entry{{DisplayName: "OS", Source: "os", Version: "15.6 (24G84)"}, {DisplayName: "OS", Source: "os", Version: "26.0 (25A5306g)"}, {DisplayName: "OS", Source: "os", Version: "10.0.26100"}}
	got := s.Software(entries)
	for i, entry := range entries {
		if got[i].Version != entry.Version {
			t.Fatalf("OS version changed: got %q, want %q", got[i].Version, entry.Version)
		}
	}
}

func TestNativeWLANEvidenceAndSafeStatusTags(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"system.wlan.status", "system.wlan.roaming_events", "system.wlan.channel_swap_events", "system.wlan.check.errors"}
	wantTags := []string{"status:ok", "status:warning", "status:critical", "reason:ipc_failure", "reason:interface_inactive", "error_type:ipc_failure"}
	tags := append(slices.Clone(wantTags), "status:"+secret, "reason:"+secret, "error_type:"+secret)
	var source []*metrics.Serie
	for _, name := range names {
		source = append(source, &metrics.Serie{Name: name, Tags: tagset.CompositeTagsFromSlice(tags), Points: []metrics.Point{{Ts: 10, Value: 1}}})
	}
	clean, err := s.Series(NewSeriesSource(source))
	if err != nil {
		t.Fatal(err)
	}
	got := clean.(*SeriesSource).Series
	if len(got) != len(names) {
		t.Fatal("lost native WLAN metrics")
	}
	for i, serie := range got {
		noSecret(t, serie)
		var gotTags []string
		serie.Tags.ForEach(func(tag string) { gotTags = append(gotTags, tag) })
		if serie.Name != names[i] || !slices.Equal(gotTags, wantTags) {
			t.Fatalf("lost WLAN names or status evidence: %s %v", serie.Name, gotTags)
		}
	}
}

func TestEquivalentNetworkIdentitiesUseSamePlaceholders(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	if s.mac("AA-BB-CC-DD-EE-FF") != s.mac("aa:bb:cc:dd:ee:ff") {
		t.Fatal("equivalent MAC representations produce different identities")
	}
	if s.ip("2001:0db8:0000::1") != s.ip("2001:db8::1") || s.ip("::ffff:192.0.2.1") != s.ip("192.0.2.1") {
		t.Fatal("equivalent IP representations produce different identities")
	}
}

func TestNativeMetricDimensionsRemainDistinctAndSanitized(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	source, err := s.Series(NewSeriesSource([]*metrics.Serie{
		{Name: "system.cpu.user", Tags: tagset.CompositeTagsFromSlice([]string{"infra_mode:end_user_device", "infra_mode:" + secret})},
		{Name: "system.cpu.user.total", Tags: tagset.CompositeTagsFromSlice([]string{"core:cpu-total", "core:" + secret})},
		{Name: "system.cpu.user.total", Tags: tagset.CompositeTagsFromSlice([]string{"core:cpu0"})},
		{Name: "system.paging.total", Tags: tagset.CompositeTagsFromSlice([]string{"pagefile_path:" + secret})},
		{Name: "system.paging.total", Tags: tagset.CompositeTagsFromSlice([]string{"pagefile_path:" + secret + "-second"})},
	}))
	if err != nil {
		t.Fatal(err)
	}
	series := source.(*SeriesSource).Series
	if len(series) != 5 {
		t.Fatal("lost captured resource metric")
	}
	tags := make([][]string, len(series))
	for i, serie := range series {
		noSecret(t, serie)
		serie.Tags.ForEach(func(tag string) { tags[i] = append(tags[i], tag) })
		if len(tags[i]) != 1 {
			t.Fatalf("lost or leaked metric dimension: %v", tags[i])
		}
	}
	if tags[0][0] != "infra_mode:end_user_device" || tags[1][0] != "core:cpu-total" || tags[2][0] != "core:cpu0" || tags[3][0] == tags[4][0] {
		t.Fatal("native metric dimension changed or distinct pagefiles collapsed")
	}
}

func TestNativeEUDMHostTagsPreserveSafeProfile(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"infra_mode:end_user_device", "os_name:darwin", "os_version:24.6.0", "total_memory_gb:16", "cpu_model:Apple_M3_Max", "device_model:MacBookPro18,3"}
	tags := append(slices.Clone(want), "os_name:"+secret, "os_version:"+secret, "total_memory_gb:"+secret, "owner:"+secret, "cloud:"+secret)
	clean, err := s.HostMetadata(&HostMetadata{HostTags: map[string][]string{"system": tags, "google cloud platform": {secret}}})
	if err != nil {
		t.Fatal(err)
	}
	noSecret(t, clean)
	host := clean.(*HostMetadata)
	if len(host.HostTags) != 1 || !slices.Equal(host.HostTags["system"], want) {
		t.Fatalf("native EUDM tags changed or sensitive tags survived: %v", host.HostTags)
	}
	for _, key := range []string{"cpu_model", "device_model"} {
		got := s.hostTags([]string{key + ":" + secret})
		noSecret(t, got)
		if len(got) != 2 || got[1] == key+":" || got[1] != s.hostTags([]string{key + ":" + secret})[1] {
			t.Fatal("free-form hardware model was not stably pseudonymized")
		}
	}
}

func TestProcessPathsFollowCapturedOperatingSystem(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		os, exe, cwd, root string
	}{
		{"Windows", `C:\capture\bin\SentinelAgent.exe`, `C:\capture`, `C:\`},
		{"darwin", "/capture/bin/SentinelAgent.exe", "/capture", "/"},
	} {
		input := &model.CollectorProc{Info: &model.SystemInfo{Os: &model.OSInfo{Name: tc.os}}, Processes: []*model.Process{{Command: &model.Command{Comm: "SentinelAgent.exe", Exe: secret, Args: []string{secret}, Cwd: secret, Root: secret}}}}
		clean := s.Process(input)
		noSecret(t, clean)
		cmd := clean.Processes[0].Command
		if cmd.Exe != tc.exe || cmd.Cwd != tc.cwd || cmd.Root != tc.root || !slices.Equal(cmd.Args, []string{tc.exe}) {
			t.Fatalf("paths do not represent captured %s: %+v", tc.os, cmd)
		}
		if input.Processes[0].Command.Exe != secret {
			t.Fatal("mutated source command")
		}
	}
}

func TestSoftwarePathsPreservePortablePathShape(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, prefix string }{
		{`D:\Users\` + secret + `\application`, `C:\capture\software\`},
		{`d:/Users/` + secret + `/application`, `C:\capture\software\`},
		{`\\?\D:\Users\` + secret, `C:\capture\software\`},
		{`\\` + secret + `\share\application`, `\\capture-server\software\`},
		{`\\?\UNC\` + secret + `\share`, `\\capture-server\software\`},
		{"/Users/" + secret + "/application", "/capture/software/"},
	} {
		clean := s.Software([]software.Entry{{InstallPaths: []string{tc.path}}})
		noSecret(t, clean)
		if len(clean[0].InstallPaths) != 1 || !strings.HasPrefix(clean[0].InstallPaths[0], tc.prefix+"path-") {
			t.Fatalf("sanitized install path has wrong platform shape: %q", clean[0].InstallPaths)
		}
	}
}

func TestHardwareUUIDMatchesHostAndProcessIdentity(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	host, err := s.HostMetadata(&HostMetadata{UUID: secret, Gohai: `{"platform":{"hardware_uuid":"` + strings.ToLower(secret) + `","serial_number":"` + secret + `"}}`})
	if err != nil {
		t.Fatal(err)
	}
	noSecret(t, host)
	var gohai map[string]map[string]string
	if err := json.Unmarshal([]byte(host.(*HostMetadata).Gohai), &gohai); err != nil {
		t.Fatal(err)
	}
	process := s.Process(&model.CollectorProc{Info: &model.SystemInfo{Uuid: secret}})
	if host.(*HostMetadata).UUID != process.Info.Uuid || gohai["platform"]["hardware_uuid"] != process.Info.Uuid {
		t.Fatal("hardware UUID, host UUID, and process UUID diverged")
	}
	if gohai["platform"]["serial_number"] == process.Info.Uuid {
		t.Fatal("serial number and UUID identities were conflated")
	}
}
