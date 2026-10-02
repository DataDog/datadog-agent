// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package identity

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"reflect"
	"strings"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
)

func (m *Map) observeLocal(sample *telemetry.Sample) {
	if sample == nil {
		return
	}
	host := func(value string) {
		if value != "" {
			m.hosts[value] = true
		}
	}
	for _, serie := range sample.Metrics {
		host(serie.Host)
		for _, resource := range serie.Resources {
			if resource.Type == "host" {
				host(resource.Name)
			}
		}
		serie.Tags.ForEach(func(tag string) {
			key, value, _ := strings.Cut(tag, ":")
			if key == "mac_address" || key == "client_mac" {
				m.addMAC(value)
			}
		})
	}
	if p := sample.Processes; p != nil {
		host(p.HostName)
		if p.Host != nil {
			host(p.Host.Name)
		}
	}
	if s := sample.Software; s != nil {
		host(s.Hostname)
	}
	if c := sample.Connections; c != nil {
		host(c.HostName)
		for _, connection := range c.Connections {
			if connection.Laddr != nil {
				m.addIP(connection.Laddr.Ip)
			}
		}
	}
	if inventory := sample.Inventory; inventory != nil {
		host(inventory.Hostname)
		if h := inventory.Host; h != nil {
			m.addIP(h.IPAddress)
			m.addIP(h.IPv6Address)
			m.addMAC(h.MacAddress)
			if parsed, err := net.ParseMAC(h.MacAddress); err == nil {
				m.primaryMAC = parsed.String()
			}
			m.observeJSON([]byte(h.Interfaces))
		}
	}
	if h := sample.HostMetadata; h != nil {
		host(h.Hostname)
		m.observeJSON([]byte(h.Gohai))
		for key, value := range h.Network {
			if key == "public_ipv4" || key == "public-ipv4" {
				m.addIP(value)
			}
		}
		for key, value := range h.Meta {
			switch key {
			case "hostname", "socket-hostname", "socket-fqdn", "ec2-hostname", "agent-hostname", "legacy-resolution-hostname":
				var name string
				if json.Unmarshal(value, &name) == nil {
					host(name)
				}
			case "host_aliases":
				var names []string
				if json.Unmarshal(value, &names) == nil {
					for _, name := range names {
						host(name)
					}
				}
			}
		}
	}
}

func (m *Map) addIP(value string) {
	if address, err := netip.ParseAddr(value); err == nil && (address.IsGlobalUnicast() || address.IsLinkLocalUnicast()) && !address.IsLoopback() {
		m.localIPs[address.Unmap()] = true
	}
}

func (m *Map) addMAC(value string) {
	if parsed, err := net.ParseMAC(value); err == nil {
		m.localMACs[parsed.String()] = true
	}
}

func (m *Map) localIP(value string) string {
	address, err := netip.ParseAddr(value)
	if err != nil || !m.localIPs[address.Unmap()] {
		return value
	}
	return ip(m.scope, value)
}

func (m *Map) localMAC(value string) string {
	parsed, err := net.ParseMAC(value)
	if err != nil || !m.localMACs[parsed.String()] {
		return value
	}
	if parsed.String() == m.primaryMAC || (m.primaryMAC == "" && len(m.localMACs) == 1) {
		return m.ClientMAC
	}
	return mac(hash(m.scope, "interface", parsed.String()))
}

// walkJSON visits only string values. Numbers and unrelated fields retain their
// original JSON representation, including integers beyond float64 precision.
func walkJSON(raw json.RawMessage, key string, visit func(string, string) string) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("invalid captured local identity JSON")
	}
	switch trimmed[0] {
	case '{':
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return nil, errors.New("invalid captured local identity JSON")
		}
		for name, value := range fields {
			changed, err := walkJSON(value, name, visit)
			if err != nil {
				return nil, err
			}
			fields[name] = changed
		}
		return json.Marshal(fields)
	case '[':
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil {
			return nil, errors.New("invalid captured local identity JSON")
		}
		for i, value := range values {
			changed, err := walkJSON(value, key, visit)
			if err != nil {
				return nil, err
			}
			values[i] = changed
		}
		return json.Marshal(values)
	case '"':
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return nil, errors.New("invalid captured local identity JSON")
		}
		changed := visit(key, value)
		if changed != value {
			return json.Marshal(changed)
		}
	}
	return raw, nil
}

func (m *Map) observeJSON(raw []byte) {
	if len(raw) == 0 {
		return
	}
	_, _ = walkJSON(raw, "", func(key, value string) string {
		switch key {
		case "ipaddress", "ipaddressv6", "ipv4", "ipv6":
			m.addIP(value)
		case "macaddress":
			m.addMAC(value)
		case "hostname":
			if value != "" {
				m.hosts[value] = true
			}
		}
		return value
	})
}

func (m *Map) rewriteLocalJSON(raw []byte) ([]byte, error) {
	return walkJSON(raw, "", func(key, value string) string {
		switch key {
		case "hostname":
			if m.hosts[value] {
				return m.Hostname
			}
		case "hardware_uuid":
			if value != "" {
				return m.UUID
			}
		case "serial_number":
			if value != "" {
				return m.token("serial", "device")
			}
		case "macaddress":
			return m.localMAC(value)
		case "ipaddress", "ipaddressv6", "ipv4", "ipv6":
			return m.localIP(value)
		}
		return value
	})
}

func (m *Map) cloudResource(value string) string {
	if value == "" {
		return value
	}
	if last := strings.LastIndex(value, "/"); last >= 0 {
		return value[:last+1] + m.token("cloud_host", value[last+1:])
	}
	return m.token("cloud_host", value)
}

// Tables use the same device-local keys as their referring connections. Values
// and remote host entries stay intact, including entries unused by this chunk.
func (m *Map) connectionTables(c *model.CollectorConnections) error {
	if c.ContainerForPid != nil {
		containers := make(map[int32]string, len(c.ContainerForPid))
		for pid, container := range c.ContainerForPid {
			containers[m.pid(pid)] = container
		}
		c.ContainerForPid = containers
	}
	if c.ResolvedHostsByName == nil {
		return nil
	}
	remote := map[string]bool{}
	for _, connection := range c.Connections {
		if connection.Raddr != nil && !connection.IntraHost {
			remote[connection.Raddr.HostName] = true
		}
	}
	hosts := make(map[string]*model.Host, len(c.ResolvedHostsByName))
	for name, host := range c.ResolvedHostsByName {
		if !m.hosts[name] || remote[name] {
			hosts[name] = host
		}
	}
	for name, host := range c.ResolvedHostsByName {
		if !m.hosts[name] {
			continue
		}
		var local *model.Host
		if host != nil {
			owned := *host
			owned.Name = m.Hostname
			local = &owned
		}
		if previous, ok := hosts[m.Hostname]; ok && !reflect.DeepEqual(previous, local) {
			return errors.New("captured local host aliases have conflicting resolution data")
		}
		hosts[m.Hostname] = local
	}
	c.ResolvedHostsByName = hosts
	return nil
}
