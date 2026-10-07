// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ioscmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateExample(t *testing.T) {
	doc := `[
		{"type": "show", "target": "running-config", "pipe": {"type": "section", "section": "system"}},
		{"type": "hostname", "hostname": "mydevice"},
		{
			"type": "interface",
			"name": "GigabitEthernet1/0/1",
			"default_first": true,
			"commands": [
				{"type": "shutdown", "value": true},
				{"type": "description", "value": "Some description"},
				{"type": "switchport", "value": false},
				{"type": "ip_addr", "addr": "192.0.2.1", "mask": "255.255.255.252"},
				{"type": "shutdown", "value": false}
			]
		}
	]`
	lines, err := Generate([]byte(doc))
	require.NoError(t, err)
	assert.Equal(t, []string{
		"show running-config | section system",
		"configure terminal",
		"hostname mydevice",
		"default interface GigabitEthernet1/0/1",
		"interface GigabitEthernet1/0/1",
		" shutdown",
		" description Some description",
		" no switchport",
		" ip address 192.0.2.1 255.255.255.252",
		" no shutdown",
		"end",
	}, lines)
}

func TestModeSwitching(t *testing.T) {
	doc := `[
		{"type": "hostname", "hostname": "a"},
		{"type": "show", "target": "version"},
		{"type": "show", "target": "clock"},
		{"type": "hostname", "hostname": "b"},
		{"type": "interface", "name": "Loopback0", "commands": []}
	]`
	lines, err := Generate([]byte(doc))
	require.NoError(t, err)
	assert.Equal(t, []string{
		"configure terminal",
		"hostname a",
		"end",
		"show version",
		"show clock",
		"configure terminal",
		"hostname b",
		"interface Loopback0",
		"end",
	}, lines)
}

func TestShowCommands(t *testing.T) {
	for _, tc := range []struct {
		json     string
		expected string
	}{
		{`{"type":"show","target":"startup-config"}`, "show startup-config"},
		{`{"type":"show","target":"running-config","interface":"Gi1/0/1"}`, "show running-config interface Gi1/0/1"},
		{`{"type":"show","target":"interfaces"}`, "show interfaces"},
		{`{"type":"show","target":"interfaces","interface":"Gi1/0/1","detail":"counters"}`, "show interfaces Gi1/0/1 counters"},
		{`{"type":"show","target":"interfaces","detail":"status","pipe":{"type":"include","pattern":"connected"}}`, "show interfaces status | include connected"},
		{`{"type":"show","target":"ip interface","brief":true,"pipe":{"type":"exclude","pattern":"unassigned"}}`, "show ip interface brief | exclude unassigned"},
		{`{"type":"show","target":"ip interface","interface":"Vlan10"}`, "show ip interface Vlan10"},
		{`{"type":"show","target":"ip route","vrf":"MGMT","prefix":"10.0.0.0"}`, "show ip route vrf MGMT 10.0.0.0"},
		{`{"type":"show","target":"vlan","brief":true}`, "show vlan brief"},
		{`{"type":"show","target":"vlan","id":20}`, "show vlan id 20"},
		{`{"type":"show","target":"cdp neighbors","detail":true}`, "show cdp neighbors detail"},
		{`{"type":"show","target":"lldp neighbors","pipe":{"type":"count","pattern":"Gi"}}`, "show lldp neighbors | count Gi"},
		{`{"type":"show","target":"mac address-table","vlan":10}`, "show mac address-table vlan 10"},
		{`{"type":"show","target":"mac address-table","interface":"Gi1/0/2","pipe":{"type":"begin","pattern":"Vlan"}}`, "show mac address-table interface Gi1/0/2 | begin Vlan"},
		{`{"type":"show","target":"inventory"}`, "show inventory"},
		{`{"type":"show","target":"interfaces","interface":"Port-channel1"}`, "show interfaces Port-channel1"},
		{`{"type":"show","target":"interfaces","interface":"TenGigabitEthernet1/1/1.100"}`, "show interfaces TenGigabitEthernet1/1/1.100"},
		{`{"type":"show","target":"running-config","interface":"Serial0/0/0:0"}`, "show running-config interface Serial0/0/0:0"},
		{`{"type":"show","target":"ip route","vrf":"Mgmt-intf"}`, "show ip route vrf Mgmt-intf"},
		{`{"type":"show","target":"interfaces","pipe":{"type":"include","pattern":"^Gi[0-9/]+ .*[Uu]p$"}}`, "show interfaces | include ^Gi[0-9/]+ .*[Uu]p$"},
	} {
		t.Run(tc.expected, func(t *testing.T) {
			lines, err := Generate([]byte("[" + tc.json + "]"))
			require.NoError(t, err)
			assert.Equal(t, []string{tc.expected}, lines)
		})
	}
}

func TestInterfaceSubCommands(t *testing.T) {
	for _, tc := range []struct {
		json     string
		expected string
	}{
		{`{"type":"description","value":null}`, "no description"},
		{`{"type":"ip_addr","addr":"10.0.0.1","mask":"255.255.255.0","secondary":true}`, "ip address 10.0.0.1 255.255.255.0 secondary"},
		{`{"type":"ip_addr_dhcp"}`, "ip address dhcp"},
		{`{"type":"no_ip_addr"}`, "no ip address"},
		{`{"type":"ipv6_addr","prefix":"2001:db8::1/64"}`, "ipv6 address 2001:db8::1/64"},
		{`{"type":"ipv6_addr","prefix":"2001:db8::/64","modifier":"eui-64"}`, "ipv6 address 2001:db8::/64 eui-64"},
		{`{"type":"ipv6_addr","prefix":"fe80::1","modifier":"link-local","remove":true}`, "no ipv6 address fe80::1 link-local"},
		{`{"type":"ipv6_addr","prefix":"2001:DB8:0:0::1/64"}`, "ipv6 address 2001:db8::1/64"},
		{`{"type":"ipv6_addr","prefix":"FE80:0::1","modifier":"link-local"}`, "ipv6 address fe80::1 link-local"},
		{`{"type":"ip_helper_address","addr":"192.0.2.10"}`, "ip helper-address 192.0.2.10"},
		{`{"type":"ip_helper_address","addr":"192.0.2.10","remove":true}`, "no ip helper-address 192.0.2.10"},
		{`{"type":"vrf_forwarding","value":"MGMT"}`, "vrf forwarding MGMT"},
		{`{"type":"vrf_forwarding","value":null}`, "no vrf forwarding"},
		{`{"type":"switchport_mode","mode":"trunk"}`, "switchport mode trunk"},
		{`{"type":"switchport_mode","mode":"dynamic desirable"}`, "switchport mode dynamic desirable"},
		{`{"type":"switchport_access_vlan","value":10}`, "switchport access vlan 10"},
		{`{"type":"switchport_access_vlan","value":null}`, "no switchport access vlan"},
		{`{"type":"switchport_voice_vlan","value":100}`, "switchport voice vlan 100"},
		{`{"type":"switchport_trunk_native_vlan","value":99}`, "switchport trunk native vlan 99"},
		{`{"type":"switchport_trunk_allowed_vlan","vlans":"10,20,30-40"}`, "switchport trunk allowed vlan 10,20,30-40"},
		{`{"type":"switchport_trunk_allowed_vlan","action":"add","vlans":"50"}`, "switchport trunk allowed vlan add 50"},
		{`{"type":"switchport_trunk_allowed_vlan","vlans":"none"}`, "switchport trunk allowed vlan none"},
		{`{"type":"speed","value":"1000"}`, "speed 1000"},
		{`{"type":"speed","value":null}`, "no speed"},
		{`{"type":"duplex","value":"full"}`, "duplex full"},
		{`{"type":"mtu","value":9000}`, "mtu 9000"},
		{`{"type":"ip_mtu","value":null}`, "no ip mtu"},
		{`{"type":"channel_group","value":{"group":1,"mode":"active"}}`, "channel-group 1 mode active"},
		{`{"type":"channel_group","value":null}`, "no channel-group"},
		{`{"type":"ip_access_group","acl":"BLOCK-IN","direction":"in"}`, "ip access-group BLOCK-IN in"},
		{`{"type":"ip_access_group","acl":"101","direction":"out"}`, "ip access-group 101 out"},
		{`{"type":"description","value":"Uplink to core-sw1 (port 48) #2, see TICKET-123"}`, "description Uplink to core-sw1 (port 48) #2, see TICKET-123"},
		{`{"type":"spanning_tree_portfast","value":true}`, "spanning-tree portfast"},
		{`{"type":"spanning_tree_bpduguard","value":false}`, "no spanning-tree bpduguard enable"},
		{`{"type":"negotiation_auto","value":true}`, "negotiation auto"},
		{`{"type":"ip_proxy_arp","value":false}`, "no ip proxy-arp"},
	} {
		t.Run(tc.expected, func(t *testing.T) {
			doc := `[{"type":"interface","name":"Gi1/0/1","commands":[` + tc.json + `]}]`
			lines, err := Generate([]byte(doc))
			require.NoError(t, err)
			assert.Equal(t, []string{"configure terminal", "interface Gi1/0/1", " " + tc.expected, "end"}, lines)
		})
	}
}

func TestInvalidDocuments(t *testing.T) {
	iface := func(sub string) string {
		return `[{"type":"interface","name":"Gi1/0/1","commands":[` + sub + `]}]`
	}
	for _, tc := range []struct {
		name      string
		json      string
		expectErr string
	}{
		{"not_array", `{"type":"hostname","hostname":"x"}`, "cannot unmarshal object"},
		{"null", `null`, "must be a JSON array"},
		{"missing_type", `[{"hostname":"x"}]`, `missing "type"`},
		{"unknown_type", `[{"type":"reload"}]`, `unknown command type "reload"`},
		{"unknown_show_target", `[{"type":"show","target":"tech-support"}]`, `unknown show target "tech-support"`},
		{"unknown_field", `[{"type":"hostname","hostname":"x","extra":1}]`, `unknown field "extra"`},
		{"field_for_other_target", `[{"type":"show","target":"version","detail":true}]`, `unknown field "detail"`},
		{"empty_hostname", `[{"type":"hostname","hostname":""}]`, "hostname is required"},
		{"hostname_injection", `[{"type":"hostname","hostname":"x\nreload"}]`, "must be a hostname"},
		{"hostname_pipe", `[{"type":"hostname","hostname":"x|y"}]`, "must be a hostname"},
		{"hostname_leading_digit", `[{"type":"hostname","hostname":"1x"}]`, "must be a hostname"},
		{"description_injection", iface(`{"type":"description","value":"x\nreload"}`), "printable ASCII"},
		{"description_pipe", iface(`{"type":"description","value":"x | redirect flash:y"}`), "printable ASCII"},
		{"description_non_ascii", iface(`{"type":"description","value":"x reload"}`), "printable ASCII"},
		{"description_help", iface(`{"type":"description","value":"why?"}`), "printable ASCII"},
		{"pipe_injection", `[{"type":"show","target":"version","pipe":{"type":"include","pattern":"x\nreload"}}]`, "printable ASCII"},
		{"pipe_chain", `[{"type":"show","target":"version","pipe":{"type":"include","pattern":"x | tee flash:y"}}]`, "printable ASCII"},
		{"pipe_alternation", `[{"type":"show","target":"version","pipe":{"type":"include","pattern":"up|down"}}]`, "printable ASCII"},
		{"pipe_help", `[{"type":"show","target":"version","pipe":{"type":"section","section":"x?"}}]`, "printable ASCII"},
		{"show_interface_injection", `[{"type":"show","target":"interfaces","interface":"Gi1/0/1|python$IFS-c$IFS'print(\"gotcha\")'","detail":"counters"}]`, "interface name"},
		{"show_config_interface_semicolon", `[{"type":"show","target":"running-config","interface":"Gi1;reload"}]`, "interface name"},
		{"show_ip_interface_dollar", `[{"type":"show","target":"ip interface","interface":"Gi1$IFS"}]`, "interface name"},
		{"show_mac_interface_quote", `[{"type":"show","target":"mac address-table","interface":"Gi1'x"}]`, "interface name"},
		{"show_interface_non_ascii", `[{"type":"show","target":"interfaces","interface":"Gi1 x"}]`, "interface name"},
		{"show_vrf_pipe", `[{"type":"show","target":"ip route","vrf":"A|B"}]`, "must be a name"},
		{"interface_name_space", `[{"type":"interface","name":"Gi 1/0/1","commands":[]}]`, "interface name"},
		{"interface_name_pipe", `[{"type":"interface","name":"Gi1/0/1|x","commands":[]}]`, "interface name"},
		{"interface_name_no_number", `[{"type":"interface","name":"Gi","commands":[]}]`, "interface name"},
		{"vrf_forwarding_dollar", iface(`{"type":"vrf_forwarding","value":"A$B"}`), "must be a name"},
		{"acl_semicolon", iface(`{"type":"ip_access_group","acl":"A;B","direction":"in"}`), "must be a name"},
		{"acl_quote", iface(`{"type":"ip_access_group","acl":"A'B","direction":"in"}`), "must be a name"},
		{"missing_commands", `[{"type":"interface","name":"Gi1/0/1"}]`, `"commands" is required`},
		{"unknown_pipe", `[{"type":"show","target":"version","pipe":{"type":"grep","pattern":"x"}}]`, `unknown pipe type "grep"`},
		{"section_pipe_pattern", `[{"type":"show","target":"version","pipe":{"type":"section","pattern":"x"}}]`, `uses "section"`},
		{"startup_interface", `[{"type":"show","target":"startup-config","interface":"Gi1"}]`, "not supported"},
		{"brief_and_interface", `[{"type":"show","target":"ip interface","brief":true,"interface":"Gi1"}]`, "mutually exclusive"},
		{"bad_vlan_id", `[{"type":"show","target":"vlan","id":5000}]`, "between 1 and 4094"},
		{"unknown_subcommand", iface(`{"type":"bogus"}`), `unknown interface command type "bogus"`},
		{"toggle_missing_value", iface(`{"type":"shutdown"}`), "value is required"},
		{"toggle_null_value", iface(`{"type":"shutdown","value":null}`), "value is required"},
		{"nullable_missing", iface(`{"type":"description"}`), "value is required"},
		{"bad_ip", iface(`{"type":"ip_addr","addr":"300.0.0.1","mask":"255.255.255.0"}`), "IPv4 address"},
		{"ipv6_as_ip", iface(`{"type":"ip_addr","addr":"2001:db8::1","mask":"255.255.255.0"}`), "IPv4 address"},
		{"bad_mask", iface(`{"type":"ip_addr","addr":"10.0.0.1","mask":"255.0.255.0"}`), "contiguous"},
		{"ipv6_no_length", iface(`{"type":"ipv6_addr","prefix":"2001:db8::1"}`), "CIDR"},
		{"link_local_not_fe80", iface(`{"type":"ipv6_addr","prefix":"2001:db8::1","modifier":"link-local"}`), "link-local"},
		{"link_local_zone_injection", iface(`{"type":"ipv6_addr","prefix":"fe80::1%x\nreload","modifier":"link-local"}`), "link-local"},
		{"link_local_zone", iface(`{"type":"ipv6_addr","prefix":"fe80::1%Gi1","modifier":"link-local"}`), "link-local"},
		{"ipv6_prefix_zone", iface(`{"type":"ipv6_addr","prefix":"fe80::1%x\nreload/64"}`), "CIDR"},
		{"bad_switchport_mode", iface(`{"type":"switchport_mode","mode":"routed"}`), "mode must be one of"},
		{"bad_access_vlan", iface(`{"type":"switchport_access_vlan","value":0}`), "between 1 and 4094"},
		{"bad_vlan_list", iface(`{"type":"switchport_trunk_allowed_vlan","vlans":"10;20"}`), "VLAN list"},
		{"add_all", iface(`{"type":"switchport_trunk_allowed_vlan","action":"add","vlans":"all"}`), `only valid with the "set" action`},
		{"bad_speed", iface(`{"type":"speed","value":"fast"}`), "speed must be one of"},
		{"speed_number", iface(`{"type":"speed","value":1000}`), "cannot unmarshal number"},
		{"bad_mtu", iface(`{"type":"mtu","value":-1}`), "must be positive"},
		{"channel_group_extra_field", iface(`{"type":"channel_group","value":{"group":1,"mode":"on","x":1}}`), `unknown field "x"`},
		{"bad_direction", iface(`{"type":"ip_access_group","acl":"A","direction":"both"}`), "direction must be one of"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Generate([]byte(tc.json))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectErr)
		})
	}
}

func TestRoundTrip(t *testing.T) {
	doc := `[
		{"type":"show","target":"interfaces","interface":"Gi1/0/1","pipe":{"type":"include","pattern":"rate"}},
		{"type":"interface","name":"Gi1/0/1","default_first":true,"commands":[
			{"type":"description","value":null},
			{"type":"channel_group","value":{"group":2,"mode":"passive"}},
			{"type":"shutdown","value":false}
		]}
	]`
	block, err := Parse([]byte(doc))
	require.NoError(t, err)
	encoded, err := json.Marshal(block)
	require.NoError(t, err)
	assert.JSONEq(t, doc, string(encoded))

	block2, err := Parse(encoded)
	require.NoError(t, err)
	assert.Equal(t, block, block2)
}

func TestRenderConstructed(t *testing.T) {
	enabled := true
	block := CommandBlock{
		&InterfaceCommand{
			Name: "Vlan10",
			Commands: []InterfaceSubCommand{
				&DescriptionCommand{Value: Some("users")},
				&VlanSettingCommand{Type: "switchport_access_vlan", Value: Null[int]()},
				&ToggleCommand{Type: "shutdown", Value: &enabled},
			},
		},
	}
	lines, err := block.Render()
	require.NoError(t, err)
	assert.Equal(t, []string{
		"configure terminal",
		"interface Vlan10",
		" description users",
		" no switchport access vlan",
		" shutdown",
		"end",
	}, lines)
}
