// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package networkv2

import (
	"fmt"
	"maps"
	"regexp"
	"slices"

	"github.com/shirou/gopsutil/v4/net"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	yaml "go.yaml.in/yaml/v2"
)

var (
	protocolsMetricsMapping = map[string]map[string]string{
		"tcp": {
			"RetransSegs":     "system.net.tcp.retrans_segs",
			"InSegs":          "system.net.tcp.in_segs",
			"OutSegs":         "system.net.tcp.out_segs",
			"ListenOverflows": "system.net.tcp.listen_overflows",
			"ListenDrops":     "system.net.tcp.listen_drops",
			"TCPBacklogDrop":  "system.net.tcp.backlog_drops",
			"TCPRetransFail":  "system.net.tcp.failed_retransmits",
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
		// AIX netstat output omits the state column for UDP rows; gopsutil
		// leaves Status as "" in that case. Map it the same as "NONE".
		"": "connections",
	}
)

type networkInstanceConfig struct {
	CollectRateMetrics       bool     `yaml:"collect_rate_metrics"`
	CollectCountMetrics      bool     `yaml:"collect_count_metrics"`
	CollectConnectionState   bool     `yaml:"collect_connection_state"`
	ExcludedInterfaces       []string `yaml:"excluded_interfaces"`
	ExcludedInterfaceRe      string   `yaml:"excluded_interface_re"`
	ExcludedInterfacePattern *regexp.Regexp
}

type networkStats interface {
	IOCounters(pernic bool) ([]net.IOCountersStat, error)
	ProtoCounters(protocols []string) ([]net.ProtoCountersStat, error)
	Connections(kind string) ([]net.ConnectionStat, error)
	NetstatTCPExtCounters() (map[string]int64, error)
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

func (n defaultNetworkStats) NetstatTCPExtCounters() (map[string]int64, error) {
	// TCPExt counters come from /proc/net/netstat which does not exist on AIX.
	return nil, nil
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
				maps.Copy(protocolStats.Stats, counters)
			}
		}
		c.submitProtocolMetrics(sender, protocolStats)
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
	if slices.Contains(c.config.instance.ExcludedInterfaces, deviceName) {
		return true
	}
	if c.config.instance.ExcludedInterfacePattern != nil {
		return c.config.instance.ExcludedInterfacePattern.MatchString(deviceName)
	}
	return false
}

func submitInterfaceMetrics(sender sender.Sender, interfaceIO net.IOCountersStat) {
	tags := []string{"device:" + interfaceIO.Name, "device_name:" + interfaceIO.Name}
	sender.Rate("system.net.bytes_rcvd", float64(interfaceIO.BytesRecv), "", tags)
	sender.Rate("system.net.bytes_sent", float64(interfaceIO.BytesSent), "", tags)
	sender.Rate("system.net.packets_in.count", float64(interfaceIO.PacketsRecv), "", tags)
	sender.Rate("system.net.packets_in.drop", float64(interfaceIO.Dropin), "", tags)
	sender.Rate("system.net.packets_in.error", float64(interfaceIO.Errin), "", tags)
	sender.Rate("system.net.packets_out.count", float64(interfaceIO.PacketsSent), "", tags)
	sender.Rate("system.net.packets_out.drop", float64(interfaceIO.Dropout), "", tags)
	sender.Rate("system.net.packets_out.error", float64(interfaceIO.Errout), "", tags)
}

func (c *NetworkCheck) submitProtocolMetrics(sender sender.Sender, protocolStats net.ProtoCountersStat) {
	if protocolMapping, ok := protocolsMetricsMapping[protocolStats.Protocol]; ok {
		for rawMetricName, metricName := range protocolMapping {
			if metricValue, ok := protocolStats.Stats[rawMetricName]; ok {
				if c.config.instance.CollectRateMetrics {
					sender.Rate(metricName, float64(metricValue), "", nil)
				}
				if c.config.instance.CollectCountMetrics {
					sender.MonotonicCount(metricName+".count", float64(metricValue), "", nil)
				}
			}
		}
	}
}

// submitConnectionsMetrics tallies connectionsStats by status (via stateMetricSuffixMapping)
// and emits one gauge per resulting suffix.
func submitConnectionsMetrics(sender sender.Sender, protocolName string, stateMetricSuffixMapping map[string]string, connectionsStats []net.ConnectionStat) {
	metricCount := map[string]float64{}
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

// Configure configures the network checks
func (c *NetworkCheck) Configure(senderManager sender.SenderManager, _ uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string, provider string) error {
	err := c.CommonConfigure(senderManager, rawInitConfig, rawInstance, source, provider)
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
