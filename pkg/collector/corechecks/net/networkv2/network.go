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
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/safchain/ethtool"
)

const (
	// CheckName is the name of the check
	CheckName = "network"
)

var (
	filesystem = afero.NewOsFs()

	ethtoolObject     = ethtool.Ethtool{}
	getEthtoolDrvInfo = ethtoolObject.DriverInfo
	getEthtoolStats   = ethtoolObject.Stats

	runCommandFunction  = runCommand
	ssAvailableFunction = checkSSExecutable
)

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
	CollectEthtoolStats       bool     `yaml:"collect_ethtool_stats"`
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
}

type defaultNetworkStats struct {
	procPath string
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

	for _, interfaceIO := range ioByInterface {
		if !c.isDeviceExcluded(interfaceIO.Name) {
			submitInterfaceMetrics(sender, interfaceIO)
			if c.config.instance.CollectEthtoolStats {
				err = handleEthtoolStats(sender, interfaceIO)
				if err != nil {
					return err
				}
			}
		}
	}

	if c.config.instance.CollectConnectionState {
		netProcfsBasePath := c.net.GetNetProcBasePath()
		for _, protocol := range []string{"udp4", "udp6", "tcp4", "tcp6"} {
			submitConnectionStateMetrics(sender, protocol, c.config.instance.CollectConnectionQueues, netProcfsBasePath)
		}
	}

	setProcPath := c.net.GetProcPath()
	collectConntrackMetrics(sender, c.config.instance.ConntrackPath, c.config.instance.UseSudoConntrack, setProcPath, c.config.instance.BlacklistConntrackMetrics, c.config.instance.WhitelistConntrackMetrics)

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

func handleEthtoolStats(sender sender.Sender, interfaceIO net.IOCountersStat) error {
	ethtoolObjectPtr, err := ethtool.NewEthtool()
	if err != nil {
		log.Errorf("Failed to create ethtool object: %s", err)
		return err
	}
	ethtoolObject = *ethtoolObjectPtr

	// Preparing the interface name and copy it into the request
	ifaceBytes := []byte(interfaceIO.Name)
	if len(ifaceBytes) > 15 {
		ifaceBytes = ifaceBytes[:15]
	}

	// Fetch driver information (ETHTOOL_GDRVINFO)
	drvInfo, err := getEthtoolDrvInfo(string(ifaceBytes))
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
	statsMap, err := getEthtoolStats(string(ifaceBytes))
	if err != nil {
		if err == unix.ENOTTY || err == unix.EOPNOTSUPP {
			log.Debugf("ethtool stats are not supported for interface: %s", interfaceIO.Name)
		} else {
			return errors.New("failed to get ethtool stats information for interface " + interfaceIO.Name + ": " + fmt.Sprintf("%d", err))
		}
	}

	processedMap := getEthtoolMetrics(driverName, statsMap)
	for extraTag, keyValuePairing := range processedMap {
		tags := []string{
			"interface:" + interfaceIO.Name,
			"driver_name:" + driverName,
			"driver_version:" + driverVersion,
			extraTag,
		}

		for metricName, metricValue := range keyValuePairing {
			metricName := fmt.Sprintf("system.net.%s", metricName)
			sender.MonotonicCount(metricName, float64(metricValue), "", tags)
		}
	}

	return nil
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
		continueCase := false
		queueTag := ""
		newKey := ""
		metricPrefix := ""
		if strings.Contains(statName, "queue_") {
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
			if queueIndex == -1 {
				continueCase = true
			}
			queueNum := parts[queueIndex+1]
			parts = append(parts[:queueIndex], parts[queueIndex+2:]...)
			queueTag = "queue:" + queueNum
			newKey = strings.Join(parts, "_")
			metricPrefix = ".queue."
		} else {
			continueCase = true
		}
		if continueCase {
			if strings.HasPrefix(statName, "cpu") {
				parts := strings.Split(statName, "_")
				if len(parts) < 2 {
					continueCase = true
				}
				cpuNum := parts[0][3:]
				if _, err := strconv.Atoi(cpuNum); err != nil {
					continueCase = true
				}
				queueTag = "cpu:" + cpuNum
				newKey = strings.Join(parts[1:], "_")
				metricPrefix = ".cpu."
			} else {
				continueCase = true
			}
		}
		if continueCase {
			if strings.Contains(statName, "[") && strings.HasSuffix(statName, "]") {
				parts := strings.SplitN(statName, "[", 2)
				if len(parts) != 2 {
					continueCase = true
				}
				metricName := parts[0]
				queueNum := strings.TrimSuffix(parts[1], "]")
				if _, err := strconv.Atoi(queueNum); err != nil {
					continueCase = true
				}
				queueTag = "queue:" + queueNum
				newKey = metricName
				metricPrefix = ".queue."
			} else {
				continueCase = true
			}
		}
		if continueCase {
			if statName != "" {
				if slices.Contains(ethtoolGlobalMetrics, statName) {
					queueTag = "global"
					newKey = statName
					metricPrefix = "."
				}
			}
		}
		if newKey != "" && queueTag != "" && metricPrefix != "" {
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

	for suffix, metrics := range results {
		sender.Gauge(fmt.Sprintf("system.net.%s.%s", protocolName, suffix), float64(metrics.count), "", nil)
		if collectConnectionQueues && protocolName[:3] == "tcp" {
			for _, point := range metrics.recvQ {
				sender.Histogram("system.net.tcp.recv_q", float64(point), "", []string{"state:" + suffix})
			}
			for _, point := range metrics.sendQ {
				sender.Histogram("system.net.tcp.send_q", float64(point), "", []string{"state:" + suffix})
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

func addConntrackStatsMetrics(sender sender.Sender, conntrackPath string, useSudoConntrack bool) {
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
	for _, line := range lines {
		if line == "" {
			continue
		}
		cols := strings.Fields(line)
		cpuNum := strings.Split(cols[0], "=")[1]
		cpuTag := []string{"cpu:" + cpuNum}
		cols = cols[1:]

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
			sender.MonotonicCount("system.net.conntrack."+metric, valueFloat, "", cpuTag)
		}
	}
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

func collectConntrackMetrics(sender sender.Sender, conntrackPath string, useSudo bool, procfsPath string, blacklistConntrackMetrics []string, whitelistConntrackMetrics []string) {
	addConntrackStatsMetrics(sender, conntrackPath, useSudo)

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
		net:       defaultNetworkStats{procPath: procfsPath},
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
