// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package capture transforms native typed output before it reaches persistence.
package capture

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// Sanitizer has an ephemeral per-capture HMAC key. Neither raw identities nor
// the key are persisted. The same identity gets the same placeholder in every
// stream, even when collectors call the transformer concurrently.
type Sanitizer struct {
	key  [32]byte
	mu   sync.Mutex
	pids map[int32]int32
}

func NewSanitizer() (*Sanitizer, error) {
	s := &Sanitizer{pids: map[int32]int32{}}
	_, err := rand.Read(s.key[:])
	return s, err
}

func (s *Sanitizer) token(kind, value string) string {
	if value == "" {
		return ""
	}
	h := hmac.New(sha256.New, s.key[:])
	h.Write([]byte(kind))
	h.Write([]byte{0})
	h.Write([]byte(value))
	return kind + "-" + hex.EncodeToString(h.Sum(nil)[:16])
}

func (s *Sanitizer) uuid(value string) string {
	if value == "" {
		return ""
	}
	token := strings.TrimPrefix(s.token("uuid", strings.ToLower(value)), "uuid-")
	return fmt.Sprintf("%s-%s-%s-%s-%s", token[:8], token[8:12], token[12:16], token[16:20], token[20:])
}

func (s *Sanitizer) pid(pid int32) int32 {
	if pid == 0 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if mapped, exists := s.pids[pid]; exists {
		return mapped
	}
	mapped := int32(len(s.pids) + 100)
	s.pids[pid] = mapped
	return mapped
}

func (s *Sanitizer) mac(value string) string {
	if parsed, err := net.ParseMAC(value); err == nil {
		value = parsed.String()
	}
	t := s.token("mac", strings.ToLower(value))
	if t == "" {
		return ""
	}
	digest := strings.TrimPrefix(t, "mac-")
	return "02:" + strings.Join([]string{digest[0:2], digest[2:4], digest[4:6], digest[6:8], digest[8:10]}, ":")
}

func (s *Sanitizer) ip(value string) string {
	if value == "" {
		return ""
	}
	parsed, err := netip.ParseAddr(value)
	if err == nil {
		parsed = parsed.Unmap()
		value = parsed.String()
	}
	t := strings.TrimPrefix(s.token("ip", value), "ip-")
	if err == nil && parsed.Is6() {
		return "2001:db8:" + t[0:4] + ":" + t[4:8] + ":" + t[8:12] + ":" + t[12:16] + ":" + t[16:20] + ":" + t[20:24]
	}
	b, _ := hex.DecodeString(t[:6])
	return fmt.Sprintf("10.%d.%d.%d", b[0], b[1], b[2])
}

var (
	versionPattern = regexp.MustCompile(`^[0-9]{1,8}(?:\.[0-9]{1,8}){0,5}$`)
	// The native macOS inventory includes Apple's build, for example 15.6
	// (24G84). It is structured version evidence, not arbitrary release text.
	macOSVersionPattern    = regexp.MustCompile(`^[0-9]{1,4}(?:\.[0-9]{1,4}){0,3} \([0-9]{1,3}[A-Z][0-9]{1,6}[a-z]?\)$`)
	windowsPlatformPattern = regexp.MustCompile(`^(?:Microsoft )?Windows (?:10|11|Server (?:2016|2019|2022|2025))(?: (?:Home(?: Single Language)?|Pro(?: for Workstations| Education)?|Professional|Enterprise(?: (?:LTSC|LTSB)(?: 20[0-9]{2})?)?|Education|Standard|Datacenter|Essentials|IoT Enterprise(?: LTSC)?))?(?: N)?(?: Evaluation)?$`)
	cpuCorePattern         = regexp.MustCompile(`^(?:cpu(?:-total|[0-9]{1,5})|[0-9]{1,5}|total)$`)
	appleCPUModelPattern   = regexp.MustCompile(`^Apple_M[1-9][0-9]?(?:_(?:Pro|Max|Ultra))?$`)
	appleDevicePattern     = regexp.MustCompile(`^(?:MacBookPro|MacBookAir|MacBook|Macmini|MacPro|Mac|iMacPro|iMac|MacStudio)[0-9]{1,3},[0-9]{1,3}$`)
)

func safeVersion(value string) string {
	if versionPattern.MatchString(value) {
		return value
	}
	return ""
}

func safeOSVersion(value string) string {
	if macOSVersionPattern.MatchString(value) {
		return value
	}
	return safeVersion(value)
}

func safeArchitecture(value string) string {
	if slices.Contains([]string{"arm64", "amd64", "x86_64", "aarch64", "arm", "386", "i386", "i686"}, value) {
		return value
	}
	return ""
}

func safeOSPlatform(value string) string {
	if slices.Contains([]string{"darwin", "Darwin", "macos", "macOS", "windows", "Windows", "win32"}, value) || windowsPlatformPattern.MatchString(value) {
		return value
	}
	return ""
}

func safeOSFamily(value string) string {
	if slices.Contains([]string{"darwin", "windows", "Standalone Workstation", "Domain Joined Workstation", "Standalone Server", "Domain Joined Server", "Domain Controller", "Backup Domain Controller"}, value) {
		return value
	}
	return ""
}

func (s *Sanitizer) hostTags(tags []string) []string {
	result := []string{"infra_mode:end_user_device"}
	for _, tag := range tags {
		key, value, ok := strings.Cut(tag, ":")
		if !ok || value == "" {
			continue
		}
		switch key {
		case "os_name":
			if value != "darwin" && value != "windows" {
				continue
			}
		case "os_version":
			if safeVersion(value) == "" {
				continue
			}
		case "total_memory_gb":
			if n, err := strconv.ParseUint(value, 10, 32); err != nil || n == 0 {
				continue
			}
		case "cpu_model":
			if !appleCPUModelPattern.MatchString(value) {
				value = s.token("cpu_model", value)
			}
		case "device_model":
			if !appleDevicePattern.MatchString(value) {
				value = s.token("device_model", value)
			}
		default:
			continue
		}
		clean := key + ":" + value
		if !slices.Contains(result, clean) {
			result = append(result, clean)
		}
	}
	return result
}

func (s *Sanitizer) processName(value string) string {
	switch value {
	case "Google Chrome", "Google Chrome Helper", "chrome.exe", "SentinelAgent.exe", "sentinel-agent":
		return value
	}
	return s.token("process", value)
}

// SeriesSource is an owned, finite copy of a serializer source.
type SeriesSource struct {
	Series []*metrics.Serie
	index  int
}

func NewSeriesSource(series []*metrics.Serie) *SeriesSource {
	return &SeriesSource{Series: series, index: -1}
}
func (s *SeriesSource) MoveNext() bool          { s.index++; return s.index < len(s.Series) }
func (s *SeriesSource) Current() *metrics.Serie { return s.Series[s.index] }
func (s *SeriesSource) Count() uint64           { return uint64(len(s.Series)) }

var metricNames = map[string]bool{
	"system.cpu.user": true, "system.cpu.system": true, "system.cpu.idle": true, "system.cpu.iowait": true, "system.cpu.num_cores": true,
	"system.cpu.interrupt": true, "system.cpu.context_switches": true, "system.cpu.stolen": true, "system.cpu.guest": true,
	"system.cpu.user.total": true, "system.cpu.nice.total": true, "system.cpu.system.total": true, "system.cpu.idle.total": true,
	"system.cpu.iowait.total": true, "system.cpu.irq.total": true, "system.cpu.softirq.total": true, "system.cpu.steal.total": true, "system.cpu.guest.total": true, "system.cpu.guestnice.total": true,
	"system.mem.total": true, "system.mem.used": true, "system.mem.free": true, "system.mem.usable": true, "system.mem.pct_usable": true,
	"system.mem.cached": true, "system.mem.committed": true, "system.mem.paged": true, "system.mem.nonpaged": true,
	"system.mem.pagefile.total": true, "system.mem.pagefile.used": true, "system.mem.pagefile.free": true, "system.mem.pagefile.pct_free": true,
	"system.paging.total": true, "system.paging.used": true, "system.paging.free": true, "system.paging.pct_free": true,
	"system.swap.total": true, "system.swap.used": true, "system.swap.free": true, "system.swap.pct_free": true, "system.swap.swap_in": true, "system.swap.swap_out": true,
	"system.disk.total": true, "system.disk.used": true, "system.disk.free": true, "system.disk.utilized": true, "system.uptime": true,
	"system.wlan.rssi": true, "system.wlan.noise": true, "system.wlan.txrate": true, "system.wlan.rxrate": true,
	"system.wlan.status": true, "system.wlan.roaming_events": true, "system.wlan.channel_swap_events": true, "system.wlan.check.errors": true,
	"system.battery.maximum_capacity_pct": true, "system.battery.current_charge_pct": true, "system.battery.cycle_count": true, "system.battery.charge_rate": true,
	"system.net.packets_in.drop": true, "system.net.packets_out.drop": true, "system.net.packets_in.error": true, "system.net.packets_out.error": true, "system.net.tcp.retrans_segs": true,
}

// Series consumes the full source and returns only allowlisted metric fields.
func (s *Sanitizer) Series(source metrics.SerieSource) (metrics.SerieSource, error) {
	var result []*metrics.Serie
	for source.MoveNext() {
		v := source.Current()
		if v == nil || !metricNames[v.Name] {
			continue
		}
		var tags []string
		v.Tags.ForEach(func(tag string) {
			key, value, ok := strings.Cut(tag, ":")
			if !ok {
				return
			}
			switch key {
			case "bssid", "mac_address", "client_mac":
				value = s.mac(value)
			case "ssid":
				value = s.token("ssid", value)
			case "interface", "device":
				value = s.token("interface", value)
			case "pagefile_path":
				value = "/capture/paging/" + s.token("path", value)
			case "core":
				if !cpuCorePattern.MatchString(value) {
					return
				}
			case "infra_mode":
				if value != "end_user_device" {
					return
				}
			case "channel":
				if _, err := strconv.ParseUint(value, 10, 16); err != nil {
					return
				}
			case "phy_mode":
				if !slices.Contains([]string{"802.11a", "802.11b", "802.11g", "802.11n", "802.11ac", "802.11ax", "802.11be"}, value) {
					return
				}
			case "status":
				if !slices.Contains([]string{"ok", "warning", "critical"}, value) {
					return
				}
			case "reason":
				if !slices.Contains([]string{"ipc_failure", "interface_inactive"}, value) {
					return
				}
			case "error_type":
				if value != "ipc_failure" {
					return
				}
			default:
				return
			}
			tags = append(tags, key+":"+value)
		})
		result = append(result, &metrics.Serie{Name: v.Name, Host: "capture-host", Points: slices.Clone(v.Points), Tags: tagset.CompositeTagsFromSlice(tags), Device: s.token("device", v.Device), MType: v.MType, Interval: v.Interval, Source: v.Source})
	}
	return NewSeriesSource(result), nil
}

// HostMetadata is a portable allowlist of the Agent host-metadata contract.
// Unknown top-level fields (including apiKey, cloud and container metadata) are
// discarded when the native sample is decoded into this type.
type HostMetadata struct {
	AgentVersion string                     `json:"agentVersion"`
	UUID         string                     `json:"uuid"`
	Hostname     string                     `json:"internalHostname"`
	OS           string                     `json:"os"`
	AgentFlavor  string                     `json:"agent-flavor"`
	SystemStats  map[string]json.RawMessage `json:"systemStats,omitempty"`
	Meta         map[string]json.RawMessage `json:"meta"`
	Network      map[string]string          `json:"network,omitempty"`
	HostTags     map[string][]string        `json:"host-tags"`
	Gohai        string                     `json:"gohai,omitempty"`
}

func (h *HostMetadata) MarshalJSON() ([]byte, error) {
	type plain HostMetadata
	return json.Marshal((*plain)(h))
}

func (s *Sanitizer) HostMetadata(native marshaler.JSONMarshaler) (marshaler.JSONMarshaler, error) {
	data, err := native.MarshalJSON()
	if err != nil {
		return nil, errors.New("cannot read native host metadata")
	}
	var input HostMetadata
	if json.Unmarshal(data, &input) != nil {
		return nil, errors.New("invalid native host metadata")
	}
	result := &HostMetadata{AgentVersion: version.AgentVersion, UUID: s.uuid(input.UUID), Hostname: "capture-host", HostTags: map[string][]string{"system": s.hostTags(input.HostTags["system"])}, Meta: map[string]json.RawMessage{"socket-hostname": json.RawMessage(`"capture-host"`), "hostname": json.RawMessage(`"capture-host"`)}}
	if slices.Contains([]string{"darwin", "windows", "win32"}, input.OS) {
		result.OS = input.OS
	}
	if input.AgentFlavor == "agent" {
		result.AgentFlavor = input.AgentFlavor
	}
	result.SystemStats = map[string]json.RawMessage{}
	for key, value := range input.SystemStats {
		switch key {
		case "cpuCores":
			var n int32
			if json.Unmarshal(value, &n) == nil && n > 0 {
				result.SystemStats[key] = slices.Clone(value)
			}
		case "machine":
			var v string
			if json.Unmarshal(value, &v) == nil && safeArchitecture(v) != "" {
				result.SystemStats[key] = slices.Clone(value)
			}
		case "platform":
			var v string
			if json.Unmarshal(value, &v) == nil && slices.Contains([]string{"darwin", "windows"}, v) {
				result.SystemStats[key] = slices.Clone(value)
			}
		case "macV":
			var fields []json.RawMessage
			if json.Unmarshal(value, &fields) == nil && len(fields) == 3 {
				var release, architecture string
				if json.Unmarshal(fields[0], &release) == nil && json.Unmarshal(fields[2], &architecture) == nil {
					// The legacy middle tuple is empty in native Agent metadata;
					// recreate it rather than carrying opaque values through.
					result.SystemStats[key], _ = json.Marshal([]any{safeOSVersion(release), [3]string{}, safeArchitecture(architecture)})
				}
			}
		case "winV":
			var fields []string
			if json.Unmarshal(value, &fields) == nil && len(fields) == 2 {
				result.SystemStats[key], _ = json.Marshal([]string{safeOSPlatform(fields[0]), safeVersion(fields[1])})
			}
		}
	}
	if input.Network != nil {
		result.Network = map[string]string{"network-id": s.token("network", input.Network["network-id"])}
	}
	// Gohai is nested JSON encoded as a string by the Agent. Decode it in memory,
	// then apply a separate allowlist so opaque raw JSON can never pass through.
	if input.Gohai != "" {
		var raw map[string]any
		if json.Unmarshal([]byte(input.Gohai), &raw) != nil {
			return nil, errors.New("invalid native gohai metadata")
		}
		clean := map[string]any{}
		for _, section := range []string{"cpu", "memory", "platform", "network"} {
			if fields, ok := raw[section].(map[string]any); ok {
				values := map[string]any{}
				for key, v := range fields {
					switch key {
					case "cpu_cores", "cpu_logical_processors", "cpu_pkgs", "cpu_numa_nodes", "cache_size", "cache_size_l1", "cache_size_l2", "cache_size_l3", "mhz", "total", "swap_total":
						switch n := v.(type) {
						case float64:
							if n >= 0 && !math.IsNaN(n) && !math.IsInf(n, 0) {
								values[key] = n
							}
						case string:
							// Gohai renders memory and cache sizes with fixed units.
							text := strings.TrimSuffix(strings.TrimSuffix(n, " KB"), "kB")
							if number, err := strconv.ParseFloat(text, 64); err == nil && number >= 0 && !math.IsNaN(number) && !math.IsInf(number, 0) {
								values[key] = n
							}
						}
					case "hostname":
						values[key] = "capture-host"
					case "serial_number":
						if str, ok := v.(string); ok {
							values[key] = s.token(key, str)
						}
					case "hardware_uuid":
						if str, ok := v.(string); ok {
							values[key] = s.uuid(str)
						}
					case "ipaddress", "ipaddressv6":
						if str, ok := v.(string); ok {
							values[key] = s.ip(str)
						}
					case "macaddress":
						if str, ok := v.(string); ok {
							values[key] = s.mac(str)
						}
					case "machine", "hardware_platform", "GOOARCH", "processor":
						if str, ok := v.(string); ok && safeArchitecture(str) != "" {
							values[key] = str
						}
					case "kernel_name", "os", "GOOS":
						if str, ok := v.(string); ok && safeOSPlatform(str) != "" {
							values[key] = str
						}
					case "kernel_release":
						if str, ok := v.(string); ok && safeVersion(str) != "" {
							values[key] = str
						}
					case "family":
						if section == "platform" {
							if str, ok := v.(string); ok && safeOSFamily(str) != "" {
								values[key] = str
							}
						}
					}
				}
				clean[section] = values
			}
		}
		encoded, err := json.Marshal(clean)
		if err != nil {
			return nil, errors.New("cannot encode sanitized gohai metadata")
		}
		result.Gohai = string(encoded)
	}
	return result, nil
}

// Process sanitizes a full native message without retaining unknown fields,
// containers, command arguments, environment tags or cloud relationships.
func (s *Sanitizer) Process(in *model.CollectorProc) *model.CollectorProc {
	out := &model.CollectorProc{HostName: "capture-host", NetworkId: s.token("network", in.NetworkId), GroupId: in.GroupId, GroupSize: in.GroupSize}
	if in.Info != nil {
		out.Info = &model.SystemInfo{Uuid: s.uuid(in.Info.Uuid), TotalMemory: in.Info.TotalMemory}
		if in.Info.Os != nil {
			out.Info.Os = &model.OSInfo{Version: safeOSVersion(in.Info.Os.Version), KernelVersion: safeVersion(in.Info.Os.KernelVersion), Platform: safeOSPlatform(in.Info.Os.Platform), Family: safeOSFamily(in.Info.Os.Family)}
			for _, v := range []string{"windows", "darwin"} {
				if strings.EqualFold(in.Info.Os.Name, v) {
					out.Info.Os.Name = v
				}
			}
		}
		for _, cpu := range in.Info.Cpus {
			if cpu != nil {
				out.Info.Cpus = append(out.Info.Cpus, &model.CPUInfo{Number: cpu.Number, Cores: cpu.Cores, Mhz: cpu.Mhz})
			}
		}
	}
	for _, p := range in.Processes {
		if p == nil {
			continue
		}
		copy := &model.Process{Pid: s.pid(p.Pid), NsPid: s.pid(p.NsPid), CreateTime: p.CreateTime, OpenFdCount: p.OpenFdCount, State: p.State, VoluntaryCtxSwitches: p.VoluntaryCtxSwitches, InvoluntaryCtxSwitches: p.InvoluntaryCtxSwitches}
		if p.Command != nil {
			name := s.processName(p.Command.Comm)
			if name == "" {
				name = s.processName(p.Command.Exe)
			}
			exe, cwd, root := "/capture/bin/"+name, "/capture", "/"
			if out.Info != nil && out.Info.Os != nil && out.Info.Os.Name == "windows" {
				exe, cwd, root = `C:\capture\bin\`+name, `C:\capture`, `C:\`
			}
			copy.Command = &model.Command{Comm: name, Exe: exe, Args: []string{exe}, Cwd: cwd, Root: root, Ppid: s.pid(p.Command.Ppid), Pgroup: s.pid(p.Command.Pgroup), OnDisk: p.Command.OnDisk}
		}
		if p.User != nil {
			copy.User = &model.ProcessUser{Name: s.token("user", p.User.Name)}
		}
		if p.Cpu != nil {
			copy.Cpu = &model.CPUStat{TotalPct: p.Cpu.TotalPct, UserPct: p.Cpu.UserPct, SystemPct: p.Cpu.SystemPct, NumThreads: p.Cpu.NumThreads, UserTime: p.Cpu.UserTime, SystemTime: p.Cpu.SystemTime}
		}
		if p.Memory != nil {
			copy.Memory = &model.MemoryStat{Rss: p.Memory.Rss, Vms: p.Memory.Vms, Swap: p.Memory.Swap, Shared: p.Memory.Shared, Text: p.Memory.Text, Data: p.Memory.Data, Dirty: p.Memory.Dirty}
		}
		out.Processes = append(out.Processes, copy)
	}
	return out
}

// Connections retains TCP evidence while dropping DNS, HTTP, database,
// container, cloud, route and unknown auxiliary metadata.
func (s *Sanitizer) Connections(in *model.CollectorConnections) *model.CollectorConnections {
	out := &model.CollectorConnections{HostName: "capture-host", NetworkId: s.token("network", in.NetworkId), GroupId: in.GroupId, GroupSize: in.GroupSize}
	addr := func(v *model.Addr) *model.Addr {
		if v == nil {
			return nil
		}
		return &model.Addr{Ip: s.ip(v.Ip), Port: v.Port}
	}
	for _, c := range in.Connections {
		if c == nil {
			continue
		}
		out.Connections = append(out.Connections, &model.Connection{Pid: s.pid(c.Pid), Laddr: addr(c.Laddr), Raddr: addr(c.Raddr), Family: c.Family, Type: c.Type, Direction: c.Direction, LastBytesSent: c.LastBytesSent, LastBytesReceived: c.LastBytesReceived, LastPacketsSent: c.LastPacketsSent, LastPacketsReceived: c.LastPacketsReceived, LastRetransmits: c.LastRetransmits, Rtt: c.Rtt, RttVar: c.RttVar, IntraHost: c.IntraHost, LastTcpEstablished: c.LastTcpEstablished, LastTcpClosed: c.LastTcpClosed, TcpFailuresByErrCode: maps.Clone(c.TcpFailuresByErrCode), SystemProbeConn: c.SystemProbeConn})
	}
	return out
}
