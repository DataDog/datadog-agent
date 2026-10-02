// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package accesspoint

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/identity"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/overlay"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func steady(value float64) schema.Pattern {
	return schema.Pattern{Steady: &schema.SteadyPattern{Value: value}}
}

func fixtureScenario(count int) *schema.Scenario {
	s := &schema.Scenario{Version: schema.Version, Meta: schema.ScenarioMeta{Name: "private-incident-name"}, Expectation: schema.Expectation{Conclusion: schema.WirelessAccessPoints, AffectedCohorts: []string{"clients-0"}}, MonitorWindow: schema.Duration{Duration: time.Minute}}
	for _, phase := range []string{"healthy", "onset", "sustained", "recovery"} {
		s.Phases = append(s.Phases, schema.Phase{Name: phase, Duration: schema.Duration{Duration: 2 * time.Minute}, NetworkMetrics: map[string]schema.APPhaseMetrics{}, Metrics: map[string]map[string]schema.Pattern{}})
	}
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("private-ap-%d", i)
		s.NetworkDevices.AccessPoints = append(s.NetworkDevices.AccessPoints, schema.AccessPointDef{Name: name, IPAddress: fmt.Sprintf("192.0.2.%d", i+1), SerialNumber: "private-serial", Vendor: "aruba", Model: "AP-535", Location: "private-location", Interfaces: []schema.NetworkInterfaceDef{{Name: "private-ethernet", Index: 1, Kind: "ethernet"}, {Name: "private-radio", Index: 2, Kind: "radio", Band: "5GHz", SSID: "private-ssid"}}})
		s.Fleet = append(s.Fleet, schema.GroupDef{Group: fmt.Sprintf("clients-%d", i), Count: 2, OS: "macos", AccessPoint: name, Radio: "private-radio"})
		s.Phases[0].NetworkMetrics[name] = schema.APPhaseMetrics{Device: map[string]schema.Pattern{"snmp.device.reachable": steady(1), "snmp.cpu.usage": steady(8)}, Interfaces: map[string]map[string]schema.Pattern{"private-radio": {"snmp.apChannelNoise": steady(-95), "snmp.apChannelBwRate": steady(15), "snmp.ifAdminStatus": steady(1), "snmp.ifOperStatus": steady(1), "snmp.ifInErrors": steady(0)}}}
	}
	s.Phases[1].NetworkMetrics["private-ap-0"] = schema.APPhaseMetrics{Interfaces: map[string]map[string]schema.Pattern{"private-radio": {"snmp.apChannelNoise": {Ramp: &schema.RampPattern{From: -95, To: -65}}, "snmp.apChannelBwRate": {Ramp: &schema.RampPattern{From: 15, To: 90}}, "snmp.ifInErrors": steady(500)}}}
	s.Phases[1].Metrics["clients-0"] = map[string]schema.Pattern{"system.wlan.rssi": {Ramp: &schema.RampPattern{From: -55, To: -83}}, "system.wlan.noise": {Ramp: &schema.RampPattern{From: -95, To: -65}}, "system.wlan.txrate": {Ramp: &schema.RampPattern{From: 600, To: 15}}}
	s.Phases[2].Metrics["clients-0"] = map[string]schema.Pattern{"system.wlan.rssi": steady(-83), "system.wlan.noise": steady(-65), "system.wlan.txrate": steady(15)}
	s.Phases[3].NetworkMetrics["private-ap-0"] = schema.APPhaseMetrics{Interfaces: map[string]map[string]schema.Pattern{"private-radio": {"snmp.apChannelNoise": steady(-95), "snmp.apChannelBwRate": steady(15), "snmp.ifInErrors": steady(0)}}}
	return s
}

func TestWirelessResourcesAndClientsAcrossBatchBoundary(t *testing.T) {
	s := fixtureScenario(25) // 125 NDM resources, crossing the normal 100 limit.
	runID := strings.Repeat("1", 32)
	m, err := New(s, runID, 42)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Unix(1700000000, 0)
	payloads := m.Metadata(at, metadata.PayloadMetadataBatchSize)
	if len(payloads) != 2 {
		t.Fatalf("expected two bounded payloads, got %d", len(payloads))
	}
	devices := map[string]metadata.DeviceMetadata{}
	interfaces := map[string]metadata.InterfaceMetadata{}
	radios := map[string]metadata.WirelessInterfaceMetadata{}
	var addresses []metadata.IPAddressMetadata
	for _, payload := range payloads {
		count := len(payload.Devices) + len(payload.Interfaces) + len(payload.WirelessInterfaces) + len(payload.IPAddresses)
		if count > metadata.PayloadMetadataBatchSize || payload.Namespace != identity.Namespace(runID) || payload.CollectTimestamp != at.Unix() {
			t.Fatal("invalid metadata batch boundary, namespace, or timestamp")
		}
		for _, d := range payload.Devices {
			devices[d.ID] = d
		}
		for _, iface := range payload.Interfaces {
			interfaces[metadata.InterfaceID(iface.DeviceID, iface.Index)] = iface
		}
		for _, radio := range payload.WirelessInterfaces {
			radios[radio.BSSID] = radio
		}
		addresses = append(addresses, payload.IPAddresses...)
	}
	if len(devices) != 25 || len(interfaces) != 50 || len(radios) != 25 || len(addresses) != 25 {
		t.Fatal("batching lost or duplicated NDM resources")
	}
	clientMACs := map[string]bool{}
	for _, group := range s.Fleet {
		wireless, err := m.Wireless(group)
		if err != nil {
			t.Fatal(err)
		}
		radio, found := radios[wireless.BSSID]
		if !found || radio.SSID != wireless.SSID || radio.Band != "5GHz" || interfaces[radio.InterfaceByIntegrationID].MacAddress != radio.BSSID {
			t.Fatal("client wireless identity does not resolve to its NDM radio")
		}
		device := devices[radio.DeviceByIntegrationID]
		if device.Status != metadata.DeviceStatusReachable || radio.AdminStatus != metadata.AdminStatusUp || radio.OperStatus != metadata.OperStatusUp {
			t.Fatal("radio degradation must keep AP reachability and interface status healthy")
		}
		for i := 0; i < group.Count; i++ {
			id := identity.New(runID, 42, group.Group, len(clientMACs))
			sample := &telemetry.Sample{Metrics: []*metrics.Serie{{Name: "system.wlan.rssi", Host: "capture-host", Points: []metrics.Point{{Value: -55}}, Tags: tagset.CompositeTagsFromSlice([]string{"bssid:02:00:00:00:00:00", "ssid:capture-ssid", "client_mac:02:00:00:00:00:01"})}}}
			if err := id.Apply(sample, group, wireless); err != nil {
				t.Fatal(err)
			}
			tags := sample.Metrics[0].Tags.UnsafeToReadOnlySliceString()
			if !slices.Contains(tags, "bssid:"+radio.BSSID) || !slices.Contains(tags, "ssid:"+radio.SSID) || !slices.Contains(tags, "client_mac:"+id.ClientMAC) || clientMACs[id.ClientMAC] {
				t.Fatal("wireless client correlation or per-device MAC uniqueness failed")
			}
			clientMACs[id.ClientMAC] = true
		}
	}
	for _, address := range addresses {
		if _, found := interfaces[address.InterfaceID]; !found {
			t.Fatal("AP address references an absent interface")
		}
	}
	encoded, err := json.Marshal(payloads)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private-serial", "private-location", "private-ethernet", "private-radio", s.Meta.Name, "affected", "wireless_access_points"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("metadata disclosed local identity or expectation %q", forbidden)
		}
	}
	other, err := New(s, strings.Repeat("2", 32), 42)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := m.Wireless(s.Fleet[0])
	b, _ := other.Wireless(s.Fleet[0])
	if a.BSSID == b.BSSID || a.SSID != "private-ssid" || b.SSID != a.SSID || reflect.DeepEqual(payloads, other.Metadata(at, 100)) {
		t.Fatal("concurrent runs share radio identities")
	}
}

func TestCorrelatedProgressionAndDeterministicCarryForward(t *testing.T) {
	s := fixtureScenario(3)
	m, err := New(s, strings.Repeat("a", 32), 42)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Unix(1700000000, 0)
	for phase := range s.Phases {
		values, err := m.Metrics(phase, s.Phases[phase].Duration.Duration, 17, at)
		if err != nil {
			t.Fatal(err)
		}
		for _, point := range values {
			value := point.Points[0].Value
			if point.Source != metrics.MetricSourceSnmp || !slices.Contains(point.Tags.UnsafeToReadOnlySliceString(), "eudm_run_id:"+m.runID) {
				t.Fatal("AP metrics do not use Agent NDM origin or run isolation")
			}
			if statusMetric(point.Name) && value != 1 {
				t.Fatal("status metric was jittered or an AP became unreachable")
			}
			if point.Name == "snmp.ifInErrors" && point.MType != metrics.APICountType {
				t.Fatal("interface error counter lost its Agent count type")
			}
			if point.Name != "snmp.apChannelNoise" {
				continue
			}
			affected := point.Host == m.accessPoints[0].device.Name && (phase == 1 || phase == 2)
			if affected && (value < -68 || value > -62) || !affected && (value < -98 || value > -92) {
				t.Fatal("AP or comparison noise disagrees with phase progression")
			}
		}
		for _, group := range s.Fleet {
			sample := &telemetry.Sample{Metrics: []*metrics.Serie{{Name: "system.cpu.user", Points: []metrics.Point{{Value: 8}}}, {Name: "system.wlan.rssi", Points: []metrics.Point{{Value: -55}}}, {Name: "system.wlan.noise", Points: []metrics.Point{{Value: -95}}}, {Name: "system.wlan.txrate", Points: []metrics.Point{{Value: 600}}}}}
			ctx := overlay.Context{Scenario: s, Group: group, PhaseIndex: phase, Elapsed: s.Phases[phase].Duration.Duration, Stream: schema.Metrics}
			if err := overlay.Apply(ctx, sample); err != nil {
				t.Fatal(err)
			}
			if sample.Metrics[0].Points[0].Value != 8 {
				t.Fatal("wireless overlay changed host workload")
			}
			affected := group.Group == "clients-0" && (phase == 1 || phase == 2)
			if affected && sample.Metrics[1].Points[0].Value != -83 || !affected && sample.Metrics[1].Points[0].Value != -55 {
				t.Fatal("client signal and AP degradation disagree")
			}
		}
	}
	want, err := m.Metrics(2, time.Minute, 40, at)
	if err != nil {
		t.Fatal(err)
	}
	// Evaluate later, earlier, and repeated samples in a different worker order.
	for i := 100; i >= 0; i-- {
		if _, err := m.Metrics(3, time.Minute, int64(i), at); err != nil {
			t.Fatal(err)
		}
	}
	got, err := m.Metrics(2, time.Minute, 40, at)
	if err != nil || !reflect.DeepEqual(want, got) {
		t.Fatal("AP carry-forward depends on call order or accumulated jitter")
	}
}
