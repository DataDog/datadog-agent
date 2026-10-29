// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package networkv2

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	yaml "go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/net/networkv2/nicinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const netstatTimeout = 10 * time.Second

var (
	// tcpStateMetricsSuffixMapping combines multiple TCP states into broader categories
	// (the default behavior, matching combine_connection_states: true). The keys are the
	// tcpstates names of <netinet/tcp_fsm.h>, which netstat prints.
	tcpStateMetricsSuffixMapping = map[string]string{
		"ESTABLISHED": "established",
		"SYN_SENT":    "opening",
		"SYN_RCVD":    "opening",
		"FIN_WAIT_1":  "closing",
		"FIN_WAIT_2":  "closing",
		"TIME_WAIT":   "time_wait",
		"CLOSED":      "closing",
		"CLOSE_WAIT":  "closing",
		"LAST_ACK":    "closing",
		"LISTEN":      "listening",
		"CLOSING":     "closing",
	}
	// tcpStateMetricsSuffixMappingUncombined maps each TCP state to its own individual
	// metric suffix (matching combine_connection_states: false, mirroring the Python check).
	tcpStateMetricsSuffixMappingUncombined = map[string]string{
		"ESTABLISHED": "estab",
		"SYN_SENT":    "syn_sent",
		"SYN_RCVD":    "syn_recv",
		"FIN_WAIT_1":  "fin_wait_1",
		"FIN_WAIT_2":  "fin_wait_2",
		"TIME_WAIT":   "time_wait",
		"CLOSED":      "close",
		"CLOSE_WAIT":  "close_wait",
		"LAST_ACK":    "time_wait", // matches Python: last_ack is folded into time_wait
		"LISTEN":      "listen",
		"CLOSING":     "closing",
	}
	// tcpStatsMetrics maps lines of `netstat -s -p tcp` to metrics. netstat prints the counters
	// in the singular when they are 0 or 1.
	tcpStatsMetrics = []struct {
		pattern *regexp.Regexp
		metric  string
	}{
		{regexp.MustCompile(`(?m)^[ \t]*(\d+) data packets? \(\d+ bytes?\) retransmitted[ \t]*$`), "system.net.tcp.retrans_packs"},
		{regexp.MustCompile(`(?m)^[ \t]*(\d+) packets? sent[ \t]*$`), "system.net.tcp.sent_packs"},
		{regexp.MustCompile(`(?m)^[ \t]*(\d+) packets? received[ \t]*$`), "system.net.tcp.rcv_packs"},
	}
	tcpStatsUnprivilegedOnce sync.Once
)

type networkInstanceConfig struct {
	CollectRateMetrics       bool     `yaml:"collect_rate_metrics"`
	CollectCountMetrics      bool     `yaml:"collect_count_metrics"`
	CollectConnectionState   bool     `yaml:"collect_connection_state"`
	CombineConnectionStates  bool     `yaml:"combine_connection_states"`
	ExcludedInterfaces       []string `yaml:"excluded_interfaces"`
	ExcludedInterfaceRe      string   `yaml:"excluded_interface_re"`
	ExcludedInterfacePattern *regexp.Regexp
}

type networkStats interface {
	Interfaces() ([]darwinInterface, error)
	Netstat(args ...string) (string, error)
	Privileged() bool
}

type defaultNetworkStats struct {
	nics *nicInfoCache
}

// Interfaces returns every interface with its counters, kernel state and IOKit details.
func (s defaultNetworkStats) Interfaces() ([]darwinInterface, error) {
	ifaces, err := getInterfaces()
	if err != nil {
		return nil, err
	}
	s.nics.enrich(ifaces)
	return ifaces, nil
}

// Netstat returns the output of netstat with args, giving up after netstatTimeout.
func (defaultNetworkStats) Netstat(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), netstatTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/usr/sbin/netstat", args...).Output()
	return string(output), err
}

// Privileged tells whether the check runs as root, the only user that macOS reports TCP
// statistics to.
func (defaultNetworkStats) Privileged() bool {
	return os.Geteuid() == 0
}

// Run executes the check
func (c *NetworkCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	ifaces, err := c.net.Interfaces()
	if err != nil {
		return err
	}
	infos := make([]ifaceInfo, 0, len(ifaces))
	for _, iface := range ifaces {
		if c.isDeviceExcluded(iface.name) {
			continue
		}
		// Interfaces that never carried traffic get no rates, as with the Python check
		if iface.ibytes != 0 || iface.obytes != 0 {
			submitInterfaceMetrics(sender, iface)
		}
		info := iface.ifaceInfo()
		submitIfaceMetrics(sender, info)
		infos = append(infos, info)
	}
	c.setInterfacesMetadata(infos)

	c.submitTCPStats(sender)

	if c.config.instance.CollectConnectionState {
		c.submitConnectionStates(sender)
	}

	sender.Commit()
	return nil
}

// submitInterfaceMetrics submits the byte, packet, drop and error rates of an interface.
func submitInterfaceMetrics(sender sender.Sender, iface darwinInterface) {
	tags := []string{"device:" + iface.name}
	sender.Rate("system.net.bytes_rcvd", float64(iface.ibytes), "", tags)
	sender.Rate("system.net.bytes_sent", float64(iface.obytes), "", tags)
	sender.Rate("system.net.packets_in.count", float64(iface.ipackets), "", tags)
	sender.Rate("system.net.packets_in.drop", float64(iface.iqdrops), "", tags)
	sender.Rate("system.net.packets_in.error", float64(iface.ierrors), "", tags)
	sender.Rate("system.net.packets_out.count", float64(iface.opackets), "", tags)
	sender.Rate("system.net.packets_out.drop", float64(iface.sendDrops), "", tags)
	sender.Rate("system.net.packets_out.error", float64(iface.oerrors), "", tags)
}

// submitTCPStats submits the TCP packet and retransmission counters of `netstat -s -p tcp`.
func (c *NetworkCheck) submitTCPStats(sender sender.Sender) {
	if !c.net.Privileged() {
		tcpStatsUnprivilegedOnce.Do(func() {
			log.Debug("Not collecting TCP stats, which macOS only reports to root")
		})
		return
	}
	output, err := c.net.Netstat("-s", "-p", "tcp")
	if err != nil {
		log.Warnf("Unable to get TCP stats from netstat: %s", err)
		return
	}
	for _, m := range tcpStatsMetrics {
		match := m.pattern.FindStringSubmatch(output)
		if match == nil {
			continue
		}
		value, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			continue
		}
		if c.config.instance.CollectRateMetrics {
			sender.Rate(m.metric, value, "", nil)
		}
		if c.config.instance.CollectCountMetrics {
			sender.MonotonicCount(m.metric+".count", value, "", nil)
		}
	}
}

// submitConnectionStates submits the number of TCP connections in each state and the number of
// UDP sockets, as `netstat -n -a` lists them.
func (c *NetworkCheck) submitConnectionStates(sender sender.Sender) {
	statesByProtocol := map[string][]string{}
	for _, protocol := range []string{"tcp", "udp"} {
		output, err := c.net.Netstat("-n", "-a", "-p", protocol)
		if err != nil {
			log.Warnf("Unable to get %s connection states from netstat: %s", protocol, err)
			return
		}
		parseNetstatSockets(output, statesByProtocol)
	}

	tcpMapping := tcpStateMetricsSuffixMapping
	if !c.config.instance.CombineConnectionStates {
		tcpMapping = tcpStateMetricsSuffixMappingUncombined
	}
	for _, protocol := range []string{"tcp4", "tcp6"} {
		metricCount := map[string]float64{}
		for _, suffix := range tcpMapping {
			metricCount[suffix] = 0
		}
		for _, state := range statesByProtocol[protocol] {
			if suffix, ok := tcpMapping[state]; ok {
				metricCount[suffix]++
			} else {
				log.Debugf("%s state mapping not found for %s", protocol, state)
			}
		}
		for suffix, count := range metricCount {
			sender.Gauge(fmt.Sprintf("system.net.%s.%s", protocol, suffix), count, "", nil)
		}
	}
	for _, protocol := range []string{"udp4", "udp6"} {
		sender.Gauge(fmt.Sprintf("system.net.%s.connections", protocol), float64(len(statesByProtocol[protocol])), "", nil)
	}
}

// parseNetstatSockets appends the state of each socket that `netstat -n -a -p tcp|udp` lists to
// statesByProtocol. UDP sockets have no state, and dual-stack sockets (tcp46, udp46) count as IPv4.
func parseNetstatSockets(output string, statesByProtocol map[string][]string) {
	for line := range strings.Lines(output) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch proto := fields[0]; {
		case strings.HasPrefix(proto, "tcp") && len(fields) >= 6:
			protocol := "tcp4"
			if proto == "tcp6" {
				protocol = "tcp6"
			}
			statesByProtocol[protocol] = append(statesByProtocol[protocol], fields[5])
		case strings.HasPrefix(proto, "udp"):
			protocol := "udp4"
			if proto == "udp6" {
				protocol = "udp6"
			}
			statesByProtocol[protocol] = append(statesByProtocol[protocol], "")
		}
	}
}

// isDeviceExcluded tells whether excluded_interfaces or excluded_interface_re exclude the interface.
func (c *NetworkCheck) isDeviceExcluded(deviceName string) bool {
	if slices.Contains(c.config.instance.ExcludedInterfaces, deviceName) {
		return true
	}
	if c.config.instance.ExcludedInterfacePattern != nil {
		return c.config.instance.ExcludedInterfacePattern.MatchString(deviceName)
	}
	return false
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

// newCheck returns a network check with the default configuration.
func newCheck(_ config.Component) check.Check {
	return &NetworkCheck{
		CheckBase: core.NewCheckBase(CheckName),
		net:       defaultNetworkStats{nics: newNICInfoCache(nicinfo.Get)},
		config: networkConfig{
			instance: networkInstanceConfig{
				CollectRateMetrics:      true,
				CombineConnectionStates: true,
			},
		},
	}
}
