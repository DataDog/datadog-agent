// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ioscmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
)

// HostnameCommand is `hostname <name>`.
type HostnameCommand struct {
	// Type is always "hostname".
	Type     string `json:"type"`
	Hostname string `json:"hostname"`
}

func (HostnameCommand) isConfig() bool { return true }

func (c HostnameCommand) render() ([]string, error) {
	if err := checkHostname("hostname", c.Hostname); err != nil {
		return nil, err
	}
	return []string{"hostname " + c.Hostname}, nil
}

// InterfaceCommand is `interface <name>` followed by indented sub-commands.
type InterfaceCommand struct {
	// Type is always "interface".
	Type string `json:"type"`
	Name string `json:"name"`
	// DefaultFirst issues `default interface <name>` before configuring the
	// interface, resetting it to factory defaults.
	DefaultFirst bool                  `json:"default_first,omitempty"`
	Commands     []InterfaceSubCommand `json:"commands"`
}

func (InterfaceCommand) isConfig() bool { return true }

func (c InterfaceCommand) render() ([]string, error) {
	if err := checkInterfaceName("interface name", c.Name); err != nil {
		return nil, err
	}
	var out []string
	if c.DefaultFirst {
		out = append(out, "default interface "+c.Name)
	}
	out = append(out, "interface "+c.Name)
	for i, sub := range c.Commands {
		if sub == nil {
			return nil, fmt.Errorf("interface %s: command %d: nil command", c.Name, i)
		}
		line, err := sub.render()
		if err != nil {
			return nil, fmt.Errorf("interface %s: command %d: %w", c.Name, i, err)
		}
		out = append(out, " "+line)
	}
	return out, nil
}

// UnmarshalJSON implements json.Unmarshaler, dispatching each sub-command on
// its "type" field.
func (c *InterfaceCommand) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type         string            `json:"type"`
		Name         string            `json:"name"`
		DefaultFirst bool              `json:"default_first"`
		Commands     []json.RawMessage `json:"commands"`
	}
	if err := decodeStrict(data, &raw); err != nil {
		return err
	}
	if raw.Commands == nil {
		return errors.New(`"commands" is required`)
	}
	cmds := make([]InterfaceSubCommand, 0, len(raw.Commands))
	for i, rawCmd := range raw.Commands {
		sub, err := decodeSubCommand(rawCmd)
		if err != nil {
			return fmt.Errorf("command %d: %w", i, err)
		}
		cmds = append(cmds, sub)
	}
	*c = InterfaceCommand{
		Type:         raw.Type,
		Name:         raw.Name,
		DefaultFirst: raw.DefaultFirst,
		Commands:     cmds,
	}
	return nil
}

func decodeSubCommand(raw json.RawMessage) (InterfaceSubCommand, error) {
	d, err := peek(raw)
	if err != nil {
		return nil, err
	}
	var sub InterfaceSubCommand
	if _, ok := toggleKeywords[d.Type]; ok {
		sub = &ToggleCommand{}
	} else if newSub, ok := subCommandTypes[d.Type]; ok {
		sub = newSub()
	} else {
		return nil, fmt.Errorf("unknown interface command type %q", d.Type)
	}
	if err := decodeStrict(raw, sub); err != nil {
		return nil, fmt.Errorf("%s: %w", d.Type, err)
	}
	return sub, nil
}

// InterfaceSubCommand is a command issued inside an `interface` block.
type InterfaceSubCommand interface {
	// render returns the CLI line for the command (without indentation).
	render() (string, error)
}

var subCommandTypes = map[string]func() InterfaceSubCommand{
	"description":                   func() InterfaceSubCommand { return &DescriptionCommand{} },
	"ip_addr":                       func() InterfaceSubCommand { return &IPAddressCommand{} },
	"ip_addr_dhcp":                  func() InterfaceSubCommand { return &IPAddressDHCPCommand{} },
	"no_ip_addr":                    func() InterfaceSubCommand { return &NoIPAddressCommand{} },
	"ipv6_addr":                     func() InterfaceSubCommand { return &IPv6AddressCommand{} },
	"ip_helper_address":             func() InterfaceSubCommand { return &IPHelperAddressCommand{} },
	"vrf_forwarding":                func() InterfaceSubCommand { return &VrfForwardingCommand{} },
	"switchport_mode":               func() InterfaceSubCommand { return &SwitchportModeCommand{} },
	"switchport_access_vlan":        func() InterfaceSubCommand { return &VlanSettingCommand{} },
	"switchport_voice_vlan":         func() InterfaceSubCommand { return &VlanSettingCommand{} },
	"switchport_trunk_native_vlan":  func() InterfaceSubCommand { return &VlanSettingCommand{} },
	"switchport_trunk_allowed_vlan": func() InterfaceSubCommand { return &SwitchportTrunkAllowedVlanCommand{} },
	"speed":                         func() InterfaceSubCommand { return &SpeedCommand{} },
	"duplex":                        func() InterfaceSubCommand { return &DuplexCommand{} },
	"mtu":                           func() InterfaceSubCommand { return &MtuCommand{} },
	"ip_mtu":                        func() InterfaceSubCommand { return &MtuCommand{} },
	"channel_group":                 func() InterfaceSubCommand { return &ChannelGroupCommand{} },
	"ip_access_group":               func() InterfaceSubCommand { return &IPAccessGroupCommand{} },
}

// negate prefixes cmd with "no " if remove is true.
func negate(remove bool, cmd string) string {
	if remove {
		return "no " + cmd
	}
	return cmd
}

// toggleKeywords maps each ToggleCommand type to its CLI keyword.
var toggleKeywords = map[string]string{
	"shutdown":                "shutdown",
	"switchport":              "switchport",
	"negotiation_auto":        "negotiation auto",
	"cdp_enable":              "cdp enable",
	"lldp_transmit":           "lldp transmit",
	"lldp_receive":            "lldp receive",
	"spanning_tree_portfast":  "spanning-tree portfast",
	"spanning_tree_bpduguard": "spanning-tree bpduguard enable",
	"ip_redirects":            "ip redirects",
	"ip_proxy_arp":            "ip proxy-arp",
	"ipv6_enable":             "ipv6 enable",
}

// ToggleCommand is a boolean setting: `<keyword>` if Value is true, or
// `no <keyword>` if it is false.
type ToggleCommand struct {
	Type  string `json:"type"`
	Value *bool  `json:"value"`
}

func (c ToggleCommand) render() (string, error) {
	keyword, ok := toggleKeywords[c.Type]
	if !ok {
		return "", fmt.Errorf("unknown toggle type %q", c.Type)
	}
	if c.Value == nil {
		return "", fmt.Errorf("%s: value is required", c.Type)
	}
	return negate(!*c.Value, keyword), nil
}

// DescriptionCommand is `description <text>`, or `no description` if Value is null.
type DescriptionCommand struct {
	// Type is always "description".
	Type  string           `json:"type"`
	Value Nullable[string] `json:"value"`
}

func (c DescriptionCommand) render() (string, error) {
	if !c.Value.Set {
		return "", errors.New("description: value is required")
	}
	if c.Value.Value == nil {
		return "no description", nil
	}
	if err := checkText("description", *c.Value.Value); err != nil {
		return "", err
	}
	return "description " + *c.Value.Value, nil
}

// IPAddressCommand is `ip address <addr> <mask> [secondary]`.
type IPAddressCommand struct {
	// Type is always "ip_addr".
	Type      string `json:"type"`
	Addr      string `json:"addr"`
	Mask      string `json:"mask"`
	Secondary bool   `json:"secondary,omitempty"`
}

func (c IPAddressCommand) render() (string, error) {
	if err := checkIPv4("addr", c.Addr); err != nil {
		return "", err
	}
	if err := checkIPv4Mask("mask", c.Mask); err != nil {
		return "", err
	}
	cmd := "ip address " + c.Addr + " " + c.Mask
	if c.Secondary {
		cmd += " secondary"
	}
	return cmd, nil
}

// IPAddressDHCPCommand is `ip address dhcp`.
type IPAddressDHCPCommand struct {
	// Type is always "ip_addr_dhcp".
	Type string `json:"type"`
}

func (IPAddressDHCPCommand) render() (string, error) { return "ip address dhcp", nil }

// NoIPAddressCommand is `no ip address`, which removes all IPv4 addresses.
type NoIPAddressCommand struct {
	// Type is always "no_ip_addr".
	Type string `json:"type"`
}

func (NoIPAddressCommand) render() (string, error) { return "no ip address", nil }

// IPv6AddressCommand is `ipv6 address <prefix> [eui-64]` or
// `ipv6 address <addr> link-local`.
type IPv6AddressCommand struct {
	// Type is always "ipv6_addr".
	Type string `json:"type"`
	// Prefix is in CIDR notation, except with the "link-local" modifier, where
	// it is a bare fe80::/10 address.
	Prefix string `json:"prefix"`
	// Modifier is "", "eui-64", or "link-local".
	Modifier string `json:"modifier,omitempty"`
	Remove   bool   `json:"remove,omitempty"`
}

func (c IPv6AddressCommand) render() (string, error) {
	switch c.Modifier {
	case "", "eui-64":
		p, err := netip.ParsePrefix(c.Prefix)
		if err != nil || !p.Addr().Is6() || p.Addr().Is4In6() {
			return "", fmt.Errorf("prefix must be an IPv6 prefix in CIDR notation, got %q", c.Prefix)
		}
	case "link-local":
		a, err := netip.ParseAddr(c.Prefix)
		if err != nil || !a.Is6() || !a.IsLinkLocalUnicast() {
			return "", fmt.Errorf("prefix must be a bare link-local IPv6 address, got %q", c.Prefix)
		}
	default:
		return "", fmt.Errorf("modifier must be \"eui-64\" or \"link-local\", got %q", c.Modifier)
	}
	cmd := "ipv6 address " + c.Prefix
	if c.Modifier != "" {
		cmd += " " + c.Modifier
	}
	return negate(c.Remove, cmd), nil
}

// IPHelperAddressCommand is `[no] ip helper-address <addr>`.
type IPHelperAddressCommand struct {
	// Type is always "ip_helper_address".
	Type   string `json:"type"`
	Addr   string `json:"addr"`
	Remove bool   `json:"remove,omitempty"`
}

func (c IPHelperAddressCommand) render() (string, error) {
	if err := checkIPv4("addr", c.Addr); err != nil {
		return "", err
	}
	return negate(c.Remove, "ip helper-address "+c.Addr), nil
}

// VrfForwardingCommand is `vrf forwarding <name>`, or `no vrf forwarding`
// if Value is null.
type VrfForwardingCommand struct {
	// Type is always "vrf_forwarding".
	Type  string           `json:"type"`
	Value Nullable[string] `json:"value"`
}

func (c VrfForwardingCommand) render() (string, error) {
	if !c.Value.Set {
		return "", errors.New("vrf_forwarding: value is required")
	}
	if c.Value.Value == nil {
		return "no vrf forwarding", nil
	}
	if err := checkName("vrf", *c.Value.Value); err != nil {
		return "", err
	}
	return "vrf forwarding " + *c.Value.Value, nil
}

// SwitchportModeCommand is `switchport mode <mode>`.
type SwitchportModeCommand struct {
	// Type is always "switchport_mode".
	Type string `json:"type"`
	// Mode is one of "access", "trunk", "dynamic auto", "dynamic desirable".
	Mode string `json:"mode"`
}

func (c SwitchportModeCommand) render() (string, error) {
	if err := checkOneOf("mode", c.Mode, "access", "trunk", "dynamic auto", "dynamic desirable"); err != nil {
		return "", err
	}
	return "switchport mode " + c.Mode, nil
}

// vlanSettingKeywords maps each VlanSettingCommand type to its CLI keyword.
var vlanSettingKeywords = map[string]string{
	"switchport_access_vlan":       "switchport access vlan",
	"switchport_voice_vlan":        "switchport voice vlan",
	"switchport_trunk_native_vlan": "switchport trunk native vlan",
}

// VlanSettingCommand is a single-VLAN setting such as
// `switchport access vlan <id>`, or its `no` form if Value is null.
type VlanSettingCommand struct {
	// Type is "switchport_access_vlan", "switchport_voice_vlan", or
	// "switchport_trunk_native_vlan".
	Type  string        `json:"type"`
	Value Nullable[int] `json:"value"`
}

func (c VlanSettingCommand) render() (string, error) {
	keyword, ok := vlanSettingKeywords[c.Type]
	if !ok {
		return "", fmt.Errorf("unknown VLAN setting type %q", c.Type)
	}
	if !c.Value.Set {
		return "", fmt.Errorf("%s: value is required", c.Type)
	}
	if c.Value.Value == nil {
		return "no " + keyword, nil
	}
	if err := checkVlan("value", *c.Value.Value); err != nil {
		return "", err
	}
	return keyword + " " + strconv.Itoa(*c.Value.Value), nil
}

// SwitchportTrunkAllowedVlanCommand is
// `switchport trunk allowed vlan [add | remove | except] <vlans>`.
type SwitchportTrunkAllowedVlanCommand struct {
	// Type is always "switchport_trunk_allowed_vlan".
	Type string `json:"type"`
	// Action is "set" (the default), "add", "remove", or "except".
	Action string `json:"action,omitempty"`
	// Vlans is a VLAN list like "10,20,30-40", or "all" or "none" (only
	// with the "set" action).
	Vlans string `json:"vlans"`
}

func (c SwitchportTrunkAllowedVlanCommand) render() (string, error) {
	action := c.Action
	if action == "" {
		action = "set"
	}
	if err := checkOneOf("action", action, "set", "add", "remove", "except"); err != nil {
		return "", err
	}
	if c.Vlans == "all" || c.Vlans == "none" {
		if action != "set" {
			return "", fmt.Errorf("vlans %q is only valid with the \"set\" action", c.Vlans)
		}
	} else if err := checkVlanList("vlans", c.Vlans); err != nil {
		return "", err
	}
	cmd := "switchport trunk allowed vlan "
	if action != "set" {
		cmd += action + " "
	}
	return cmd + c.Vlans, nil
}

// SpeedCommand is `speed <value>`, or `no speed` if Value is null.
type SpeedCommand struct {
	// Type is always "speed".
	Type  string           `json:"type"`
	Value Nullable[string] `json:"value"`
}

func (c SpeedCommand) render() (string, error) {
	if !c.Value.Set {
		return "", errors.New("speed: value is required")
	}
	if c.Value.Value == nil {
		return "no speed", nil
	}
	if err := checkOneOf("speed", *c.Value.Value, "auto", "10", "100", "1000", "2500", "5000", "10000", "25000", "40000", "100000"); err != nil {
		return "", err
	}
	return "speed " + *c.Value.Value, nil
}

// DuplexCommand is `duplex <value>`, or `no duplex` if Value is null.
type DuplexCommand struct {
	// Type is always "duplex".
	Type  string           `json:"type"`
	Value Nullable[string] `json:"value"`
}

func (c DuplexCommand) render() (string, error) {
	if !c.Value.Set {
		return "", errors.New("duplex: value is required")
	}
	if c.Value.Value == nil {
		return "no duplex", nil
	}
	if err := checkOneOf("duplex", *c.Value.Value, "auto", "full", "half"); err != nil {
		return "", err
	}
	return "duplex " + *c.Value.Value, nil
}

// MtuCommand is `mtu <bytes>` or `ip mtu <bytes>`, or the `no` form if Value is null.
type MtuCommand struct {
	// Type is "mtu" or "ip_mtu".
	Type  string        `json:"type"`
	Value Nullable[int] `json:"value"`
}

func (c MtuCommand) render() (string, error) {
	var keyword string
	switch c.Type {
	case "mtu":
		keyword = "mtu"
	case "ip_mtu":
		keyword = "ip mtu"
	default:
		return "", fmt.Errorf("unknown MTU type %q", c.Type)
	}
	if !c.Value.Set {
		return "", fmt.Errorf("%s: value is required", c.Type)
	}
	if c.Value.Value == nil {
		return "no " + keyword, nil
	}
	if *c.Value.Value <= 0 {
		return "", fmt.Errorf("%s must be positive, got %d", c.Type, *c.Value.Value)
	}
	return keyword + " " + strconv.Itoa(*c.Value.Value), nil
}

// ChannelGroup is the value of a ChannelGroupCommand.
type ChannelGroup struct {
	Group int `json:"group"`
	// Mode is one of "active", "passive", "on", "auto", "desirable".
	Mode string `json:"mode"`
}

// ChannelGroupCommand is `channel-group <group> mode <mode>`, or
// `no channel-group` if Value is null.
type ChannelGroupCommand struct {
	// Type is always "channel_group".
	Type  string                 `json:"type"`
	Value Nullable[ChannelGroup] `json:"value"`
}

func (c ChannelGroupCommand) render() (string, error) {
	if !c.Value.Set {
		return "", errors.New("channel_group: value is required")
	}
	if c.Value.Value == nil {
		return "no channel-group", nil
	}
	v := *c.Value.Value
	if v.Group <= 0 {
		return "", fmt.Errorf("group must be positive, got %d", v.Group)
	}
	if err := checkOneOf("mode", v.Mode, "active", "passive", "on", "auto", "desirable"); err != nil {
		return "", err
	}
	return fmt.Sprintf("channel-group %d mode %s", v.Group, v.Mode), nil
}

// IPAccessGroupCommand is `[no] ip access-group <acl> {in | out}`.
type IPAccessGroupCommand struct {
	// Type is always "ip_access_group".
	Type      string `json:"type"`
	ACL       string `json:"acl"`
	Direction string `json:"direction"`
	Remove    bool   `json:"remove,omitempty"`
}

func (c IPAccessGroupCommand) render() (string, error) {
	if err := checkName("acl", c.ACL); err != nil {
		return "", err
	}
	if err := checkOneOf("direction", c.Direction, "in", "out"); err != nil {
		return "", err
	}
	return negate(c.Remove, "ip access-group "+c.ACL+" "+c.Direction), nil
}
