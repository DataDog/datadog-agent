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
// Baseline addresses identify the device; remote services retain their native identities.
type Map struct {
	Hostname, UUID, ClientMAC, RunTag string
	runID, scope                      string
	valid                             bool
	*Baseline
}

var runPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// New derives a device identity and discovers local addresses from captured
// samples before replay workers start. Baselines are never modified.
func New(runID string, seed uint64, cohort string, ordinal int, baseline ...*telemetry.Sample) *Map {
	observed := NewBaseline()
	for _, sample := range baseline {
		observed.Observe(sample)
	}
	return NewWithBaseline(runID, seed, cohort, ordinal, observed)
}

// NewWithBaseline reuses compact identity evidence without retaining or
// rescanning decoded telemetry for each simulated device. The baseline must
// not be changed after it is shared with replay workers.
func NewWithBaseline(runID string, seed uint64, cohort string, ordinal int, baseline *Baseline) *Map {
	if baseline == nil {
		baseline = NewBaseline()
	}
	scope := string(hash(runID, strconv.FormatUint(seed, 10), cohort, strconv.Itoa(ordinal)))
	m := &Map{runID: runID, scope: scope, valid: runPattern.MatchString(runID) && ordinal >= 0,
		Baseline: baseline}
	m.Hostname = "eudm-" + runID + "-" + strconv.FormatInt(int64(ordinal), 36)
	m.UUID = uuid(hash(scope, "host"))
	m.ClientMAC = mac(hash(scope, "client"))
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

// SSID preserves the native or explicitly configured network name. AP resource
// isolation comes from its namespace and radio MAC, not from renaming the network.
func SSID(_ string, value string) string { return value }

func (m *Map) pid(value int32) int32 {
	if value <= 0 {
		return value
	}
	// A translation modulo MaxInt32 is injective for positive PIDs;
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

func (m *Map) tags(baseline []string, group schema.GroupDef, wireless *Wireless) []string {
	result := make([]string, 0, len(baseline)+len(group.Tags)+1)
	for _, tag := range baseline {
		key, value, ok := strings.Cut(tag, ":")
		if !ok {
			result = append(result, tag)
			continue
		}
		switch key {
		case "eudm_run_id":
			continue
		case "host", "hostname":
			if m.hosts[value] {
				value = m.Hostname
			}
		case "bssid":
			if wireless != nil {
				value = wireless.BSSID
			} else if group.BSSID != "" {
				value = group.BSSID
			}
		case "ssid":
			if wireless != nil {
				value = wireless.SSID
			} else if group.SSID != "" {
				value = group.SSID
			}
		case "mac_address", "client_mac":
			value = m.localMAC(value)
		}
		result = append(result, key+":"+value)
	}
	result = append(result, group.Tags...)
	return append(result, m.RunTag)
}

// Apply changes only simulated device identity. Native application, user,
// resource, and network attributes remain intact unless a scenario overrides them.
func (m *Map) Apply(sample *telemetry.Sample, group schema.GroupDef, wireless *Wireless) error {
	if !m.valid || sample == nil {
		return errors.New("identity requires a valid opaque run ID, ordinal and sample")
	}
	if len(m.hosts) == 0 {
		// Standalone callers can provide one sample without shared baselines.
		// Keep the map immutable when several streams invoke Apply concurrently.
		local := *m
		local.Baseline = NewBaseline()
		local.Observe(sample)
		m = &local
	}
	if wireless != nil {
		if _, err := net.ParseMAC(wireless.BSSID); err != nil || wireless.SSID == "" {
			return errors.New("wireless identity requires a BSSID and SSID")
		}
	}
	for _, serie := range sample.Metrics {
		serie.Host = m.Hostname
		for i := range serie.Resources {
			if serie.Resources[i].Type == "host" && m.hosts[serie.Resources[i].Name] {
				serie.Resources[i].Name = m.Hostname
			}
		}
		serie.Tags = tagset.CompositeTagsFromSlice(m.tags(serie.Tags.UnsafeToReadOnlySliceString(), group, wireless))
	}
	if inventory := sample.Inventory; inventory != nil {
		inventory.Hostname, inventory.UUID = m.Hostname, m.UUID
		if h := inventory.SystemInfo; h != nil && h.SerialNumber != "" {
			h.SerialNumber = m.token("serial", "device")
		}
		if h := inventory.Host; h != nil {
			h.IPAddress, h.IPv6Address = m.localIP(h.IPAddress), m.localIP(h.IPv6Address)
			h.MacAddress = m.localMAC(h.MacAddress)
			if h.Interfaces != "" {
				encoded, err := m.rewriteLocalJSON([]byte(h.Interfaces))
				if err != nil {
					return err
				}
				h.Interfaces = string(encoded)
			}
			if h.HypervisorGuestUUID != "" {
				h.HypervisorGuestUUID = m.UUID
			}
			if h.DmiProductUUID != "" {
				h.DmiProductUUID = m.UUID
			}
			if h.DmiBoardAssetTag != "" {
				h.DmiBoardAssetTag = m.token("asset", h.DmiBoardAssetTag)
			}
			if h.CloudProviderHostID != "" {
				h.CloudProviderHostID = m.token("cloud_host", h.CloudProviderHostID)
			}
			h.CanonicalCloudResourceID = m.cloudResource(h.CanonicalCloudResourceID)
		}
	}
	if h := sample.HostMetadata; h != nil {
		h.Hostname, h.UUID = m.Hostname, m.UUID
		encoded, _ := json.Marshal(m.Hostname)
		if h.Meta == nil {
			h.Meta = map[string]json.RawMessage{}
		}
		h.Meta["hostname"], h.Meta["socket-hostname"] = encoded, slices.Clone(encoded)
		for key, value := range h.Meta {
			var native string
			switch key {
			case "socket-fqdn", "ec2-hostname", "agent-hostname", "legacy-resolution-hostname":
				if json.Unmarshal(value, &native) == nil && m.hosts[native] {
					h.Meta[key] = slices.Clone(encoded)
				}
			case "instance-id":
				if json.Unmarshal(value, &native) == nil && native != "" {
					h.Meta[key], _ = json.Marshal(m.token("cloud_host", native))
				}
			case "ccrid":
				if json.Unmarshal(value, &native) == nil && native != "" {
					h.Meta[key], _ = json.Marshal(m.cloudResource(native))
				}
			case "host_aliases":
				var aliases []string
				if json.Unmarshal(value, &aliases) == nil {
					for i := range aliases {
						if m.hosts[aliases[i]] {
							aliases[i] = m.Hostname
						}
					}
					h.Meta[key], _ = json.Marshal(aliases)
				}
			}
		}
		for key, value := range h.Network {
			if key == "public_ipv4" || key == "public-ipv4" {
				h.Network[key] = m.localIP(value)
			}
		}
		for key, tags := range h.HostTags {
			h.HostTags[key] = m.tags(tags, group, nil)
		}
		if h.Gohai != "" {
			encoded, err := m.rewriteLocalJSON([]byte(h.Gohai))
			if err != nil {
				return err
			}
			h.Gohai = string(encoded)
		}
	}
	if p := sample.Processes; p != nil {
		p.HostName = m.Hostname
		if p.Host != nil {
			p.Host.Name = m.Hostname
		}
		if p.Info != nil {
			p.Info.Uuid = m.UUID
		}
		for _, process := range p.Processes {
			process.Pid, process.NsPid = m.pid(process.Pid), m.pid(process.NsPid)
			process.Tags = m.tags(process.Tags, group, nil)
			if cmd := process.Command; cmd != nil {
				cmd.Ppid, cmd.Pgroup = m.pid(cmd.Ppid), m.pid(cmd.Pgroup)
			}
		}
	}
	if c := sample.Connections; c != nil {
		c.HostName = m.Hostname
		if err := m.connectionsDNS(c); err != nil {
			return err
		}
		if err := m.connectionTables(c); err != nil {
			return err
		}
		for _, connection := range c.Connections {
			connection.Pid = m.pid(connection.Pid)
			if connection.Laddr != nil {
				connection.Laddr.Ip = m.localIP(connection.Laddr.Ip)
				if m.hosts[connection.Laddr.HostName] {
					connection.Laddr.HostName = m.Hostname
				}
			}
			if connection.Raddr != nil && connection.IntraHost {
				connection.Raddr.Ip = m.localIP(connection.Raddr.Ip)
				if m.hosts[connection.Raddr.HostName] {
					connection.Raddr.HostName = m.Hostname
				}
			}
		}
	}
	if software := sample.Software; software != nil {
		software.Hostname = m.Hostname
	}
	return nil
}
