// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package networkv2 provides a check for network connection and socket statistics
package networkv2

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/spf13/afero"
	"golang.org/x/sys/unix"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/safchain/ethtool"
	"github.com/vishvananda/netlink"
)

const (
	// CheckName is the name of the check
	CheckName = "network"
)

var (
	filesystem = afero.NewOsFs()

	getNewEthtool = newEthtool

	runCommandFunction  = runCommand
	ssAvailableFunction = checkSSExecutable
)

type ethtoolInterface interface {
	DriverInfo(intf string) (ethtool.DrvInfo, error)
	Stats(intf string) (map[string]uint64, error)
	Close()
}

var _ ethtoolInterface = (*ethtool.Ethtool)(nil)

func newEthtool() (ethtoolInterface, error) {
	eth, err := ethtool.NewEthtool()
	return eth, err
}

// NetworkCheck represent a network check
type NetworkCheck struct {
	core.CheckBase
	net    networkStats
	config networkConfig
}

type networkInstanceConfig struct {
	CollectRateMetrics        bool     `yaml:"collect_rate_metrics"`
	CollectCountMetrics       bool     `yaml:"collect_count_metrics"`
	CollectConnectionState    bool     `yaml:"collect_connection_state"`
	CollectConnectionQueues   bool     `yaml:"collect_connection_queues"`
	ExcludedInterfaces        []string `yaml:"excluded_interfaces"`
	ExcludedInterfaceRe       string   `yaml:"excluded_interface_re"`
	ExcludedInterfacePattern  *regexp.Regexp
	CollectEthtoolStats       bool
	CollectEthtoolMetrics     bool     `yaml:"collect_ethtool_metrics"`
	CollectEnaMetrics         bool     `yaml:"collect_aws_ena_metrics"`
	ConntrackPath             string   `yaml:"conntrack_path"`
	UseSudoConntrack          bool     `yaml:"use_sudo_conntrack"`
	BlacklistConntrackMetrics []string `yaml:"blacklist_conntrack_metrics"`
	WhitelistConntrackMetrics []string `yaml:"whitelist_conntrack_metrics"`
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
	NetstatAndSnmpCounters(protocols []string) (map[string]net.ProtoCountersStat, error)
	GetProcPath() string
	GetNetProcBasePath() string
	GetConnectionTelemetry() telemetry.Gauge
	GetRecvQTelemetry() telemetry.Gauge
	GetSendQTelemetry() telemetry.Gauge
	GetConntrackTelemetry() telemetry.Gauge
}

type defaultNetworkStats struct {
	procPath          string
	tlmConnectionDiff telemetry.Gauge
	tlmRecvQDiff      telemetry.Gauge
	tlmSendQDiff      telemetry.Gauge
	tlmConntrackDiff  telemetry.Gauge
}

type connectionStateEntry struct {
	count uint64
	recvQ []uint64
	sendQ []uint64
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

func (n defaultNetworkStats) NetstatAndSnmpCounters(protocols []string) (map[string]net.ProtoCountersStat, error) {
	return netstatAndSnmpCounters(n.GetNetProcBasePath(), protocols)
}

func (n defaultNetworkStats) GetProcPath() string {
	return n.procPath
}

func (n defaultNetworkStats) GetNetProcBasePath() string {
	netProcfsPath := n.procPath
	// in a containerized environment
	if os.Getenv("DOCKER_DD_AGENT") != "" && netProcfsPath != "/proc" {
		netProcfsPath = fmt.Sprintf("%s/1", netProcfsPath)
	}
	return netProcfsPath
}

func (n defaultNetworkStats) GetConnectionTelemetry() string {
	return n.tlmConnectionDiff
}

func (n defaultNetworkStats) GetRecvQTelemetry() string {
	return n.tlmRecvQDiff
}

func (n defaultNetworkStats) GetSendQTelemetry() string {
	return n.tlmSendQDiff
}

func (n defaultNetworkStats) GetConntrackTelemetry() string {
	return n.tlmConntrackDiff
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

	protocols := []string{"Tcp", "TcpExt", "Ip", "IpExt", "Udp"}
	counters, err := c.net.NetstatAndSnmpCounters(protocols)
	if err != nil {
		log.Debug(err)
	} else {
		for _, protocol := range protocols {
			if _, ok := counters[protocol]; ok {
				c.submitProtocolMetrics(sender, counters[protocol])
			}
		}
		if tcpStats, ok := counters["Tcp"]; ok {
			if val, ok := tcpStats.Stats["CurrEstab"]; ok {
				sender.Gauge("system.net.tcp.current_established", float64(val), "", nil)
			}
		}
	}

	submitInterfaceSysMetrics(sender)

	ethtoolObject, err := getNewEthtool()
	if err != nil {
		log.Errorf("Failed to create ethtool object: %s", err)
		return err
	}
	defer ethtoolObject.Close()

	for _, interfaceIO := range ioByInterface {
		if !c.isInterfaceExcluded(interfaceIO.Name) {
			submitInterfaceMetrics(sender, interfaceIO)
			if c.config.instance.CollectEthtoolStats {
				err = handleEthtoolStats(sender, ethtoolObject, interfaceIO, c.config.instance.CollectEnaMetrics, c.config.instance.CollectEthtoolMetrics)
				if err != nil {
					return err
				}
			}
		}
	}

	if c.config.instance.CollectConnectionState {
		netProcfsBasePath := c.net.GetNetProcBasePath()
		for _, protocol := range []string{"udp4", "udp6", "tcp4", "tcp6"} {
			submitConnectionStateMetrics(sender, protocol, c.config.instance.CollectConnectionQueues, netProcfsBasePath, c.net.GetConnectionTelemetry(), c.net.GetRecvQTelemetry(), c.net.GetSendQTelemetry())
		}
	}

	setProcPath := c.net.GetProcPath()
	collectConntrackMetrics(sender, c.config.instance.ConntrackPath, c.config.instance.UseSudoConntrack, setProcPath, c.config.instance.BlacklistConntrackMetrics, c.config.instance.WhitelistConntrackMetrics, c.net.GetConntrackTelemetry())

	sender.Commit()
	return nil
}

func (c *NetworkCheck) isInterfaceExcluded(interfaceName string) bool {
	if slices.Contains(c.config.instance.ExcludedInterfaces, interfaceName) {
		log.Debugf("Skipping network interface %s", interfaceName)
		return true
	}
	if c.config.instance.ExcludedInterfacePattern != nil && c.config.instance.ExcludedInterfacePattern.MatchString(interfaceName) {
		log.Debugf("Skipping network interface from match: %s", interfaceName)
		return true
	}
	return false
}

func submitInterfaceSysMetrics(sender sender.Sender) {
	sysNetLocation := "/sys/class/net"
	sysNetMetrics := []string{"mtu", "tx_queue_len", "up"}
	ifaces, err := afero.ReadDir(filesystem, sysNetLocation)
	if err != nil {
		log.Debugf("Unable to list %s, skipping system iface metrics: %s.", sysNetLocation, err)
		return
	}
	for _, iface := range ifaces {
		ifaceTag := []string{fmt.Sprintf("iface:%s", iface.Name())}
		for _, metricName := range sysNetMetrics {
			metricFileName := metricName
			if metricName == "up" {
				metricFileName = "carrier"
			}
			metricFilepath := filepath.Join(sysNetLocation, iface.Name(), metricFileName)
			val, err := readIntFile(metricFilepath, filesystem)
			if err != nil {
				log.Debugf("Unable to read %s, skipping: %s.", metricFilepath, err)
			}
			sender.Gauge(fmt.Sprintf("system.net.iface.%s", metricName), float64(val), "", ifaceTag)
		}
		queuesFilepath := filepath.Join(sysNetLocation, iface.Name(), "queues")
		queues, err := afero.ReadDir(filesystem, queuesFilepath)
		if err != nil {
			log.Debugf("Unable to list %s, skipping: %s.", queuesFilepath, err)
		} else {
			txQueueCount, rxQueueCount := 0, 0
			for _, queue := range queues {
				if strings.HasPrefix(queue.Name(), "tx-") {
					txQueueCount++
				} else if strings.HasPrefix(queue.Name(), "rx-") {
					rxQueueCount++
				}
			}
			sender.Gauge("system.net.iface.num_tx_queues", float64(txQueueCount), "", ifaceTag)
			sender.Gauge("system.net.iface.num_rx_queues", float64(rxQueueCount), "", ifaceTag)
		}
	}
}

func submitInterfaceMetrics(sender sender.Sender, interfaceIO net.IOCountersStat) {
	tags := []string{fmt.Sprintf("device:%s", interfaceIO.Name), fmt.Sprintf("device_name:%s", interfaceIO.Name)}
	speedVal, err := readIntFile(fmt.Sprintf("/sys/class/net/%s/speed", interfaceIO.Name), filesystem)
	if err == nil {
		tags = append(tags, fmt.Sprintf("speed:%s", strconv.Itoa(speedVal)))
	}
	mtuVal, err := readIntFile(fmt.Sprintf("/sys/class/net/%s/mtu", interfaceIO.Name), filesystem)
	if err == nil {
		tags = append(tags, fmt.Sprintf("mtu:%s", strconv.Itoa(mtuVal)))
	}
	sender.Rate("system.net.bytes_rcvd", float64(interfaceIO.BytesRecv), "", tags)
	sender.Rate("system.net.bytes_sent", float64(interfaceIO.BytesSent), "", tags)
	sender.Rate("system.net.packets_in.count", float64(interfaceIO.PacketsRecv), "", tags)
	sender.Rate("system.net.packets_in.drop", float64(interfaceIO.Dropin), "", tags)
	sender.Rate("system.net.packets_in.error", float64(interfaceIO.Errin), "", tags)
	sender.Rate("system.net.packets_out.count", float64(interfaceIO.PacketsSent), "", tags)
	sender.Rate("system.net.packets_out.drop", float64(interfaceIO.Dropout), "", tags)
	sender.Rate("system.net.packets_out.error", float64(interfaceIO.Errout), "", tags)
}

func handleEthtoolStats(sender sender.Sender, ethtoolObject ethtoolInterface, interfaceIO net.IOCountersStat, collectEnaMetrics bool, collectEthtoolMetrics bool) error {
	if interfaceIO.Name == "lo" || interfaceIO.Name == "lo0" {
		// Skip loopback ifaces as they don't support SIOCETHTOOL
		log.Debugf("Skipping loopbackinterface %s", interfaceIO.Name)
		return nil
	}

	// Preparing the interface name and copy it into the request
	iface := interfaceIO.Name
	if len(iface) > 15 {
		iface = iface[:15]
	}

	// Fetch driver information (ETHTOOL_GDRVINFO)
	drvInfo, err := ethtoolObject.DriverInfo(iface)
	if err != nil {
		if err == unix.ENOTTY || err == unix.EOPNOTSUPP {
			log.Debugf("driver info is not supported for interface: %s", interfaceIO.Name)
		} else {
			return errors.New("failed to get driver info for interface " + interfaceIO.Name + ": " + fmt.Sprintf("%d", err))
		}
	}

	replacer := strings.NewReplacer("\x00", "")
	driverName := replacer.Replace(drvInfo.Driver)
	driverVersion := replacer.Replace(drvInfo.Version)

	// Fetch ethtool stats values (ETHTOOL_GSTATS)
	statsMap, err := ethtoolObject.Stats(iface)
	if err != nil {
		if err == unix.ENOTTY || err == unix.EOPNOTSUPP {
			log.Debugf("ethtool stats are not supported for interface: %s", interfaceIO.Name)
		} else {
			return errors.New("failed to get ethtool stats information for interface " + interfaceIO.Name + ": " + fmt.Sprintf("%d", err))
		}
	}

	if collectEnaMetrics {
		enaMetrics := getEnaMetrics(statsMap)
		tags := []string{
			"device:" + interfaceIO.Name,
			"driver_name:" + driverName,
			"driver_version:" + driverVersion,
		}

		count := 0
		for metricName, metricValue := range enaMetrics {
			metricName := fmt.Sprintf("system.net.%s", metricName)
			sender.Gauge(metricName, float64(metricValue), "", tags)
			count++
		}
		log.Debugf("tracked %d network ena metrics for interface %s", count, interfaceIO.Name)
	}

	if collectEthtoolMetrics {
		processedMap := getEthtoolMetrics(driverName, statsMap)
		for extraTag, keyValuePairing := range processedMap {
			tags := []string{
				"device:" + interfaceIO.Name,
				"driver_name:" + driverName,
				"driver_version:" + driverVersion,
				extraTag,
			}

			for metricName, metricValue := range keyValuePairing {
				metricName := fmt.Sprintf("system.net.%s", metricName)
				sender.MonotonicCount(metricName, float64(metricValue), "", tags)
			}
		}
	}

	return nil
}

func getEnaMetrics(statsMap map[string]uint64) map[string]uint64 {
	metrics := make(map[string]uint64)

	for stat, value := range statsMap {
		if slices.Contains(enaMetricNames, stat) {
			metrics[enaMetricPrefix+stat] = value
		}
	}

	return metrics
}

func getEthtoolMetrics(driverName string, statsMap map[string]uint64) map[string]map[string]uint64 {
	result := map[string]map[string]uint64{}
	if _, ok := ethtoolMetricNames[driverName]; !ok {
		return result
	}
	ethtoolGlobalMetrics := []string{}
	if _, ok := ethtoolGlobalMetricNames[driverName]; ok {
		ethtoolGlobalMetrics = ethtoolGlobalMetricNames[driverName]
	}
	keys := make([]string, 0, len(statsMap))
	values := make([]uint64, 0, len(statsMap))
	for key, value := range statsMap {
		keys = append(keys, key)
		values = append(values, value)
	}
	for keyIndex := 0; keyIndex < len(keys); keyIndex++ {
		statName := keys[keyIndex]
		continueCase := true
		queueTag := ""
		newKey := ""
		metricPrefix := ""
		if strings.Contains(statName, "queue_") {
			// Extract the queue and the metric name from ethtool stat name:
			//   queue_0_tx_cnt -> (queue:0, tx_cnt)
			//   tx_queue_0_bytes -> (queue:0, tx_bytes)
			parts := strings.Split(statName, "_")
			queueIndex := -1
			for i, part := range parts {
				if part == "queue" && i+1 < len(parts) {
					if _, err := strconv.Atoi(parts[i+1]); err == nil {
						queueIndex = i
						break
					}
				}
			}
			// It's possible the stat name contains the string "queue" but does not have an index
			// In this case, this is not actually a queue metric and we should keep trying
			if queueIndex > -1 {
				queueNum := parts[queueIndex+1]
				parts = append(parts[:queueIndex], parts[queueIndex+2:]...)
				queueTag = "queue:" + queueNum
				newKey = strings.Join(parts, "_")
				metricPrefix = ".queue."
			}
		}
		if continueCase {
			// Extract the cpu and the metric name from ethtool stat name:
			//   cpu0_rx_bytes -> (cpu:0, rx_bytes)
			if strings.HasPrefix(statName, "cpu") {
				parts := strings.Split(statName, "_")
				if len(parts) >= 2 {
					cpuNum := parts[0][3:]
					if _, err := strconv.Atoi(cpuNum); err == nil {
						queueTag = "cpu:" + cpuNum
						newKey = strings.Join(parts[1:], "_")
						metricPrefix = ".cpu."
						continueCase = false
					}
				}
			}
		}
		if continueCase {
			if strings.Contains(statName, "[") && strings.HasSuffix(statName, "]") {
				// Extract the queue and the metric name from ethtool stat name:
				//   tx_stop[0] -> (queue:0, tx_stop)
				parts := strings.SplitN(statName, "[", 2)
				if len(parts) == 2 {
					metricName := parts[0]
					queueNum := strings.TrimSuffix(parts[1], "]")
					if _, err := strconv.Atoi(queueNum); err == nil {
						queueTag = "queue:" + queueNum
						newKey = metricName
						metricPrefix = ".queue."
						continueCase = false
					}
				}
			}
		}
		if continueCase {
			if strings.HasPrefix(statName, "rx") || strings.HasPrefix(statName, "tx") {
				// Extract the queue and the metric name from ethtool stat name:
				//   tx0_bytes -> (queue:0, tx_bytes)
				//   rx1_packets -> (queue:1, rx_packets)
				parts := strings.Split(statName, "_")
				queueIndex := -1
				queueNum := -1
				for i, part := range parts {
					if !strings.HasPrefix(part, "rx") && !strings.HasPrefix(part, "tx") {
						continue
					}
					if num, err := strconv.Atoi(part[2:]); err == nil {
						queueIndex = i
						queueNum = num
						break
					}
				}
				if queueIndex > -1 {
					parts[queueIndex] = parts[queueIndex][:2]
					queueTag = fmt.Sprintf("queue:%d", queueNum)
					newKey = strings.Join(parts, "_")
					metricPrefix = ".queue."
					continueCase = false
				}
			}
		}
		if continueCase {
			// if we've made it this far, check if the stat name is a global metric for the NIC
			if statName != "" {
				if slices.Contains(ethtoolGlobalMetrics, statName) {
					queueTag = "global"
					newKey = statName
					metricPrefix = "."
				}
			}
		}
		if newKey != "" && queueTag != "" && metricPrefix != "" {
			if queueTag != "global" {
				// we already guard against parsing unsupported NICs
				queueMetrics := ethtoolMetricNames[driverName]
				// skip queues metrics we don't support for the NIC
				if !slices.Contains(queueMetrics, newKey) {
					continue
				}
			}
			if result[queueTag] == nil {
				result[queueTag] = make(map[string]uint64)
			}
			result[queueTag][driverName+metricPrefix+newKey] = values[keyIndex]
		}
	}
	return result
}

func (c *NetworkCheck) submitProtocolMetrics(sender sender.Sender, protocolStats net.ProtoCountersStat) {
	if protocolMapping, ok := protocolsMetricsMapping[protocolStats.Protocol]; ok {
		for rawMetricName, metricName := range protocolMapping {
			if metricValue, ok := protocolStats.Stats[rawMetricName]; ok {
				if c.config.instance.CollectRateMetrics {
					sender.Rate(metricName, float64(metricValue), "", nil)
				}
				if c.config.instance.CollectCountMetrics {
					sender.MonotonicCount(fmt.Sprintf("%s.count", metricName), float64(metricValue), "", nil)
				}
			}
		}
	}
}

// Try using `ss` for increased performance over `netstat`
func checkSSExecutable() bool {
	_, err := exec.LookPath("ss")
	if err != nil {
		log.Debug("`ss` executable not found in system PATH")
		return false
	}
	return true
}

func getStateMetricsFromNetlink(protocol string, procfsPath string) (map[string]*connectionStateEntry, error) {
	results := make(map[string]*connectionStateEntry)

	suffixMapping := netlinkStateMetricsMapping
	if protocol[:3] == "udp" {
		results["connections"] = &connectionStateEntry{
			count: 0,
			recvQ: []uint64{},
			sendQ: []uint64{},
		}
	} else {
		for _, state := range suffixMapping {
			if state == "connections" {
				continue
			}
			if _, exists := results[state]; !exists {
				results[state] = &connectionStateEntry{
					count: 0,
					recvQ: []uint64{},
					sendQ: []uint64{},
				}
			}
		}
	}

	switch protocol {
	case "tcp4", "tcp6":
		var family uint8
		if strings.HasSuffix(protocol, "6") {
			family = unix.AF_INET6
		} else {
			family = unix.AF_INET
		}

		conns, err := netlink.SocketDiagTCPInfo(family)
		for _, conn := range conns {
			diagMsg := conn.InetDiagMsg

			if diagMsg == nil {
				continue
			}

			state, ok := suffixMapping[diagMsg.State]
			if !ok {
				continue
			}

			recvQ := parseQueue(fields[1])
			sendQ := parseQueue(fields[2])
			if entry, exists := results[state]; exists {
				entry.count = entry.count + 1
				entry.recvQ = append(entry.recvQ, diagMsg.RQueue)
				entry.sendQ = append(entry.sendQ, diagMsg.WQueue)
			}
		}
	case "udp4", "udp6":
		var family uint8
		if strings.HasSuffix(protocol, "6") {
			family = unix.AF_INET6
		} else {
			family = unix.AF_INET
		}

		conns, err := netlink.SocketDiagUDPInfo(family)
		for _, conn := range conns {
			diagMsg := conn.InetDiagMsg

			if diagMsg == nil {
				continue
			}

			state = "connections"
			if entry, exists := results[state]; exists {
				entry.count = entry.count + 1
				entry.recvQ = append(entry.recvQ, diagMsg.RQueue)
				entry.sendQ = append(entry.sendQ, diagMsg.WQueue)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}

	return results, nil
}

func getSocketStateMetrics(protocol string, procfsPath string) (map[string]*connectionStateEntry, error) {
	env := []string{"PROC_ROOT=" + procfsPath}
	// Pass the IP version to `ss` because there's no built-in way of distinguishing between the IP versions in the output
	// Also calls `ss` for each protocol, because on some systems (e.g. Ubuntu 14.04), there is a bug that print `tcp` even if it's `udp`
	// The `-H` flag isn't available on old versions of `ss`.

	ipFlag := fmt.Sprintf("--ipv%s", protocol[len(protocol)-1:])
	protocolFlag := fmt.Sprintf("--%s", protocol[:len(protocol)-1])
	// Go's exec.Command environment is the same as the running process unlike python so we do not need to adjust the PATH
	cmd := fmt.Sprintf("ss --numeric %s --all %s", protocolFlag, ipFlag)
	output, err := runCommandFunction([]string{"sh", "-c", cmd}, env)
	if err != nil {
		return nil, fmt.Errorf("error executing ss command: %v", err)
	}
	return parseSocketStatsMetrics(protocol, output)
}

func getNetstatStateMetrics(protocol string, _ string) (map[string]*connectionStateEntry, error) {
	output, err := runCommandFunction([]string{"netstat", "-n", "-u", "-t", "-a"}, []string{})
	if err != nil {
		return nil, fmt.Errorf("error executing netstat command: %v", err)
	}
	return parseNetstatMetrics(protocol, output)
}

// why not sum here
func parseSocketStatsMetrics(protocol, output string) (map[string]*connectionStateEntry, error) {
	results := make(map[string]*connectionStateEntry)

	suffixMapping := tcpStateMetricsSuffixMapping["ss"]
	if protocol[:3] == "udp" {
		results["connections"] = &connectionStateEntry{
			count: 0,
			recvQ: []uint64{},
			sendQ: []uint64{},
		}
	} else {
		for _, state := range suffixMapping {
			if state == "connections" {
				continue
			}
			if _, exists := results[state]; !exists {
				results[state] = &connectionStateEntry{
					count: 0,
					recvQ: []uint64{},
					sendQ: []uint64{},
				}
			}
		}
	}

	// State       Recv-Q   Send-Q     Local Address:Port          Peer Address:Port
	// LISTEN      0        4096       127.0.0.53%lo:53                 0.0.0.0:*
	// LISTEN      0        4096           127.0.0.1:5001               0.0.0.0:*
	// LISTEN      0        4096           127.0.0.1:5000               0.0.0.0:*
	// LISTEN      0        10               0.0.0.0:27500              0.0.0.0:*
	// LISTEN      0        4096          127.0.0.54:53                 0.0.0.0:*
	// LISTEN      0        4096             0.0.0.0:5355               0.0.0.0:*
	// LISTEN      0        4096           127.0.0.1:631                0.0.0.0:*
	// SYN-SENT    0        1           192.168.64.6:46118      169.254.169.254:80
	// ESTAB       0        0           192.168.64.6:50204        3.233.157.145:443
	// ESTAB       0        0              127.0.0.1:51064            127.0.0.1:5001
	// ESTAB       0        0           192.168.64.6:50522        34.107.243.93:443
	// SYN-SENT    0        1           192.168.64.6:46104      169.254.169.254:80
	// SYN-SENT    0        1           192.168.64.6:46124      169.254.169.254:80
	// SYN-SENT    0        1           192.168.64.6:56644      169.254.169.254:80
	// ESTAB       0        0           192.168.64.6:55976         3.233.158.71:443
	// TIME-WAIT   0        0           192.168.64.6:38964        3.233.157.100:443
	// SYN-SENT    0        1           192.168.64.6:56654      169.254.169.254:80
	// ESTAB       0        0              127.0.0.1:5001             127.0.0.1:51064
	// SYN-SENT    0        1           192.168.64.6:56650      169.254.169.254:80
	// SYN-SENT    0        1           192.168.64.6:53594      100.100.100.200:80

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		// skip malformed ss entry result
		if len(fields) < 3 {
			continue
		}

		var stateField string
		// skip the header
		if fields[0] == "State" {
			continue
		}
		if protocol[:3] == "udp" {
			// all UDP suffixes resolve to connections
			stateField = "NONE"
		} else {
			stateField = fields[0]
		}
		// skip connection states we do not have mappings for
		state, ok := suffixMapping[stateField]
		if !ok {
			continue
		}

		recvQ := parseQueue(fields[1])
		sendQ := parseQueue(fields[2])
		if entry, exists := results[state]; exists {
			entry.count = entry.count + 1
			entry.recvQ = append(entry.recvQ, recvQ)
			entry.sendQ = append(entry.sendQ, sendQ)
		}
	}
	return results, nil
}

func parseNetstatMetrics(protocol, output string) (map[string]*connectionStateEntry, error) {
	protocol = strings.ReplaceAll(protocol, "4", "") // the output entry is tcp, tcp6, udp, udp6 so we need to strip the 4
	results := make(map[string]*connectionStateEntry)
	suffixMapping := tcpStateMetricsSuffixMapping["netstat"]
	if protocol[:3] == "udp" {
		results["connections"] = &connectionStateEntry{
			count: 0,
			recvQ: []uint64{},
			sendQ: []uint64{},
		}
	} else {
		for _, state := range suffixMapping {
			if state == "connections" {
				continue
			}
			if _, exists := results[state]; !exists {
				results[state] = &connectionStateEntry{
					count: 0,
					recvQ: []uint64{},
					sendQ: []uint64{},
				}
			}
		}
	}

	// Active Internet connections (w/o servers)
	// Proto Recv-Q Send-Q Local Address           Foreign Address         State
	// tcp        0      0 46.105.75.4:80          79.220.227.193:2032     SYN_RECV
	// tcp        0      0 46.105.75.4:143         90.56.111.177:56867     ESTABLISHED
	// tcp        0      0 46.105.75.4:50468       107.20.207.175:443      TIME_WAIT
	// tcp6       0      0 46.105.75.4:80          93.15.237.188:58038     FIN_WAIT2
	// tcp6       0      0 46.105.75.4:80          79.220.227.193:2029     ESTABLISHED
	// udp        0      0 0.0.0.0:123             0.0.0.0:*
	// udp        0      0 192.168.64.6:68         192.168.64.1:67         ESTABLISHED
	// udp6       0      0 :::41458                :::*
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)

		if len(fields) < 5 {
			continue
		}

		// filter out the rows that do not match the current protocol enumeration
		entryProtocol := fields[0]
		if protocol != entryProtocol {
			continue
		}

		var stateField string
		if protocol[:3] == "udp" {
			// all UDP suffixes resolve to connections
			stateField = "NONE"
		} else {
			stateField = fields[5]
		}
		// skip connection states we do not have mappings for
		state, ok := suffixMapping[stateField]
		if !ok {
			continue
		}

		recvQ := parseQueue(fields[1])
		sendQ := parseQueue(fields[2])
		if entry, exists := results[state]; exists {
			entry.count = entry.count + 1
			entry.recvQ = append(entry.recvQ, recvQ)
			entry.sendQ = append(entry.sendQ, sendQ)
		}
	}
	return results, nil
}

func parseQueue(queueStr string) uint64 {
	var queue uint64
	_, err := fmt.Sscanf(queueStr, "%d", &queue)
	if err != nil {
		return 0
	}
	return queue
}

func submitConnectionStateMetrics(
	sender sender.Sender,
	protocolName string,
	collectConnectionQueues bool,
	procfsPath string,
	tlmConnectionDiff telemetry.Gauge,
	tlmRecvQDiff telemetry.Gauge,
	tlmSendQDiff telemetry.Gauge,
) {
	var getStateMetrics func(ipVersion string, procfsPath string) (map[string]*connectionStateEntry, error)
	if ssAvailableFunction() {
		log.Debug("Using `ss` for connection state metrics")
		getStateMetrics = getSocketStateMetrics
	} else {
		log.Debug("Using `netstat` for connection state metrics")
		getStateMetrics = getNetstatStateMetrics
	}

	results, err := getStateMetrics(protocolName, procfsPath)
	if err != nil {
		log.Debug("Error getting connection state metrics:", err)
		return
	}
	netlinkRes, err = getStateMetricsFromNetlink(protocolName)
	if err != nil {
		log.Debug("Error getting connection states from netlink:", err)
	}

	for suffix, metrics := range results {
		sender.Gauge(fmt.Sprintf("system.net.%s.%s", protocolName, suffix), float64(metrics.count), "", nil)
		if netlinkRes != nil {
			diff := float64(metrics.count - netlinkRes.count)
			tlmConnectionDiff.Set(diff, protocolName, suffix)
		}
		if collectConnectionQueues && protocolName[:3] == "tcp" {
			recvQSum := 0
			sendQSum := 0
			for _, point := range metrics.recvQ {
				recvQSum += point
				sender.Histogram("system.net.tcp.recv_q", float64(point), "", []string{"state:" + suffix})
			}

			for _, point := range metrics.sendQ {
				sendQSum += point
				sender.Histogram("system.net.tcp.send_q", float64(point), "", []string{"state:" + suffix})
			}

			if netlinkRes != nil {
				netlinkRecvQSum := 0
				netlinkSendQSum := 0

				for _, netlinkPoint := range netlinkRes.recvQ {
					netlinkRecvQSum += netlinkPoint
				}
				for _, netlinkPoint := range netlinkRes.recvQ {
					netlinkSendQSum += netlinkPoint
				}

				diff := float64(recvQSum - netlinkRecvQSum)
				tlmRecvQDiff.Set(diff, protocolName, suffix)
				diff := float64(sendQSum - netlinkSendQSum)
				tlmSendQDiff.Set(diff, protocolName, suffix)
			}
		}
	}
}

func netstatAndSnmpCounters(procfsPath string, protocolNames []string) (map[string]net.ProtoCountersStat, error) {
	counters := make(map[string]net.ProtoCountersStat)
	for _, protocol := range protocolNames {
		counters[protocol] = net.ProtoCountersStat{Protocol: protocol, Stats: make(map[string]int64)}
	}

	for _, subdirectory := range procfsSubdirectories {
		fs := filesystem
		procfsSubdirectoryPath := filepath.Join(procfsPath, "net", subdirectory)
		f, err := fs.Open(procfsSubdirectoryPath)
		if err != nil {
			if subdirectory == "snmp" {
				continue
			}
			return nil, err
		}
		defer f.Close()

		// Inter-|   Receive                                                 |  Transmit
		//  face |bytes     packets errs drop fifo frame compressed multicast|bytes       packets errs drop fifo colls carrier compressed
		//     lo:45890956   112797   0    0    0     0          0         0    45890956   112797    0    0    0     0       0          0
		//   eth0:631947052 1042233   0   19    0   184          0      1206  1208625538  1320529    0    0    0     0       0          0
		//   eth1:       0        0   0    0    0     0          0         0           0        0    0    0    0     0       0          0
		scanner := bufio.NewScanner(f)
		var protocolName string
		for scanner.Scan() {
			line := scanner.Text()
			i := strings.IndexRune(line, ':')
			if i == -1 {
				return nil, fmt.Errorf("%s is not fomatted correctly, expected ':'", procfsSubdirectoryPath)
			}
			if slices.Contains(protocolNames, line[:i]) {
				protocolName = line[:i]
			} else {
				continue
			}

			counterNames := strings.Split(line[i+2:], " ")

			if !scanner.Scan() {
				return nil, fmt.Errorf("%s is not fomatted correctly, not data line", procfsSubdirectoryPath)
			}
			line = scanner.Text()

			counterValues := strings.Split(line[i+2:], " ")
			if len(counterNames) != len(counterValues) {
				return nil, fmt.Errorf("%s is not fomatted correctly, expected same number of columns", procfsSubdirectoryPath)
			}
			for j := range counterNames {
				value, err := strconv.ParseInt(counterValues[j], 10, 64)
				if err != nil {
					return nil, err
				}
				counters[protocolName].Stats[counterNames[j]] = value
			}
		}
	}
	return counters, nil
}

func readIntFile(filePath string, fs afero.Fs) (int, error) {
	data, err := afero.ReadFile(fs, filePath)
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}
	return value, nil
}

type conntrackStat struct {
	cpuID         string
	Found         float64
	Invalid       float64
	Ignore        float64
	Insert        float64
	InsertFailed  float64
	Drop          float64
	EarlyDrop     float64
	Error         float64
	SearchRestart float64
	ClashResolve  float64
	ChainTooLong  float64
}

func addConntrackStatsFromProcFile(procfsPath string) ([]*conntrackStat, error) {
	statFilePath := filepath.Join(procfsPath, "net", "stat", "nf_conntrack")

	f, err := fs.Open(statFilePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	lineNum := 0
	headers := []string{}
	stats := []*conntrackStat{}

	// entries  clashres found new invalid ignore delete chainlength insert insert_failed drop early_drop icmp_error  expect_new expect_create expect_delete search_restart
	// 00000002  000000cd 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000  00000000 00000000 00000000 00000000

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		if lineNum == 0 {
			headers = strings.Split(line, " ")
		} else {
			// each line is a cpu stat, top line is headers
			stat := &conntrackStat{cpuID: lineNum - 1}
			for i, hexVal := range strings.Split(line, " ") {
				val, err := strconv.ParseInt(hexVal, 16, 64)
				if err != nil {
					return nil, err
				}

				switch headers[i] {
				case "found":
					stat.Found = float64(val)
				case "invalid":
					stat.Found = float64(val)
				case "ignore":
					stat.Ignore = float64(val)
				case "insert":
					stat.Insert = float64(val)
				case "insert_failed":
					stat.InsertFailed = float64(val)
				case "drop":
					stat.Drop = float64(val)
				case "early_drop":
					stat.EarlyDrop = float64(val)
				// procfile header string is different depending on version
				case "error", "icmp_error":
					stat.Error = float64(val)
				case "search_restart":
					stat.SearchRestart = float64(val)
				case "clash_resolve":
					stat.ChainTooLong = float64(val)
				case "chaintoolong":
					stat.ChainTooLong = float64(val)
				default:
					continue
				}

			}
			stats = append(stats, stat)
		}

		lineNum += 1
	}

	return stats, nil
}

func addConntrackStatsMetrics(sender sender.Sender, conntrackPath string, useSudoConntrack bool) []*conntrackStat {
	if conntrackPath == "" {
		return
	}

	// In CentOS, conntrack is located in /sbin and /usr/sbin which may not be in the agent user PATH
	cmd := []string{conntrackPath, "-S"}
	if useSudoConntrack {
		cmd = append([]string{"sudo"}, cmd...)
	}

	output, err := runCommandFunction(cmd, []string{})
	if err != nil {
		log.Debugf("Couldn't use %s to get conntrack stats: %v", conntrackPath, err)
		return
	}

	// conntrack -S sample:
	// cpu=0 found=27644 invalid=19060 ignore=485633411 insert=0 insert_failed=1 \
	//       drop=1 early_drop=0 error=0 search_restart=39936711
	// cpu=1 found=21960 invalid=17288 ignore=475938848 insert=0 insert_failed=1 \
	//       drop=1 early_drop=0 error=0 search_restart=36983181
	lines := strings.Split(output, "\n")
	stats := make([]*conntrackStat, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		cols := strings.Fields(line)
		cpuNum := strings.Split(cols[0], "=")[1]
		cols = cols[1:]

		stat := &conntrackStat{cpuID: cpuNum}

		for _, cell := range cols {
			parts := strings.Split(cell, "=")
			if len(parts) != 2 {
				continue
			}
			metric, valueStr := parts[0], parts[1]
			valueFloat, err := strconv.ParseFloat(valueStr, 64)
			if err != nil {
				log.Debugf("Error converting value %s for metric %s: %v", valueStr, metric, err)
				continue
			}

			switch metric {
			case "found":
				stat.Found = valueFloat
			case "invalid":
				stat.Found = valueFloat
			case "ignore":
				stat.Ignore = valueFloat
			case "insert":
				stat.Insert = valueFloat
			case "insert_failed":
				stat.InsertFailed = valueFloat
			case "drop":
				stat.Drop = valueFloat
			case "early_drop":
				stat.EarlyDrop = valueFloat
			case "error":
				stat.Error = valueFloat
			case "search_restart":
				stat.SearchRestart = valueFloat
			case "clash_resolve":
				stat.ClashResolve = valueFloat
			case "chaintoolong":
				stat.ChainTooLong = valueFloat
			default:
				continue
			}
			stats = append(stats, stat)
		}
	}
	return stats
}

func runCommand(cmd []string, env []string) (string, error) {
	execCmd := exec.Command(cmd[0], cmd[1:]...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	execCmd.Env = append(execCmd.Environ(), env...)
	execCmd.Stdout = &out
	execCmd.Stderr = &stderr
	err := execCmd.Run()
	if err != nil {
		return "", fmt.Errorf("error executing command: %v, stderr: %s", err, stderr.String())
	}
	return out.String(), nil
}

func collectConntrackMetrics(sender sender.Sender, conntrackPath string, useSudo bool, procfsPath string, blacklistConntrackMetrics []string, whitelistConntrackMetrics []string, tlmConntrackDiff telemetry.Gauge) {
	stats := addConntrackStatsMetrics(sender, conntrackPath, useSudo)
	procStats, err := addConntrackStatsFromProcFile(procfsPath)
	if err != nil {
		log.Debugf("Unable to acquire conntrack stats from procfile: %v", err)
	}

	for i, stat := range stats {
		cpuTag := []string{"cpu:" + stat.cpuID}
		sender.MonotonicCount("system.net.conntrack.found", stat.Found, "", cpuTag)
		sender.MonotonicCount("system.net.conntrack.invalid", stat.Invalid, "", cpuTag)
		sender.MonotonicCount("system.net.conntrack.ignore", stat.Ignore, "", cpuTag)
		sender.MonotonicCount("system.net.conntrack.insert", stat.Insert, "", cpuTag)
		sender.MonotonicCount("system.net.conntrack.insert_failed", stat.InsertFailed, "", cpuTag)
		sender.MonotonicCount("system.net.conntrack.drop", stat.Drop, "", cpuTag)
		sender.MonotonicCount("system.net.conntrack.early_drop", stat.EarlyDrop, "", cpuTag)
		sender.MonotonicCount("system.net.conntrack.error", stat.Error, "", cpuTag)
		sender.MonotonicCount("system.net.conntrack.search_restart", stat.SearchRestart, "", cpuTag)
		sender.MonotonicCount("system.net.conntrack.clash_resolve", stat.ClashResolve, "", cpuTag)
		sender.MonotonicCount("system.net.conntrack.chaintoolong", stat.ChainTooLong, "", cpuTag)

		if procStats != nil {
			procStat := procStats[i]
			diff := float64(stat.Found - procStat.Found)
			tlmConntrackDiff.Set(diff, stat.cpuID, "found")
			diff := float64(stat.Invalid - procStat.Invalid)
			tlmConntrackDiff.Set(diff, stat.cpuID, "invalid")
			diff := float64(stat.Ignore - procStat.Ignore)
			tlmConntrackDiff.Set(diff, stat.cpuID, "ignore")
			diff := float64(stat.InsertFailed - procStat.InsertFailed)
			tlmConntrackDiff.Set(diff, stat.cpuID, "insert_failed")
			diff := float64(stat.Drop - procStat.Drop)
			tlmConntrackDiff.Set(diff, stat.cpuID, "drop")
			diff := float64(stat.EarlyDrop - procStat.EarlyDrop)
			tlmConntrackDiff.Set(diff, stat.cpuID, "early_drop")
			diff := float64(stat.Error - procStat.Error)
			tlmConntrackDiff.Set(diff, stat.cpuID, "error")
			diff := float64(stat.SearchRestart - procStat.SearchRestart)
			tlmConntrackDiff.Set(diff, stat.cpuID, "search_restart")
			diff := float64(stat.ClashResolve - procStat.ClashResolve)
			tlmConntrackDiff.Set(diff, stat.cpuID, "clash_resolve")
			diff := float64(stat.ChainTooLong - procStat.ChainTooLong)
			tlmConntrackDiff.Set(diff, stat.cpuID, "chaintoolong")
		}
	}

	conntrackFilesLocation := filepath.Join(procfsPath, "sys", "net", "netfilter")
	var availableFiles []string
	fs := filesystem
	files, err := afero.ReadDir(fs, conntrackFilesLocation)
	if err != nil {
		log.Debugf("Unable to list files in %s: %v", conntrackFilesLocation, err)
	} else {
		for _, file := range files {
			if file.Mode().IsRegular() && strings.HasPrefix(file.Name(), "nf_conntrack_") {
				availableFiles = append(availableFiles, strings.TrimPrefix(file.Name(), "nf_conntrack_"))
			}
		}
	}

	// By default, only max and count are reported. However if the blacklist is set,
	// the whitelist is losing its default value
	for _, metricName := range availableFiles {
		if len(blacklistConntrackMetrics) > 0 {
			if slices.ContainsFunc(blacklistConntrackMetrics, func(s string) bool {
				return strings.Contains(metricName, s)
			}) {
				continue
			}
		} else if len(whitelistConntrackMetrics) > 0 {
			if !slices.ContainsFunc(whitelistConntrackMetrics, func(s string) bool {
				return strings.Contains(metricName, s)
			}) {
				continue
			}
		} else {
			if !slices.ContainsFunc([]string{"max", "count"}, func(s string) bool {
				return strings.Contains(metricName, s)
			}) {
				continue
			}
		}
		metricFileLocation := filepath.Join(conntrackFilesLocation, "nf_conntrack_"+metricName)
		value, err := readIntFile(metricFileLocation, fs)
		if err != nil {
			log.Debugf("Error reading %s: %v", metricFileLocation, err)
		}
		sender.Gauge("system.net.conntrack."+metricName, float64(value), "", nil)
	}
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

	if c.config.instance.CollectEthtoolMetrics || c.config.instance.CollectEnaMetrics {
		c.config.instance.CollectEthtoolStats = true
	}

	return nil
}

// Factory creates a new check factory
func Factory(cfg config.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(cfg)
	})
}

func newCheck(cfg config.Component) check.Check {
	procfsPath := "/proc"
	if cfg.IsConfigured("procfs_path") {
		procfsPath = strings.TrimRight(cfg.GetString("procfs_path"), "/")
	}

	return &NetworkCheck{
		CheckBase: core.NewCheckBase(CheckName),
		net: defaultNetworkStats{
			procPath:          procfsPath,
			tlmConnectionDiff: telemetry.NewGauge("net", "connections_diff", []string{"protocol", "state"}, "Gauge the difference of connection counts by state and protocol from non-shell command"),
			tlmRecvqDiff:      telemetry.NewGauge("net", "recv_q_diff", []string{"protocol", "state"}, "Gauge the difference of connection recvq counts by state and protocol from non-shell command"),
			tlmSendqDiff:      telemetry.NewGauge("net", "send_q_diff", []string{"protocol", "state"}, "Gauge the difference of connection sendq counts by state and protocol from non-shell command"),
			tlmConntrackDiff:  telemetry.NewGauge("net", "conntrack_diff", []string{"cpu", "field"}, "Guage the difference of conntrack stats from procfile instead of shell command"),
		},
		config: networkConfig{
			instance: networkInstanceConfig{
				CollectRateMetrics:        true,
				ConntrackPath:             "",
				WhitelistConntrackMetrics: []string{"max", "count"},
				UseSudoConntrack:          true,
			},
		},
	}
}
