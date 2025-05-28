// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

//nolint:revive // TODO(PLINT) Fix revive linter
package networkv2

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/shirou/gopsutil/v4/net"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "network"
)

// NetworkCheck represent a network check
type NetworkCheck struct {
	core.CheckBase
	net    networkStats
	config networkConfig
}

type networkInstanceConfig struct {
	CollectConnectionState   bool     `yaml:"collect_connection_state"`
	CollectConnectionQueues  bool     `yaml:"collect_connection_queues"`
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
}

type defaultNetworkStats struct{}

func (n defaultNetworkStats) IOCounters(pernic bool) ([]net.IOCountersStat, error) {
	return net.IOCounters(pernic)
}

func (n defaultNetworkStats) ProtoCounters(protocols []string) ([]net.ProtoCountersStat, error) {
	return net.ProtoCounters(protocols)
}

func (n defaultNetworkStats) Connections(kind string) ([]net.ConnectionStat, error) {
	return net.Connections(kind)
}

// Run executes the check
func (c *NetworkCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	// _cx_state_psutil
	if c.config.instance.CollectConnectionState {
		for _, protocol := range []string{"udp4", "udp6", "tcp4", "tcp6"} {
			connectionsStats, err := c.net.Connections(protocol)
			if err != nil {
				return err
			}
			submitConnectionsMetrics(sender, protocol, connectionsStats)
		}
	}

	// _cx_counters_psutil
	ioByInterface, err := c.net.IOCounters(true)
	if err != nil {
		return err
	}

	for _, interfaceIO := range ioByInterface {
		if !c.isDeviceExcluded(interfaceIO.Name) {
			submitInterfaceMetrics(sender, interfaceIO)
		}
	}

	// _tcp_stats
	submitTcpStats(sender)

	sender.Commit()
	return nil
}

func submitConnectionsMetrics(sender sender.Sender, protocolName string, connectionsStats []net.ConnectionStat) {
	metricCount := map[string]float64{}
	var stateMetricSuffixMapping map[string]string
	switch protocolName {
	case "udp4", "udp6":
		stateMetricSuffixMapping = udpStateMetricsSuffixMapping
	case "tcp4", "tcp6":
		stateMetricSuffixMapping = tcpStateMetricsSuffixMapping["psutil"]
	}
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

const (
	AF_INET  = 2
	AF_INET6 = 23
)

// TCPSTATS DWORD mappings
// https://learn.microsoft.com/en-us/windows/win32/api/tcpmib/ns-tcpmib-mib_tcpstats_lh
type mibTcpStats struct {
	dwRtoAlgorithm uint32
	dwRtoMin       uint32
	dwRtoMax       uint32
	dwMaxConn      uint32
	dwActiveOpens  uint32
	dwPassiveOpens uint32
	dwAttemptFails uint32
	dwEstabResets  uint32
	dwCurrEstab    uint32
	dwInSegs       uint32
	dwOutSegs      uint32
	dwRetransSegs  uint32
	dwInErrs       uint32
	dwOutRsts      uint32
	dwNumConns     uint32
}

var (
	iphlpapi               = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetTcpStatisticsEx = iphlpapi.NewProc("GetTcpStatisticsEx")
)

// gopsutil does not call GetTcpStatisticsEx so we need to do so manually for these stats
func getTcpStats(inet uint) (mibTcpStats, error) {
	tcpStats := &mibTcpStats{}
	r0, _, _ := syscall.Syscall(procGetTcpStatisticsEx.Addr(), 2, uintptr(unsafe.Pointer(tcpStats)), uintptr(inet), 0)
	if r0 != 0 {
		err := syscall.Errno(r0)
		return nil, err
	}
	return tcpStats, nil
}

// Collect metrics from Microsoft's TCPSTATS
func submitTcpStats(sender sender.Sender) error {
	tcpStatsMapping := map[string]string{
		"dwActiveOpens":  ".active_opens",
		"dwPassiveOpens": ".passive_opens",
		"dwAttemptFails": ".attempt_fails",
		"dwEstabResets":  ".established_resets",
		"dwCurrEstab":    ".current_established",
		"dwInSegs":       ".in_segs",
		"dwOutSegs":      ".out_segs",
		"dwRetransSegs":  ".retrans_segs",
		"dwInErrs":       ".in_errors",
		"dwOutRsts":      ".out_resets",
		"dwNumConns":     ".connections",
	}

	tcp4Stats, err := getTcpStats(AF_INET)
	if err != nil {
		return err
	}

	tcp6Stats, err := getTcpStats(AF_INET6)
	if err != nil {
		return err
	}

	// Create tcp metrics that are a sum of tcp4 and tcp6 metrics
	tcpAllStats := &mibTcpStats{}
	if tcp4Stats != nil && tcp6Stats != nil {
		tcpAllStats = &mibTcpStats{
			dwRtoAlgorithm: tcp4Stats.dwRtoAlgorithm + tcp6Stats.dwRtoAlgorithm,
			dwRtoMin:       tcp4Stats.dwRtoMin + tcp6Stats.dwRtoMin,
			dwRtoMax:       tcp4Stats.dwRtoMax + tcp6Stats.dwRtoMax,
			dwMaxConn:      tcp4Stats.dwMaxConn + tcp6Stats.dwMaxConn,
			dwActiveOpens:  tcp4Stats.dwActiveOpens + tcp6Stats.dwActiveOpens,
			dwPassiveOpens: tcp4Stats.dwPassiveOpens + tcp6Stats.dwPassiveOpens,
			dwAttemptFails: tcp4Stats.dwAttemptFails + tcp6Stats.dwAttemptFails,
			dwEstabResets:  tcp4Stats.dwEstabResets + tcp6Stats.dwEstabResets,
			dwCurrEstab:    tcp4Stats.dwCurrEstab + tcp6Stats.dwCurrEstab,
			dwInSegs:       tcp4Stats.dwInSegs + tcp6Stats.dwInSegs,
			dwOutSegs:      tcp4Stats.dwOutSegs + tcp6Stats.dwOutSegs,
			dwRetransSegs:  tcp4Stats.dwRetransSegs + tcp6Stats.dwRetransSegs,
			dwInErrs:       tcp4Stats.dwInErrs + tcp6Stats.dwInErrs,
			dwOutRsts:      tcp4Stats.dwOutRsts + tcp6Stats.dwOutRsts,
			dwNumConns:     tcp4Stats.dwNumConns + tcp6Stats.dwNumConns,
		}
	}

	submitMetricsFromStruct(sender, "system.net.tcp.", tcpAllStats, tcpStatsMapping)
	submitMetricsFromStruct(sender, "system.net.tcp4.", tcp4Stats, tcpStatsMapping)
	submitMetricsFromStruct(sender, "system.net.tcp6.", tcp6Stats, tcpStatsMapping)
}

func submitMetricsFromStruct(sender sender.Sender, metricPrefix string, tcpStats *mibTcpStats, tcpStatsMapping map[string]string) {
	sType := reflect.TypeOf(reflect.ValueOf(tcpStats))
	for i := 0; i < sType.NumFields(); i++ {
		field := sType.Field(i)
		metricName := metricPrefix + tcpMapping[field.Name()]
		metricValue := field.Uint()
		if strings.HasSuffix(metricName, ".connections") || strings.HasSuffix(".current_established") {
			sender.Gauge(metricName, float64(metricValue), "", nil)
		} else {
			sender.Rate(metricName, float64(metricValue), "", nil)
			sender.MonotonicCount(fmt.Sprintf("%s.count", metricName), float64(metricValue), "", nil)
		}
	}
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
func Factory(cfg config.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(cfg)
	})
}

func newCheck(_ config.Component) check.Check {
	return &NetworkCheck{
		net:       defaultNetworkStats{},
		CheckBase: core.NewCheckBase(CheckName),
	}
}
