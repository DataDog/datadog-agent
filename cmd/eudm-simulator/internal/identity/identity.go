// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package identity rewrites captured identities without consulting the replay host.
package identity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// Wireless is a shared radio identity supplied by the access-point model.
type Wireless struct{ BSSID, SSID string }

// Map is immutable and safe to use concurrently on independently owned samples.
type Map struct {
	Hostname, UUID, ClientMAC, NetworkID, RunTag string
	runID, scope                                 string
	valid                                        bool
}

var runPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var placeholder = regexp.MustCompile(`[a-z_]+-[0-9a-f]{32}`)

// New derives a device identity from its declaration, independent of scheduling.
func New(runID string, seed uint64, cohort string, ordinal int) *Map {
	scope := string(hash(runID, strconv.FormatUint(seed, 10), cohort, strconv.Itoa(ordinal)))
	m := &Map{runID: runID, scope: scope, valid: runPattern.MatchString(runID) && ordinal >= 0}
	m.Hostname = "eudm-" + runID + "-" + strconv.FormatInt(int64(ordinal), 36)
	m.UUID = uuid(hash(scope, "host"))
	m.ClientMAC = mac(hash(scope, "client"))
	m.NetworkID = m.token("network", "host")
	m.RunTag = "eudm_run_id:" + runID
	return m
}

func hash(parts ...string) []byte {
	h := sha256.New()
	for _, part := range parts {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(part)))
		h.Write(size[:])
		h.Write([]byte(part))
	}
	return h.Sum(nil)
}

func uuid(b []byte) string {
	b = slices.Clone(b[:16])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	t := hex.EncodeToString(b)
	return t[:8] + "-" + t[8:12] + "-" + t[12:16] + "-" + t[16:20] + "-" + t[20:]
}

func mac(b []byte) string {
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", b[0]&0xfc|0x02, b[1], b[2], b[3], b[4], b[5])
}

func token(scope, kind, value string) string {
	return kind + "-" + hex.EncodeToString(hash(scope, kind, value)[:12])
}

func (m *Map) token(kind, value string) string { return token(m.scope, kind, value) }

// Namespace isolates all NDM resources belonging to one opaque run.
func Namespace(runID string) string { return "eudm-" + runID }

// RadioMAC is shared by an AP radio and every client associated with it.
// A configured BSSID is scoped too, so concurrent runs cannot share a resource.
func RadioMAC(runID, ap, radio, configured string) string {
	if parsed, err := net.ParseMAC(configured); err == nil {
		configured = parsed.String()
	}
	return mac(hash(runID, "radio", ap, radio, configured))
}

// SSID scopes a captured or configured network name while preserving sharing.
func SSID(runID, value string) string { return token(runID, "ssid", value) }

func (m *Map) pid(value int32) int32 {
	if value == 0 {
		return 0
	}
	// A translation modulo MaxInt32 is injective for sanitized positive PIDs;
	// it needs no mutable allocation table or collection-order assumptions.
	offset := int64(binary.BigEndian.Uint32(hash(m.scope, "pid")[:4])) % math.MaxInt32
	return int32((int64(value)-1+offset)%math.MaxInt32 + 1)
}

func ip(scope, value string) string {
	if value == "" {
		return ""
	}
	address, err := netip.ParseAddr(value)
	if err == nil {
		address = address.Unmap()
		value = address.String()
	}
	b := hash(scope, "ip", value)
	if address.Is6() {
		return fmt.Sprintf("2001:db8:%x:%x:%x:%x:%x:%x", binary.BigEndian.Uint16(b[:2]), binary.BigEndian.Uint16(b[2:4]), binary.BigEndian.Uint16(b[4:6]), binary.BigEndian.Uint16(b[6:8]), binary.BigEndian.Uint16(b[8:10]), binary.BigEndian.Uint16(b[10:12]))
	}
	return fmt.Sprintf("10.%d.%d.%d", b[0], b[1], b[2])
}

func (m *Map) replaceTokens(value string) string {
	return placeholder.ReplaceAllStringFunc(value, func(value string) string {
		kind, _, _ := strings.Cut(value, "-")
		scope := m.scope
		// Application and hardware model labels describe shared profiles.
		switch kind {
		case "process", "software", "software_version", "publisher", "cpu_model", "cpu_vendor", "device_model", "hardware_vendor", "domain":
			scope = m.runID
		}
		return token(scope, kind, value)
	})
}

func (m *Map) path(value string) string {
	value = m.replaceTokens(value)
	value = strings.ReplaceAll(value, `\\capture-server\`, `\\`+token(m.runID, "server", "capture")+`\`)
	value = strings.ReplaceAll(value, `C:\capture`, `C:\eudm\`+m.Hostname)
	return strings.ReplaceAll(value, "/capture", "/eudm/"+m.Hostname)
}

func (m *Map) tags(baseline []string, group schema.GroupDef, wireless *Wireless) []string {
	result := []string{m.RunTag, "infra_mode:end_user_device"}
	for _, tag := range baseline {
		key, value, ok := strings.Cut(tag, ":")
		if !ok {
			continue
		}
		switch key {
		case "eudm_run_id", "run_id", "infra_mode", "infrastructure_mode", "host", "host_id", "scenario", "scenario_name", "cohort", "affected", "affected_status", "expected", "expectation", "conclusion":
			continue
		case "bssid":
			if wireless != nil {
				value = wireless.BSSID
			} else {
				if group.BSSID != "" {
					value = group.BSSID
				}
				value = RadioMAC(m.runID, "", "", value)
			}
		case "ssid":
			if wireless != nil {
				value = wireless.SSID
			} else {
				if group.SSID != "" {
					value = group.SSID
				}
				value = SSID(m.runID, value)
			}
		case "mac_address", "client_mac":
			value = m.ClientMAC
		case "pagefile_path":
			value = m.path(value)
		default:
			value = m.replaceTokens(value)
		}
		result = append(result, key+":"+value)
	}
	result = append(result, group.Tags...)
	slices.Sort(result)
	return slices.Compact(result)
}

// Apply rewrites an owned sample after scenario overlays. It never clones,
// changes resource values, or reads host environment state.
func (m *Map) Apply(sample *telemetry.Sample, group schema.GroupDef, wireless *Wireless) error {
	if !m.valid || sample == nil {
		return errors.New("identity requires a valid opaque run ID, ordinal and sample")
	}
	if wireless != nil {
		if _, err := net.ParseMAC(wireless.BSSID); err != nil || wireless.SSID == "" {
			return errors.New("wireless identity requires a BSSID and SSID")
		}
	}
	for _, serie := range sample.Metrics {
		serie.Host = m.Hostname
		serie.Device = m.replaceTokens(serie.Device)
		serie.Tags = tagset.CompositeTagsFromSlice(m.tags(serie.Tags.UnsafeToReadOnlySliceString(), group, wireless))
	}
	if inventory := sample.Inventory; inventory != nil {
		inventory.Hostname, inventory.UUID = m.Hostname, m.UUID
		if h := inventory.SystemInfo; h != nil {
			h.Manufacturer, h.ModelNumber = m.replaceTokens(h.Manufacturer), m.replaceTokens(h.ModelNumber)
			h.ModelName, h.Identifier = m.replaceTokens(h.ModelName), m.replaceTokens(h.Identifier)
			if h.SerialNumber != "" {
				h.SerialNumber = m.token("serial", h.SerialNumber)
			}
		}
		if h := inventory.Host; h != nil {
			h.IPAddress, h.IPv6Address = ip(m.scope, h.IPAddress), ip(m.scope, h.IPv6Address)
			if h.MacAddress != "" {
				h.MacAddress = m.ClientMAC
			}
			h.CPUVendor, h.CPUModel = m.replaceTokens(h.CPUVendor), m.replaceTokens(h.CPUModel)
		}
	}
	if h := sample.HostMetadata; h != nil {
		h.Hostname, h.UUID = m.Hostname, m.UUID
		encoded, _ := json.Marshal(m.Hostname)
		if h.Meta == nil {
			h.Meta = map[string]json.RawMessage{}
		}
		h.Meta["hostname"], h.Meta["socket-hostname"] = encoded, slices.Clone(encoded)
		if h.Network == nil {
			h.Network = map[string]string{}
		}
		h.Network["network-id"] = m.NetworkID
		h.HostTags = map[string][]string{"system": m.tags(h.HostTags["system"], group, nil)}
		if h.Gohai != "" {
			var gohai map[string]map[string]any
			if err := json.Unmarshal([]byte(h.Gohai), &gohai); err != nil {
				return errors.New("cannot rewrite captured gohai")
			}
			for _, fields := range gohai {
				for key, value := range fields {
					text, ok := value.(string)
					if !ok {
						continue
					}
					switch key {
					case "hostname":
						fields[key] = m.Hostname
					case "hardware_uuid":
						fields[key] = m.UUID
					case "model_name", "vendor_id":
						fields[key] = m.replaceTokens(text)
					case "serial_number":
						fields[key] = m.token("serial", text)
					case "macaddress":
						fields[key] = m.ClientMAC
					case "ipaddress", "ipaddressv6":
						fields[key] = ip(m.scope, text)
					}
				}
			}
			data, err := json.Marshal(gohai)
			if err != nil {
				return errors.New("cannot encode rewritten gohai")
			}
			h.Gohai = string(data)
		}
	}
	if p := sample.Processes; p != nil {
		p.HostName, p.NetworkId = m.Hostname, m.NetworkID
		if p.Info != nil {
			p.Info.Uuid = m.UUID
		}
		for _, process := range p.Processes {
			process.Pid, process.NsPid = m.pid(process.Pid), m.pid(process.NsPid)
			process.Tags = m.tags(process.Tags, group, nil)
			if cmd := process.Command; cmd != nil {
				cmd.Comm, cmd.Exe, cmd.Cwd, cmd.Root = m.replaceTokens(cmd.Comm), m.path(cmd.Exe), m.path(cmd.Cwd), m.path(cmd.Root)
				cmd.Ppid, cmd.Pgroup = m.pid(cmd.Ppid), m.pid(cmd.Pgroup)
				for i := range cmd.Args {
					cmd.Args[i] = m.path(cmd.Args[i])
				}
			}
			if process.User != nil {
				process.User.Name = m.replaceTokens(process.User.Name)
			}
		}
	}
	if c := sample.Connections; c != nil {
		c.HostName, c.NetworkId = m.Hostname, m.NetworkID
		if err := m.connectionsDNS(c); err != nil {
			return err
		}
		for _, connection := range c.Connections {
			connection.Pid = m.pid(connection.Pid)
			if connection.Laddr != nil {
				connection.Laddr.Ip = ip(m.scope, connection.Laddr.Ip)
				connection.Laddr.HostName = m.Hostname
			}
			if connection.Raddr != nil {
				scope := m.runID
				if connection.IntraHost {
					scope = m.scope
					connection.Raddr.HostName = m.Hostname
				}
				connection.Raddr.Ip = ip(scope, connection.Raddr.Ip)
			}
			if connection.RemoteNetworkId != "" {
				connection.RemoteNetworkId = token(m.runID, "network", connection.RemoteNetworkId)
			}
		}
	}
	if software := sample.Software; software != nil {
		software.Hostname = m.Hostname
		for i := range software.Metadata.Software {
			entry := &software.Metadata.Software[i]
			entry.DisplayName, entry.Publisher = m.replaceTokens(entry.DisplayName), m.replaceTokens(entry.Publisher)
			entry.Version = m.replaceTokens(entry.Version)
			entry.UserSID = m.replaceTokens(entry.UserSID)
			if entry.ProductCode != "" {
				if group.OS == "windows" {
					entry.ProductCode = "{" + uuid(hash(m.scope, "product", entry.ProductCode)) + "}"
				} else {
					entry.ProductCode = m.token("product", entry.ProductCode)
				}
			}
			for j := range entry.InstallPaths {
				entry.InstallPaths[j] = m.path(entry.InstallPaths[j])
			}
		}
	}
	return nil
}
