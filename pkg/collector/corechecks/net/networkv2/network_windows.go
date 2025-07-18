// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package networkv2 provides a check for network connection and socket statistics
package networkv2

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
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

var (
	tcpStateMetricsSuffixMapping = map[string]string{
		"ESTABLISHED":  "established",
		"SYN_SENT":     "opening",
		"SYN_RECEIVED": "opening",
		"FIN_WAIT_1":   "closing",
		"FIN_WAIT_2":   "closing",
		"TIME_WAIT":    "time_wait",
		"CLOSED":       "closing",
		"CLOSE_WAIT":   "closing",
		"LAST_ACK":     "closing",
		"LISTEN":       "listening",
		"CLOSING":      "closing",
	}
	udpStateMetricsSuffixMapping = map[string]string{
		"": "connections", // gopsutil does not return a ConnectionStat.Status for UDP so it is an empty string
	}
)

// NetworkCheck represent a network check
type NetworkCheck struct {
	core.CheckBase
	net    networkStats
	config networkConfig
}

type networkInstanceConfig struct {
	CollectRateMetrics       bool     `yaml:"collect_rate_metrics"`
	CollectCountMetrics      bool     `yaml:"collect_count_metrics"`
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
	TCPStats(kind string) (*mibTCPStats, error)
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

func (n defaultNetworkStats) TCPStats(kind string) (*mibTCPStats, error) {
	return getTCPStats(kind)
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
	c.submitTCPStats(sender)

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
		stateMetricSuffixMapping = tcpStateMetricsSuffixMapping
	}

	for _, suffix := range stateMetricSuffixMapping {
		metricCount[suffix] = 0
	}

	for _, connectionStats := range connectionsStats {
		if suffix, ok := stateMetricSuffixMapping[connectionStats.Status]; ok {
			metricCount[suffix]++
		} else {
			log.Debugf("%s state mapping not found for %s", protocolName, connectionStats.Status)
		}
	}

	for suffix, count := range metricCount {
		sender.Gauge(fmt.Sprintf("system.net.%s.%s", protocolName, suffix), count, "", nil)
	}
}

const (
	AF_INET  = 2  //revive:disable-line
	AF_INET6 = 23 //revive:disable-line
)

// TCPSTATS DWORD mappings
// https://learn.microsoft.com/en-us/windows/win32/api/tcpmib/ns-tcpmib-mib_tcpstats_lh
type mibTCPStats struct {
	DwRtoAlgorithm uint32
	DwRtoMin       uint32
	DwRtoMax       uint32
	DwMaxConn      uint32
	DwActiveOpens  uint32 `metric_name:"active_opens"`
	DwPassiveOpens uint32 `metric_name:"passive_opens"`
	DwAttemptFails uint32 `metric_name:"attempt_fails"`
	DwEstabResets  uint32 `metric_name:"established_resets"`
	DwCurrEstab    uint32 `metric_name:"current_established" metric_type:"gauge"`
	DwInSegs       uint32 `metric_name:"in_segs"`
	DwOutSegs      uint32 `metric_name:"out_segs"`
	DwRetransSegs  uint32 `metric_name:"retrans_segs"`
	DwInErrs       uint32 `metric_name:"in_errors"`
	DwOutRsts      uint32 `metric_name:"out_resets"`
	DwNumConns     uint32 `metric_name:"connections" metric_type:"gauge"`
}

var (
	iphlpapi               = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetTCPStatisticsEx = iphlpapi.NewProc("GetTcpStatisticsEx")
)

// gopsutil does not call GetTcpStatisticsEx so we need to do so manually for these stats
func getTCPStats(protocolName string) (*mibTCPStats, error) {
	var inet uint
	switch protocolName {
	case "tcp4":
		inet = AF_INET
	case "tcp6":
		inet = AF_INET6
	}
	tcpStats := &mibTCPStats{}
	// the syscall will always populate the struct on success
	r0, _, _ := procGetTCPStatisticsEx.Call(uintptr(unsafe.Pointer(tcpStats)), uintptr(inet))
	if r0 != 0 {
		err := syscall.Errno(r0)
		return nil, err
	}
	return tcpStats, nil
}

// Collect metrics from Microsoft's TCPSTATS
func (c *NetworkCheck) submitTCPStats(sender sender.Sender) {
	tcp4Stats, err := c.net.TCPStats("tcp4")
	if err != nil {
		log.Errorf("OSError getting TCP4 stats from GetTcpStatisticsEx: %s", err)
	}

	tcp6Stats, err := c.net.TCPStats("tcp6")
	if err != nil {
		log.Errorf("OSError getting TCP6 stats from GetTcpStatisticsEx: %s", err)
	}

	// Create tcp metrics that are a sum of tcp4 and tcp6 metrics
	if tcp4Stats != nil && tcp6Stats != nil {
		tcpAllStats := &mibTCPStats{
			DwRtoAlgorithm: tcp4Stats.DwRtoAlgorithm + tcp6Stats.DwRtoAlgorithm,
			DwRtoMin:       tcp4Stats.DwRtoMin + tcp6Stats.DwRtoMin,
			DwRtoMax:       tcp4Stats.DwRtoMax + tcp6Stats.DwRtoMax,
			DwMaxConn:      tcp4Stats.DwMaxConn + tcp6Stats.DwMaxConn,
			DwActiveOpens:  tcp4Stats.DwActiveOpens + tcp6Stats.DwActiveOpens,
			DwPassiveOpens: tcp4Stats.DwPassiveOpens + tcp6Stats.DwPassiveOpens,
			DwAttemptFails: tcp4Stats.DwAttemptFails + tcp6Stats.DwAttemptFails,
			DwEstabResets:  tcp4Stats.DwEstabResets + tcp6Stats.DwEstabResets,
			DwCurrEstab:    tcp4Stats.DwCurrEstab + tcp6Stats.DwCurrEstab,
			DwInSegs:       tcp4Stats.DwInSegs + tcp6Stats.DwInSegs,
			DwOutSegs:      tcp4Stats.DwOutSegs + tcp6Stats.DwOutSegs,
			DwRetransSegs:  tcp4Stats.DwRetransSegs + tcp6Stats.DwRetransSegs,
			DwInErrs:       tcp4Stats.DwInErrs + tcp6Stats.DwInErrs,
			DwOutRsts:      tcp4Stats.DwOutRsts + tcp6Stats.DwOutRsts,
			DwNumConns:     tcp4Stats.DwNumConns + tcp6Stats.DwNumConns,
		}
		c.submitMetricsFromStruct(sender, "system.net.tcp.", tcpAllStats)
	}
	c.submitMetricsFromStruct(sender, "system.net.tcp4.", tcp4Stats)
	c.submitMetricsFromStruct(sender, "system.net.tcp6.", tcp6Stats)
}

func (c *NetworkCheck) submitMetricsFromStruct(sender sender.Sender, metricPrefix string, tcpStats *mibTCPStats) {
	if tcpStats == nil {
		return
	}

	s := reflect.ValueOf(tcpStats).Elem()
	sType := s.Type()
	for i := 0; i < s.NumField(); i++ {
		tag := sType.Field(i).Tag
		metricNameTag := tag.Get("metric_name")
		if metricNameTag == "" {
			continue
		}

		metricName := metricPrefix + metricNameTag
		metricValue := s.Field(i).Uint()
		if tag.Get("metric_type") == "gauge" {
			sender.Gauge(metricName, float64(metricValue), "", nil)
		} else {
			if c.config.instance.CollectRateMetrics {
				sender.Rate(metricName, float64(metricValue), "", nil)
			}
			if c.config.instance.CollectCountMetrics {
				sender.MonotonicCount(fmt.Sprintf("%s.count", metricName), float64(metricValue), "", nil)
			}
		}
	}
}

func (c *NetworkCheck) isDeviceExcluded(deviceName string) bool {
	if slices.Contains(c.config.instance.ExcludedInterfaces, deviceName) {
		return true
	}
	if c.config.instance.ExcludedInterfacePattern != nil {
		return c.config.instance.ExcludedInterfacePattern.MatchString(deviceName)
	}
	return false
}

func submitInterfaceMetrics(sender sender.Sender, interfaceIO net.IOCountersStat) {
	tags := []string{fmt.Sprintf("device:%s", interfaceIO.Name)}
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
		CheckBase: core.NewCheckBase(CheckName),
		net:       defaultNetworkStats{},
		config: networkConfig{
			instance: networkInstanceConfig{
				CollectRateMetrics: true,
			},
		},
	}
}
