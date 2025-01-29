// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

//nolint:revive // TODO(PLINT) Fix revive linter
package network

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/spf13/afero"
	"golang.org/x/sys/unix"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "network"

	SIOCETHTOOL        = 0x8946
	ETHTOOL_GDRVINFO   = 0x00000003
	ETHTOOL_GSTATS     = 0x0000001D
	ETHTOOL_GSTRINGS   = 0x0000001B
	ETHTOOL_GSSET_INFO = 0x00000037
	ETH_SS_STATS       = 0x1
	ETH_GSTRING_LEN    = 32
)

var (
	protocolsMetricsMapping = map[string]map[string]string{
		"tcp": {
			"RetransSegs":  "system.net.tcp.retrans_segs",
			"InSegs":       "system.net.tcp.in_segs",
			"OutSegs":      "system.net.tcp.out_segs",
			"ActiveOpens":  "system.net.tcp.active_opens",
			"PassiveOpens": "system.net.tcp.passive_opens",
			"AttemptFails": "system.net.tcp.attempt_fails",
			"EstabResets":  "system.net.tcp.established_resets",
			"InErrs":       "system.net.tcp.in_errors",
			"OutRsts":      "system.net.tcp.out_resets",
			"InCsumErrors": "system.net.tcp.in_csum_errors",
			// below here are TcpExt metrics:
			// https://github.com/DataDog/integrations-core/blob/master/network/datadog_checks/network/check_linux.py#L220
			"ListenOverflows":      "system.net.tcp.listen_overflows",
			"ListenDrops":          "system.net.tcp.listen_drops",
			"TCPBacklogDrop":       "system.net.tcp.backlog_drops",
			"TCPRetransFail":       "system.net.tcp.failed_retransmits",
			"IPReversePathFilter":  "system.net.ip.reverse_path_filter",
			"PruneCalled":          "system.net.tcp.prune_called",
			"RcvPruned":            "system.net.tcp.prune_rcv_drops",
			"OfoPruned":            "system.net.tcp.prune_ofo_called",
			"PAWSActive":           "system.net.tcp.paws_connection_drops",
			"PAWSEstab":            "system.net.tcp.paws_established_drops",
			"SyncookiesSent":       "system.net.tcp.syn_cookies_sent",
			"SyncookiesRecv":       "system.net.tcp.syn_cookies_recv",
			"SyncookiesFailed":     "system.net.tcp.syn_cookies_failed",
			"TCPAbortOnTimeout":    "system.net.tcp.abort_on_timeout",
			"TCPSynRetrans":        "system.net.tcp.syn_retrans",
			"TCPFromZeroWindowAdv": "system.net.tcp.from_zero_window",
			"TCPToZeroWindowAdv":   "system.net.tcp.to_zero_window",
			"TWRecycled":           "system.net.tcp.tw_reused",
		},
		"udp": {
			"InDatagrams":  "system.net.udp.in_datagrams",
			"NoPorts":      "system.net.udp.no_ports",
			"InErrors":     "system.net.udp.in_errors",
			"OutDatagrams": "system.net.udp.out_datagrams",
			"RcvbufErrors": "system.net.udp.rcv_buf_errors",
			"SndbufErrors": "system.net.udp.snd_buf_errors",
			"InCsumErrors": "system.net.udp.in_csum_errors",
		},
	}
	tcpStateMetricsSuffixMapping = map[string]string{
		"ESTABLISHED": "established",
		"SYN_SENT":    "opening",
		"SYN_RECV":    "opening",
		"FIN_WAIT1":   "closing",
		"FIN_WAIT2":   "closing",
		"TIME_WAIT":   "time_wait",
		"CLOSE":       "closing",
		"CLOSE_WAIT":  "closing",
		"LAST_ACK":    "closing",
		"LISTEN":      "listening",
		"CLOSING":     "closing",
	}

	udpStateMetricsSuffixMapping = map[string]string{
		"NONE": "connections",
	}

	filesystem = afero.NewOsFs()

	getSyscall = unix.Syscall
	getSocket  = syscall.Socket
	getClose   = syscall.Close
)

// NetworkCheck represent a network check
type NetworkCheck struct {
	core.CheckBase
	net    networkStats
	config networkConfig
}

type networkInstanceConfig struct {
	CollectConnectionState   bool     `yaml:"collect_connection_state"`
	ExcludedInterfaces       []string `yaml:"excluded_interfaces"`
	ExcludedInterfaceRe      string   `yaml:"excluded_interface_re"`
	ExcludedInterfacePattern *regexp.Regexp
}

type networkInitConfig struct{}

type networkConfig struct {
	instance networkInstanceConfig
	initConf networkInitConfig
}

type networkStats interface {
	IOCounters(pernic bool) ([]net.IOCountersStat, error)
	ProtoCounters(protocols []string) ([]net.ProtoCountersStat, error)
	Connections(kind string) ([]net.ConnectionStat, error)
	NetstatTCPExtCounters() (map[string]int64, error)
}

type defaultNetworkStats struct {
	procPath string
}

func (n defaultNetworkStats) IOCounters(pernic bool) ([]net.IOCountersStat, error) {
	return net.IOCounters(pernic)
}

func (n defaultNetworkStats) ProtoCounters(protocols []string) ([]net.ProtoCountersStat, error) {
	return net.ProtoCounters(protocols)
}

func (n defaultNetworkStats) Connections(kind string) ([]net.ConnectionStat, error) {
	return net.Connections(kind)
}

func (n defaultNetworkStats) NetstatTCPExtCounters() (map[string]int64, error) {
	return netstatTCPExtCounters(n.procPath)
}

type ifreq struct {
	Name [16]byte
	Data uintptr
}

type ethtool_drvinfo struct {
	Driver  string
	Version string
}

type ethtool_stats struct {
	Count uint32
	Data  []uint64
}

type ethtool_gstrings struct {
	Cmd       uint32
	StringSet uint32
	Len       uint32
	Data      []byte
}

// Run executes the check
func (c *NetworkCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	ioByInterface, err := c.net.IOCounters(true)
	if err != nil {
		return err
	}
	for _, interfaceIO := range ioByInterface {
		if !c.isDeviceExcluded(interfaceIO.Name) {
			submitInterfaceMetrics(sender, interfaceIO)
			err = fetchEthtoolStats(sender, interfaceIO)
			if err != nil {
				return err
			}
		}
	}

	protocols := []string{"tcp", "udp"}
	protocolsStats, err := c.net.ProtoCounters(protocols)
	if err != nil {
		return err
	}
	for _, protocolStats := range protocolsStats {
		// For TCP we want some extra counters coming from /proc/net/netstat if available
		if protocolStats.Protocol == "tcp" {
			counters, err := c.net.NetstatTCPExtCounters()
			if err != nil {
				log.Debug(err)
			} else {
				for counter, value := range counters {
					protocolStats.Stats[counter] = value
				}
			}
		}
		submitProtocolMetrics(sender, protocolStats)
	}

	if c.config.instance.CollectConnectionState {
		connectionsStats, err := c.net.Connections("udp4")
		if err != nil {
			return err
		}
		submitConnectionsMetrics(sender, "udp4", udpStateMetricsSuffixMapping, connectionsStats)

		connectionsStats, err = c.net.Connections("udp6")
		if err != nil {
			return err
		}
		submitConnectionsMetrics(sender, "udp6", udpStateMetricsSuffixMapping, connectionsStats)

		connectionsStats, err = c.net.Connections("tcp4")
		if err != nil {
			return err
		}
		submitConnectionsMetrics(sender, "tcp4", tcpStateMetricsSuffixMapping, connectionsStats)

		connectionsStats, err = c.net.Connections("tcp6")
		if err != nil {
			return err
		}
		submitConnectionsMetrics(sender, "tcp6", tcpStateMetricsSuffixMapping, connectionsStats)
	}

	sender.Commit()
	return nil
}

func (c *NetworkCheck) isDeviceExcluded(deviceName string) bool {
	for _, excludedDevice := range c.config.instance.ExcludedInterfaces {
		if deviceName == excludedDevice {
			return true
		}
	}
	if c.config.instance.ExcludedInterfacePattern != nil {
		return c.config.instance.ExcludedInterfacePattern.MatchString(deviceName)
	}
	return false
}

func submitInterfaceMetrics(sender sender.Sender, interfaceIO net.IOCountersStat) {
	tags := []string{fmt.Sprintf("device:%s", interfaceIO.Name), fmt.Sprintf("device_name:%s", interfaceIO.Name)}
	sender.Rate("system.net.bytes_rcvd", float64(interfaceIO.BytesRecv), "", tags)
	sender.Rate("system.net.bytes_sent", float64(interfaceIO.BytesSent), "", tags)
	sender.Rate("system.net.packets_in.count", float64(interfaceIO.PacketsRecv), "", tags)
	sender.Rate("system.net.packets_in.drop", float64(interfaceIO.Dropin), "", tags)
	sender.Rate("system.net.packets_in.error", float64(interfaceIO.Errin), "", tags)
	sender.Rate("system.net.packets_out.count", float64(interfaceIO.PacketsSent), "", tags)
	sender.Rate("system.net.packets_out.drop", float64(interfaceIO.Dropout), "", tags)
	sender.Rate("system.net.packets_out.error", float64(interfaceIO.Errout), "", tags)
}

func fetchEthtoolStats(sender sender.Sender, interfaceIO net.IOCountersStat) error {
	ethtoolSocket, err := getSocket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return err
	}
	defer getClose(ethtoolSocket)
	if err != nil {
		return err
	}

	// Preparing the interface name and copy it into the request
	ifaceBytes := []byte(interfaceIO.Name)
	if len(ifaceBytes) > 15 {
		ifaceBytes = ifaceBytes[:15]
	}
	var req ifreq
	copy(req.Name[:], ifaceBytes)

	// Fetch driver information (ETHTOOL_GDRVINFO)
	var drvInfo ethtool_drvinfo
	req.Data = uintptr(unsafe.Pointer(&drvInfo))
	_, _, errno := getSyscall(unix.SYS_IOCTL, uintptr(ethtoolSocket), uintptr(ETHTOOL_GDRVINFO), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		return errors.New("failed to get driver info for interface " + interfaceIO.Name + ": " + fmt.Sprintf("%d", errno))
	}

	driverName := string(bytes.Trim([]byte(drvInfo.Driver[:]), "\x00"))
	driverVersion := string(bytes.Trim([]byte(drvInfo.Version[:]), "\x00"))

	// Fetch stats names (ETHTOOL_GSTRINGS)
	var stringSet ethtool_gstrings
	stringSet.Cmd = ETHTOOL_GSTRINGS
	stringSet.StringSet = ETH_SS_STATS

	req.Data = uintptr(unsafe.Pointer(&stringSet))
	_, _, errno = getSyscall(unix.SYS_IOCTL, uintptr(ethtoolSocket), uintptr(ETHTOOL_GSSET_INFO), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		return errors.New("failed to get stats set info for interface " + interfaceIO.Name + ": " + fmt.Sprintf("%d", errno))
	}
	statsCount := stringSet.Len

	buf := make([]byte, statsCount*ETH_GSTRING_LEN)
	stringSet.Data = buf
	req.Data = uintptr(unsafe.Pointer(&stringSet))
	_, _, errno = getSyscall(unix.SYS_IOCTL, uintptr(ethtoolSocket), uintptr(ETHTOOL_GSTRINGS), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		return errors.New("failed to get stats names for interface " + interfaceIO.Name + ": " + fmt.Sprintf("%d", errno))
	}

	statsNames := make([]string, statsCount)
	for i := 0; i < int(statsCount); i++ {
		offset := i * ETH_GSTRING_LEN
		statsNames[i] = string(bytes.Trim(buf[offset:offset+ETH_GSTRING_LEN], "\x00"))
	}

	// Fetch stats values (ETHTOOL_GSTATS)
	data := make([]uint64, statsCount)
	stats := ethtool_stats{
		Count: uint32(statsCount),
		Data:  data,
	}

	req.Data = uintptr(unsafe.Pointer(&stats))
	_, _, errno = getSyscall(unix.SYS_IOCTL, uintptr(ethtoolSocket), uintptr(ETHTOOL_GSTATS), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		return errors.New("failed to get stats for interface " + interfaceIO.Name + ": " + fmt.Sprintf("%d", errno))
	}

	tags := []string{
		"interface:" + interfaceIO.Name,
		"driver_name:" + driverName,
		"driver_version:" + driverVersion,
	}

	for i, statName := range statsNames {
		if i >= len(data) {
			break
		}
		metricName := fmt.Sprintf("system.net.%s", statName)
		sender.Rate(metricName, float64(data[i]), "", tags)
	}

	return nil
}

func submitProtocolMetrics(sender sender.Sender, protocolStats net.ProtoCountersStat) {
	if protocolMapping, ok := protocolsMetricsMapping[protocolStats.Protocol]; ok {
		for rawMetricName, metricName := range protocolMapping {
			if metricValue, ok := protocolStats.Stats[rawMetricName]; ok {
				sender.Rate(metricName, float64(metricValue), "", nil)
				sender.MonotonicCount(fmt.Sprintf("%s.count", metricName), float64(metricValue), "", nil)
			}
		}
	}
}

func submitConnectionsMetrics(sender sender.Sender, protocolName string, stateMetricSuffixMapping map[string]string, connectionsStats []net.ConnectionStat) {
	metricCount := map[string]float64{}
	for _, suffix := range stateMetricSuffixMapping {
		metricCount[suffix] = 0
	}

	for _, connectionStats := range connectionsStats {
		metricCount[stateMetricSuffixMapping[connectionStats.Status]]++
	}

	for suffix, count := range metricCount {
		sender.Gauge(fmt.Sprintf("system.net.%s.%s", protocolName, suffix), count, "", nil)
	}
}

func netstatTCPExtCounters(procfsPath string) (map[string]int64, error) {
	fs := filesystem
	f, err := fs.Open(procfsPath + "/net/netstat")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	counters := map[string]int64{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		i := strings.IndexRune(line, ':')
		if i == -1 {
			return nil, errors.New(procfsPath + "/net/netstat is not fomatted correctly, expected ':'")
		}
		proto := strings.ToLower(line[:i])
		if proto != "tcpext" {
			continue
		}

		counterNames := strings.Split(line[i+2:], " ")

		if !scanner.Scan() {
			return nil, errors.New(procfsPath + "/net/netstat is not fomatted correctly, not data line")
		}
		line = scanner.Text()

		counterValues := strings.Split(line[i+2:], " ")
		if len(counterNames) != len(counterValues) {
			return nil, errors.New(procfsPath + "/net/netstat is not fomatted correctly, expected same number of columns")
		}

		for j := range counterNames {
			value, err := strconv.ParseInt(counterValues[j], 10, 64)
			if err != nil {
				return nil, err
			}
			counters[counterNames[j]] = value
		}
	}
	return counters, nil
}

// Configure configures the network checks
func (c *NetworkCheck) Configure(senderManager sender.SenderManager, _ uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	err := c.CommonConfigure(senderManager, rawInitConfig, rawInstance, source)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(rawInitConfig, &c.config.initConf)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(rawInstance, &c.config.instance)
	if err != nil {
		return err
	}

	if c.config.instance.ExcludedInterfaceRe != "" {
		pattern, err := regexp.Compile(c.config.instance.ExcludedInterfaceRe)
		if err != nil {
			log.Errorf("Failed to parse network check option excluded_interface_re: %s", err)
		} else {
			c.config.instance.ExcludedInterfacePattern = pattern
		}
	}

	return nil
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	procfsPath := "/proc"
	if pkgconfigsetup.Datadog().IsConfigured("procfs_path") {
		procfsPath = pkgconfigsetup.Datadog().GetString("procfs_path")
	}
	return &NetworkCheck{
		net:       defaultNetworkStats{procPath: procfsPath},
		CheckBase: core.NewCheckBase(CheckName),
	}
}
