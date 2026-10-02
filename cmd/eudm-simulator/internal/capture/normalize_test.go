// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package capture

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/gogo/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

type rawMetadata []byte

func (r rawMetadata) MarshalJSON() ([]byte, error) { return r, nil }

func TestMetricValuesAndDimensionsRemainUnchanged(t *testing.T) {
	in := &metrics.Serie{Name: "system.wlan.rssi", Host: "engineer-laptop", Device: "en0", Source: metrics.MetricSourceWlan,
		MType: metrics.APIGaugeType, Interval: 17, Unit: "dBm", SourceTypeName: "system", NoIndex: true,
		Tags:      tagset.CompositeTagsFromSlice([]string{"ssid:Office WiFi", "bssid:01:02:03:04:05:06", "team:engineering", "custom-tag"}),
		Resources: []metrics.Resource{{Type: "host", Name: "engineer-laptop"}}, Points: []metrics.Point{{Ts: 123.125, Value: -48.5}}}
	source, err := NewNormalizer().Series(NewSeriesSource([]*metrics.Serie{in}))
	if err != nil {
		t.Fatal(err)
	}
	out := source.(*SeriesSource).Series[0]
	if !reflect.DeepEqual(in, out) {
		t.Fatal("capture changed metric fields or dimensions")
	}
	out.Points[0].Value = 0
	out.Resources[0].Name = "changed"
	out.Tags.UnsafeToReadOnlySliceString()[0] = "changed"
	if in.Points[0].Value != -48.5 || in.Resources[0].Name != "engineer-laptop" || in.Tags.UnsafeToReadOnlySliceString()[0] != "ssid:Office WiFi" {
		t.Fatal("metric copy aliases producer memory")
	}
}

func TestHostMetadataPreservesProjectedValuesAndExcludesCredentials(t *testing.T) {
	input := rawMetadata(`{"agentVersion":"7.85.0-local","internalHostname":"engineer-laptop","uuid":"native-uuid","os":"darwin","python":"3.13.1","apiKey":"credential-sentinel","config":{"api_key":"credential-sentinel"},"meta":{"hostname":"engineer-laptop","custom":"native-value"},"systemStats":{"machine":"arm64","macV":["26.0 (25A354)",["native", "release", "info"],"arm64"]},"network":{"network-id":"office-network"},"host-tags":{"system":["team:engineering","device_model:MacBook Pro"]},"gohai":"{\"cpu\":{\"model_name\":\"AMD Ryzen 9 9950X\"},\"platform\":{\"serial_number\":\"DEVICE12345\"}}"}`)
	value, err := NewNormalizer().HostMetadata(input)
	if err != nil {
		t.Fatal(err)
	}
	out := value.(*HostMetadata)
	if out.Hostname != "engineer-laptop" || out.UUID != "native-uuid" || out.Network["network-id"] != "office-network" ||
		!slices.Equal(out.HostTags["system"], []string{"team:engineering", "device_model:MacBook Pro"}) || !strings.Contains(out.Gohai, "AMD Ryzen 9 9950X") || !strings.Contains(out.Gohai, "DEVICE12345") {
		t.Fatal("native host metadata changed")
	}
	encoded, err := out.MarshalJSON()
	if err != nil || strings.Contains(string(encoded), "credential-sentinel") {
		t.Fatal("credential fields crossed the metadata projection")
	}
	owned, err := NewNormalizer().HostMetadata(out)
	if err != nil {
		t.Fatal(err)
	}
	owned.(*HostMetadata).HostTags["system"][0] = "changed"
	if out.HostTags["system"][0] != "team:engineering" {
		t.Fatal("metadata copy aliases producer memory")
	}
	nativeGohai := `{"network":{"interfaces":[{"name":"en0","addresses":["192.0.2.4","2001:db8::4"]}]},"filesystem":[{"name":"/dev/disk3s1","size":1000000}]}`
	native, err := NewNormalizer().HostMetadata(&HostMetadata{Gohai: nativeGohai})
	if err != nil || native.(*HostMetadata).Gohai != nativeGohai {
		t.Fatal("nested native gohai fields changed")
	}
	if _, err := NewNormalizer().HostMetadata(&HostMetadata{Gohai: "{"}); err == nil {
		t.Fatal("invalid embedded gohai JSON accepted")
	}
}

func TestProcessPreservesFullNativeMessageAndOwnership(t *testing.T) {
	in := &model.CollectorProc{HostName: "engineer-laptop", NetworkId: "office-network", GroupId: 4, GroupSize: 1,
		Info: &model.SystemInfo{Uuid: "device-uuid", TotalMemory: 16 << 30, Os: &model.OSInfo{Name: "darwin", Version: "26.0 (25A354)"}},
		Processes: []*model.Process{{Pid: 426, NsPid: 426, CreateTime: 123456789, Command: &model.Command{Comm: "Visual Studio Code", Exe: "/Applications/Visual Studio Code.app/Contents/MacOS/Electron", Cwd: "/Users/alex/work", Args: []string{"code", "--user-data-dir=/Users/alex/Library/Code", "--token=********"}, Ppid: 1}, User: &model.ProcessUser{Name: "alex", Uid: 501},
			IoStat: &model.IOStat{ReadRate: 25.5, WriteRate: 77}, ByteKey: []byte{1, 2, 3}, Tags: []string{"team:engineering"}, PortInfo: &model.PortInfo{Tcp: []int32{4000}}, ZombieChildrenCount: 2}}}
	out := NewNormalizer().Process(in)
	if !proto.Equal(in, out) {
		t.Fatal("process capture discarded native fields")
	}
	out.Processes[0].Command.Args[0] = "changed"
	out.Processes[0].IoStat.ReadRate = 0
	out.Processes[0].ByteKey[0] = 0
	out.Processes[0].PortInfo.Tcp[0] = 0
	if in.Processes[0].Command.Args[0] != "code" || in.Processes[0].IoStat.ReadRate != 25.5 || in.Processes[0].ByteKey[0] != 1 || in.Processes[0].PortInfo.Tcp[0] != 4000 {
		t.Fatal("process copy aliases producer memory")
	}
}

func TestConnectionsPreserveDNSAndAllNativeFields(t *testing.T) {
	for _, version := range []int{1, 2} {
		in := &model.CollectorConnections{HostName: "engineer-laptop", NetworkId: "office-network", GroupSize: 1,
			Domains: []string{"api.example.org", "unused.example.org"}, EncodedTags: []byte{1, 2, 3}, ContainerForPid: map[int32]string{426: "container-a"},
			AgentConfiguration: &model.AgentConfiguration{EudmEnabled: true},
			Connections:        []*model.Connection{{Pid: 426, Laddr: &model.Addr{Ip: "192.0.2.1", Port: 50000}, Raddr: &model.Addr{Ip: "198.51.100.2", Port: 443}, LastBytesSent: 1234, Rtt: 1500, TcpFailuresByErrCode: map[uint32]uint32{111: 3}}}}
		var err error
		if version == 1 {
			in.EncodedDNS, err = model.NewV1DNSEncoder().Encode(map[string]*model.DNSEntry{"198.51.100.2": {Names: []string{"api.example.org"}}})
		} else {
			encoder := model.NewV2DNSEncoder()
			var offsets []int32
			in.EncodedDomainDatabase, offsets, err = encoder.EncodeDomainDatabase(in.Domains)
			if err == nil {
				in.EncodedDnsLookups, err = encoder.EncodeMapped(map[string]*model.DNSDatabaseEntry{"198.51.100.2": {NameOffsets: []int32{0}}}, offsets)
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		out := NewNormalizer().Connections(in)
		if out == nil || !proto.Equal(in, out) {
			t.Fatal("connection capture changed fields or encoded table bytes")
		}
		var names []string
		if err := out.IterateDNS(out.Connections[0].Raddr, func(_, _ int, name string) bool { names = append(names, name); return true }); err != nil || !slices.Equal(names, []string{"api.example.org"}) {
			t.Fatal("DNS association changed")
		}
		out.EncodedTags[0] = 0
		out.Connections[0].TcpFailuresByErrCode[111] = 0
		out.ContainerForPid[426] = "changed"
		if in.EncodedTags[0] != 1 || in.Connections[0].TcpFailuresByErrCode[111] != 3 || in.ContainerForPid[426] != "container-a" {
			t.Fatal("connection copy aliases producer memory")
		}
	}
}

func TestConnectionDNSRejectsMalformedTables(t *testing.T) {
	input := &model.CollectorConnections{Connections: []*model.Connection{{Laddr: &model.Addr{Ip: "192.0.2.1"}, Raddr: &model.Addr{Ip: "198.51.100.2"}}}, EncodedDNS: []byte{255}}
	if NewNormalizer().Connections(input) != nil {
		t.Fatal("malformed DNS accepted")
	}
	input.EncodedDNS = nil
	input.EncodedDomainDatabase = []byte{1, 0, 1, 'a'}
	input.EncodedDnsLookups = []byte{2, 1}
	if NewNormalizer().Connections(input) != nil {
		t.Fatal("truncated V2 DNS accepted")
	}
}

func TestProtobufStringsAndBytesArePreserved(t *testing.T) {
	fill := func(ptr any) {
		value := reflect.ValueOf(ptr).Elem()
		for i := 0; i < value.NumField(); i++ {
			field := value.Field(i)
			if field.CanSet() && field.Kind() == reflect.String {
				field.SetString("native-protobuf-value")
			}
			if field.CanSet() && field.Type() == reflect.TypeFor[[]byte]() {
				field.SetBytes([]byte("native-protobuf-value"))
			}
		}
	}
	process, connection, command, user, info, osInfo, cpu, addr := &model.Process{}, &model.Connection{}, &model.Command{}, &model.ProcessUser{}, &model.SystemInfo{}, &model.OSInfo{}, &model.CPUInfo{}, &model.Addr{}
	for _, value := range []any{process, connection, command, user, info, osInfo, cpu, addr} {
		fill(value)
	}
	process.Command, process.User = command, user
	info.Os, info.Cpus = osInfo, []*model.CPUInfo{cpu}
	connection.Laddr, connection.Raddr = addr, addr
	proc := &model.CollectorProc{Processes: []*model.Process{process}, Info: info}
	conn := &model.CollectorConnections{Connections: []*model.Connection{connection}}
	if !proto.Equal(proc, NewNormalizer().Process(proc)) || !proto.Equal(conn, NewNormalizer().Connections(conn)) {
		t.Fatal("protobuf fields were discarded")
	}
	// All typed fields survive the same JSON boundary used by the bundle writer.
	data, err := json.Marshal(NewNormalizer().Process(proc))
	if err != nil || !strings.Contains(string(data), "native-protobuf-value") {
		t.Fatal("native strings missing from portable process JSON")
	}
}
