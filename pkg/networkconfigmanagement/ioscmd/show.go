// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ioscmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ShowCommand is any show command.
type ShowCommand interface {
	Command
	isShow()
}

// OutputPipe is an output filter, rendered as `| <type> <regex>`.
type OutputPipe struct {
	// Type is one of "section", "include", "exclude", "begin", "count".
	Type string `json:"type"`
	// Section is the regex for "section" pipes.
	Section string `json:"section,omitempty"`
	// Pattern is the regex for all other pipes.
	Pattern string `json:"pattern,omitempty"`
}

func (p OutputPipe) render() (string, error) {
	switch p.Type {
	case "section":
		if p.Pattern != "" {
			return "", errors.New(`section pipe uses "section", not "pattern"`)
		}
		if err := checkText("pipe section", p.Section); err != nil {
			return "", err
		}
		return "section " + p.Section, nil
	case "include", "exclude", "begin", "count":
		if p.Section != "" {
			return "", fmt.Errorf(`%s pipe uses "pattern", not "section"`, p.Type)
		}
		if err := checkText("pipe pattern", p.Pattern); err != nil {
			return "", err
		}
		return p.Type + " " + p.Pattern, nil
	default:
		return "", fmt.Errorf("unknown pipe type %q", p.Type)
	}
}

// ShowBase holds the fields common to all show commands.
type ShowBase struct {
	// Type is the show command, e.g. "show version".
	Type string      `json:"type"`
	Pipe *OutputPipe `json:"pipe,omitempty"`
}

func (ShowBase) isConfig() bool { return false }
func (ShowBase) isShow()        {}

// finish joins parts and appends the pipe (if any).
func (s ShowBase) finish(parts ...string) ([]string, error) {
	cmd := strings.Join(parts, " ")
	if s.Pipe != nil {
		p, err := s.Pipe.render()
		if err != nil {
			return nil, err
		}
		cmd += " | " + p
	}
	return []string{cmd}, nil
}

// ShowRunningConfig is `show running-config [interface <name>]`.
type ShowRunningConfig struct {
	ShowBase
	Interface string `json:"interface,omitempty"`
}

func (s ShowRunningConfig) render() ([]string, error) {
	parts := []string{"show", "running-config"}
	if s.Interface != "" {
		if err := checkInterfaceName("interface", s.Interface); err != nil {
			return nil, err
		}
		parts = append(parts, "interface", s.Interface)
	}
	return s.finish(parts...)
}

// ShowInterfaces is `show interfaces [<name>] [<detail>]`.
type ShowInterfaces struct {
	ShowBase
	Interface string `json:"interface,omitempty"`
	// Detail is one of "status", "description", "switchport", "trunk".
	Detail string `json:"detail,omitempty"`
}

func (s ShowInterfaces) render() ([]string, error) {
	parts := []string{"show", "interfaces"}
	if s.Interface != "" {
		if err := checkInterfaceName("interface", s.Interface); err != nil {
			return nil, err
		}
		parts = append(parts, s.Interface)
	}
	if s.Detail != "" {
		if err := checkOneOf("detail", s.Detail, "status", "description", "switchport", "trunk"); err != nil {
			return nil, err
		}
		parts = append(parts, s.Detail)
	}
	return s.finish(parts...)
}

// ShowInterfacesCounters is `show interfaces [<name>] counters [<detail>]`.
type ShowInterfacesCounters struct {
	ShowBase
	Interface string `json:"interface,omitempty"`
	// Detail is one of "errors", "etherchannel", "protocol status", "trunk".
	Detail string `json:"detail,omitempty"`
}

func (s ShowInterfacesCounters) render() ([]string, error) {
	parts := []string{"show", "interfaces"}
	if s.Interface != "" {
		if err := checkInterfaceName("interface", s.Interface); err != nil {
			return nil, err
		}
		parts = append(parts, s.Interface)
	}
	parts = append(parts, "counters")
	if s.Detail != "" {
		if err := checkOneOf("detail", s.Detail, "errors", "etherchannel", "protocol status", "trunk"); err != nil {
			return nil, err
		}
		parts = append(parts, s.Detail)
	}
	return s.finish(parts...)
}

// ShowIPInterface is `show ip interface [brief | <name>]`.
type ShowIPInterface struct {
	ShowBase
	Brief     bool   `json:"brief,omitempty"`
	Interface string `json:"interface,omitempty"`
}

func (s ShowIPInterface) render() ([]string, error) {
	parts := []string{"show", "ip", "interface"}
	switch {
	case s.Brief && s.Interface != "":
		return nil, errors.New("brief and interface are mutually exclusive")
	case s.Brief:
		parts = append(parts, "brief")
	case s.Interface != "":
		if err := checkInterfaceName("interface", s.Interface); err != nil {
			return nil, err
		}
		parts = append(parts, s.Interface)
	}
	return s.finish(parts...)
}

// ShowIPRoute is `show ip route [vrf <vrf>] [<prefix>]`.
type ShowIPRoute struct {
	ShowBase
	Vrf    string `json:"vrf,omitempty"`
	Prefix string `json:"prefix,omitempty"`
}

func (s ShowIPRoute) render() ([]string, error) {
	parts := []string{"show", "ip", "route"}
	if s.Vrf != "" {
		if err := checkName("vrf", s.Vrf); err != nil {
			return nil, err
		}
		parts = append(parts, "vrf", s.Vrf)
	}
	if s.Prefix != "" {
		if err := checkIPv4("prefix", s.Prefix); err != nil {
			return nil, err
		}
		parts = append(parts, s.Prefix)
	}
	return s.finish(parts...)
}

// ShowIPArp is `show ip arp [vrf <vrf>] [<addr> | <interface>]`.
type ShowIPArp struct {
	ShowBase
	Vrf       string `json:"vrf,omitempty"`
	Addr      string `json:"addr,omitempty"`
	Interface string `json:"interface,omitempty"`
}

func (s ShowIPArp) render() ([]string, error) {
	parts := []string{"show", "ip", "arp"}
	if s.Vrf != "" {
		if err := checkName("vrf", s.Vrf); err != nil {
			return nil, err
		}
		parts = append(parts, "vrf", s.Vrf)
	}
	switch {
	case s.Addr != "" && s.Interface != "":
		return nil, errors.New("addr and interface are mutually exclusive")
	case s.Addr != "":
		if err := checkIPv4("addr", s.Addr); err != nil {
			return nil, err
		}
		parts = append(parts, s.Addr)
	case s.Interface != "":
		if err := checkInterfaceName("interface", s.Interface); err != nil {
			return nil, err
		}
		parts = append(parts, s.Interface)
	}
	return s.finish(parts...)
}

// ShowVlan is `show vlan [brief | id <vlan>]`.
type ShowVlan struct {
	ShowBase
	Brief bool `json:"brief,omitempty"`
	ID    int  `json:"id,omitempty"`
}

func (s ShowVlan) render() ([]string, error) {
	parts := []string{"show", "vlan"}
	switch {
	case s.Brief && s.ID != 0:
		return nil, errors.New("brief and id are mutually exclusive")
	case s.Brief:
		parts = append(parts, "brief")
	case s.ID != 0:
		if err := checkVlan("id", s.ID); err != nil {
			return nil, err
		}
		parts = append(parts, "id", strconv.Itoa(s.ID))
	}
	return s.finish(parts...)
}

// ShowNeighbors is `show cdp neighbors [detail]` or `show lldp neighbors [detail]`.
type ShowNeighbors struct {
	ShowBase
	Detail bool `json:"detail,omitempty"`
}

func (s ShowNeighbors) render() ([]string, error) {
	if err := checkOneOf("type", s.Type, "show cdp neighbors", "show lldp neighbors"); err != nil {
		return nil, err
	}
	parts := []string{s.Type}
	if s.Detail {
		parts = append(parts, "detail")
	}
	return s.finish(parts...)
}

// ShowMacAddressTable is `show mac address-table [interface <name> | vlan <id>]`.
type ShowMacAddressTable struct {
	ShowBase
	Interface string `json:"interface,omitempty"`
	Vlan      int    `json:"vlan,omitempty"`
}

func (s ShowMacAddressTable) render() ([]string, error) {
	parts := []string{"show", "mac", "address-table"}
	switch {
	case s.Interface != "" && s.Vlan != 0:
		return nil, errors.New("interface and vlan are mutually exclusive")
	case s.Interface != "":
		if err := checkInterfaceName("interface", s.Interface); err != nil {
			return nil, err
		}
		parts = append(parts, "interface", s.Interface)
	case s.Vlan != 0:
		if err := checkVlan("vlan", s.Vlan); err != nil {
			return nil, err
		}
		parts = append(parts, "vlan", strconv.Itoa(s.Vlan))
	}
	return s.finish(parts...)
}

// ShowSimple is a show command that takes no arguments, e.g. `show version`.
type ShowSimple struct {
	ShowBase
}

func (s ShowSimple) render() ([]string, error) {
	if err := checkOneOf("type", s.Type, "show startup-config", "show version", "show inventory", "show clock",
		"show logging", "show users", "show boot", "show dhcp lease"); err != nil {
		return nil, err
	}
	return s.finish(s.Type)
}
