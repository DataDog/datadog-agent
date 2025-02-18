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
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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
	"github.com/safchain/ethtool"
)

const (
	// CheckName is the name of the check
	CheckName = "network"
)

var (
	tcpStateMetricsSuffixMapping_ss = map[string]string{
		"ESTAB":      "established",
		"SYN-SENT":   "opening",
		"SYN-RECV":   "opening",
		"FIN-WAIT-1": "closing",
		"FIN-WAIT-2": "closing",
		"TIME-WAIT":  "time_wait",
		"UNCONN":     "closing",
		"CLOSE-WAIT": "closing",
		"LAST-ACK":   "closing",
		"LISTEN":     "listening",
		"CLOSING":    "closing",
	}

	tcpStateMetricsSuffixMapping_netstat = map[string]string{
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

	ethtoolObject = ethtool.Ethtool{}
	getDrvInfo    = ethtoolObject.DriverInfo
	getStats      = ethtoolObject.Stats

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

type networkInitConfig struct {
}

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

func (n defaultNetworkStats) NetstatAndSnmpCounters(protocols []string) (map[string]net.ProtoCountersStat, error) {
	return netstatAndSnmpCounters(n.procPath, protocols)
}

func (n defaultNetworkStats) GetProcPath() string {
	return n.procPath
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
				submitProtocolMetrics(sender, counters[protocol])
			}
		}
	}

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
		ssAvailable := false
		if ssAvailableFunction() == nil {
			ssAvailable = true
		}
		connectionsStats, err := c.net.Connections("udp4")
		if err != nil {
			return err
		}
		submitConnectionsMetrics(sender, "udp4", udpStateMetricsSuffixMapping, connectionsStats, c.config.instance.CollectConnectionQueues, ssAvailable)

		connectionsStats, err = c.net.Connections("udp6")
		if err != nil {
			return err
		}
		submitConnectionsMetrics(sender, "udp6", udpStateMetricsSuffixMapping, connectionsStats, c.config.instance.CollectConnectionQueues, ssAvailable)

		connectionsStats, err = c.net.Connections("tcp4")
		if err != nil {
			return err
		}
		if ssAvailable {
			submitConnectionsMetrics(sender, "tcp4", tcpStateMetricsSuffixMapping_ss, connectionsStats, c.config.instance.CollectConnectionQueues, ssAvailable)
		} else {
			submitConnectionsMetrics(sender, "tcp4", tcpStateMetricsSuffixMapping_netstat, connectionsStats, c.config.instance.CollectConnectionQueues, ssAvailable)
		}

		connectionsStats, err = c.net.Connections("tcp6")
		if err != nil {
			return err
		}
		if ssAvailable {
			submitConnectionsMetrics(sender, "tcp6", tcpStateMetricsSuffixMapping_ss, connectionsStats, c.config.instance.CollectConnectionQueues, ssAvailable)
		} else {
			submitConnectionsMetrics(sender, "tcp6", tcpStateMetricsSuffixMapping_netstat, connectionsStats, c.config.instance.CollectConnectionQueues, ssAvailable)
		}
	}

	setProcPath := c.net.GetProcPath()
	collectConntrackMetrics(sender, c.config.instance.ConntrackPath, c.config.instance.UseSudoConntrack, setProcPath, c.config.instance.BlacklistConntrackMetrics, c.config.instance.WhitelistConntrackMetrics)

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
	drvInfo, err := getDrvInfo(string(ifaceBytes))
	if err != nil {
		if err == unix.ENOTTY || err == unix.EOPNOTSUPP {
			log.Debugf("driver info is not supported for interface: %s", interfaceIO.Name)
		} else {
			return errors.New("failed to get driver info for interface " + interfaceIO.Name + ": " + fmt.Sprintf("%d", err))
		}
	}

	driverName := string(bytes.Trim([]byte(drvInfo.Driver[:]), "\x00"))
	driverVersion := string(bytes.Trim([]byte(drvInfo.Version[:]), "\x00"))

	// Fetch ethtool stats values (ETHTOOL_GSTATS)
	statsMap, err := getStats(string(ifaceBytes))
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
	if _, ok := ETHTOOL_METRIC_NAMES[driverName]; !ok {
		return result
	}
	ethtoolGlobalMetrics := []string{}
	if _, ok := ETHTOOL_GLOBAL_METRIC_NAMES[driverName]; ok {
		ethtoolGlobalMetrics = ETHTOOL_GLOBAL_METRIC_NAMES[driverName]
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
				if contains(ethtoolGlobalMetrics, statName) {
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

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
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

func checkSSExecutable() error {
	_, err := exec.LookPath("ss")
	if err != nil {
		return errors.New("`ss` executable not found in system PATH")
	}
	return nil
}

func getQueueMetrics(ipVersion string) (map[string][]uint64, error) {
	cmd := fmt.Sprintf("ss --numeric --tcp --all --ipv%s", ipVersion)
	output, err := runCommand([]string{"sh", "-c", cmd})
	if err != nil {
		return nil, fmt.Errorf("error executing ss command: %v", err)
	}
	return parseQueueMetrics(output)
}

func getQueueMetricsNetstat(ipVersion string) (map[string][]uint64, error) {
	output, err := runCommand([]string{"sh", "-c", "netstat", "-n -u -t -a"})
	if err != nil {
		return nil, fmt.Errorf("error executing netstat command: %v", err)
	}
	return parseQueueMetricsNetstat(output)
}

func parseQueueMetrics(output string) (map[string][]uint64, error) {
	queueMetrics := make(map[string][]uint64)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 2 {
			val, ok := tcpStateMetricsSuffixMapping_ss[fields[0]]
			if ok {
				state := val
				recvQ := parseQueue(fields[1])
				sendQ := parseQueue(fields[2])
				queueMetrics[state] = append(queueMetrics[state], recvQ, sendQ)
			}
		}
	}
	return queueMetrics, nil
}

func parseQueueMetricsNetstat(output string) (map[string][]uint64, error) {
	queueMetrics := make(map[string][]uint64)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "tcp") {
			fields := strings.Fields(line)
			if len(fields) > 5 {
				val, ok := tcpStateMetricsSuffixMapping_netstat[fields[5]]
				if ok {
					state := val
					recvQ := parseQueue(fields[1])
					sendQ := parseQueue(fields[2])
					queueMetrics[state] = append(queueMetrics[state], recvQ, sendQ)
				}
			}
		}
	}

	return queueMetrics, nil
}

func parseQueue(queueStr string) uint64 {
	var queue uint64
	_, err := fmt.Sscanf(queueStr, "%d", &queue)
	if err != nil {
		return 0
	}
	return queue
}

func submitConnectionsMetrics(sender sender.Sender, protocolName string, stateMetricSuffixMapping map[string]string, connectionsStats []net.ConnectionStat, collectConnectionQueues bool, ssAvailable bool) {
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
	queueMetrics := make(map[string][]uint64)
	if collectConnectionQueues && protocolName[:3] == "tcp" {
		var queues map[string][]uint64
		var err error
		// pass in version number
		if ssAvailable {
			queues, err = getQueueMetrics(protocolName[len(protocolName)-1:])
			if err != nil {
				log.Debug("Error getting queue metrics with ss:", err)
				return
			}
		} else {
			queues, err = getQueueMetricsNetstat(protocolName[len(protocolName)-1:])
			if err != nil {
				log.Debug("Error getting queue metrics with netstat:", err)
				return
			}
		}
		for state, queues := range queues {
			queueMetrics[state] = append(queueMetrics[state], queues...)
		}
		for state, queues := range queueMetrics {
			for _, queue := range queues {
				sender.Histogram("system.net.tcp.recv_q", float64(queue), "", []string{"state:" + state})
				sender.Histogram("system.net.tcp.send_q", float64(queue), "", []string{"state:" + state})
			}
		}
	}
}

func netstatAndSnmpCounters(procfsPath string, protocolNames []string) (map[string]net.ProtoCountersStat, error) {
	counters := make(map[string]net.ProtoCountersStat)
	for _, protocol := range protocolNames {
		counters[protocol] = net.ProtoCountersStat{Protocol: protocol, Stats: make(map[string]int64)}
	}

	for _, subdirectory := range []string{"netstat", "snmp"} {
		fs := filesystem
		f, err := fs.Open(procfsPath + "/net/" + subdirectory)
		if err != nil {
			if subdirectory == "snmp" {
				continue
			}
			return nil, err
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		var protocolName string
		for scanner.Scan() {
			line := scanner.Text()
			i := strings.IndexRune(line, ':')
			if i == -1 {
				return nil, errors.New(procfsPath + "/net/" + subdirectory + " is not fomatted correctly, expected ':'")
			}
			if contains(protocolNames, line[:i]) {
				protocolName = line[:i]
			} else {
				continue
			}

			counterNames := strings.Split(line[i+2:], " ")

			if !scanner.Scan() {
				return nil, errors.New(procfsPath + "/net/" + subdirectory + " is not fomatted correctly, not data line")
			}
			line = scanner.Text()

			counterValues := strings.Split(line[i+2:], " ")
			if len(counterNames) != len(counterValues) {
				return nil, errors.New(procfsPath + "/net/" + subdirectory + " is not fomatted correctly, expected same number of columns")
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
	cmd := []string{conntrackPath, "-S"}
	if useSudoConntrack {
		cmd = append([]string{"sudo"}, cmd...)
	}

	output, err := runCommandFunction(cmd)
	if err != nil {
		log.Debugf("Couldn't use %s to get conntrack stats: %v", conntrackPath, err)
		return
	}

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

func runCommand(cmd []string) (string, error) {
	execCmd := exec.Command(cmd[0], cmd[1:]...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	execCmd.Stdout = &out
	execCmd.Stderr = &stderr
	err := execCmd.Run()
	if err != nil {
		return "", fmt.Errorf("error executing command: %v, stderr: %s", err, stderr.String())
	}
	return out.String(), nil
}

func collectConntrackMetrics(sender sender.Sender, conntrackPath string, useSudo bool, procfsPath string, blacklistConntrackMetrics []string, whitelistConntrackMetrics []string) {
	if conntrackPath != "None" {
		addConntrackStatsMetrics(sender, conntrackPath, useSudo)
		conntrackFilesLocation := procfsPath + "/sys/net/netfilter"
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
		for _, metricName := range availableFiles {
			if len(blacklistConntrackMetrics) > 0 {
				if contains(blacklistConntrackMetrics, metricName) {
					continue
				}
			} else if len(whitelistConntrackMetrics) > 0 {
				if !contains(whitelistConntrackMetrics, metricName) {
					continue
				}
			} else {
				if !contains([]string{"max", "count"}, metricName) {
					continue
				}
			}
			metricFileLocation := filepath.Join(conntrackFilesLocation, "nf_conntrack_"+metricName)
			value, err := readIntFile(metricFileLocation, fs)
			if err != nil {
				log.Debugf("Error reading %s: %v", metricFileLocation, err)
			}
			sender.Gauge("system.net.conntrack."+metricName, float64(value), "", []string{})
		}
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
