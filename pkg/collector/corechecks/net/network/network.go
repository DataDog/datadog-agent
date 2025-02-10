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
	"os"
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

	ethtoolObject = ethtool.Ethtool{}
	getDrvInfo    = ethtoolObject.DriverInfo
	getStats      = ethtoolObject.Stats

	runCommandFunction = runCommand
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
	CollectEthtoolStats      bool `yaml:"collect_ethtool_stats"`
}

type networkInitConfig struct {
	ConntrackPath             string   `yaml:"conntrack_path"`
	UseSudoConntrack          bool     `yaml:"use_sudo_conntrack"`
	BlacklistConntrackMetrics []string `yaml:"blacklist_conntrack_metrics"`
	WhitelistConntrackMetrics []string `yaml:"whitelist_conntrack_metrics"`
}

type networkConfig struct {
	instance networkInstanceConfig
	initConf networkInitConfig
}

type networkStats interface {
	IOCounters(pernic bool) ([]net.IOCountersStat, error)
	ProtoCounters(protocols []string) ([]net.ProtoCountersStat, error)
	Connections(kind string) ([]net.ConnectionStat, error)
	NetstatTCPExtCounters() (map[string]int64, error)
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

func (n defaultNetworkStats) NetstatTCPExtCounters() (map[string]int64, error) {
	return netstatTCPExtCounters(n.procPath)
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

	setProcPath := c.net.GetProcPath()
	log.Debug(c.config)
	log.Debug(c.config.instance)
	log.Debug(c.config.initConf)
	collectConntrackMetrics(sender, c.config.initConf.ConntrackPath, c.config.initConf.UseSudoConntrack, setProcPath, c.config.initConf.BlacklistConntrackMetrics, c.config.initConf.WhitelistConntrackMetrics)

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
			sender.Rate(metricName, float64(metricValue), "", tags)
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

func readIntFile(filePath string) (int, error) {
	data, err := os.ReadFile(filePath)
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
	execCmd.Stdout = &out
	err := execCmd.Run()
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

func collectConntrackMetrics(sender sender.Sender, conntrackPath string, useSudo bool, procfsPath string, blacklistConntrackMetrics []string, whitelistConntrackMetrics []string) {
	if conntrackPath != "None" {
		addConntrackStatsMetrics(sender, conntrackPath, useSudo)
		conntrackFilesLocation := procfsPath + "/sys/net/netfilter"

		var availableFiles []string
		files, err := os.ReadDir(conntrackFilesLocation)
		if err != nil {
			log.Debugf("Unable to list files in %s: %v", conntrackFilesLocation, err)
		} else {
			for _, file := range files {
				if file.Type().IsRegular() && strings.HasPrefix(file.Name(), "nf_conntrack_") {
					availableFiles = append(availableFiles, strings.TrimPrefix(file.Name(), "nf_conntrack_"))
				}
			}
		}
		for _, metricName := range availableFiles {
			if len(blacklistConntrackMetrics) > 0 {
				if contains(blacklistConntrackMetrics, metricName) {
					continue
				}
			} else {
				if !contains(whitelistConntrackMetrics, metricName) {
					continue
				}
			}
			metricFileLocation := filepath.Join(conntrackFilesLocation, "nf_conntrack_"+metricName)
			value, err := readIntFile(metricFileLocation)
			if err != nil {
				log.Debugf("Error reading %s: %v", metricFileLocation, err)
			}
			sender.Rate("system.net.conntrack."+metricName, float64(value), "", []string{})
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
