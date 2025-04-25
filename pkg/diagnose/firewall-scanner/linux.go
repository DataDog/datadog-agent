package firewall_scanner

import (
	"os"
	"os/exec"
	"regexp"
	"strings"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	logger "github.com/DataDog/datadog-agent/comp/core/log/def"
)

const (
	blockerDiagnosisNameLinux = "Firewall blockers on Linux"

	keyProtocolIndex = "ProtocolIndex"
	keyDestPortIndex = "DestPortIndex"
)

type LinuxFirewallScanner struct{}

func (scanner *LinuxFirewallScanner) DiagnoseBlockedPorts(forProtocol string, destPorts IntegrationsByDestPort, log logger.Component) []diagnose.Diagnosis {
	if os.Geteuid() != 0 {
		log.Warn("Cannot check firewall rules without admin/root access")
		return []diagnose.Diagnosis{}
	}

	var blockedPorts []BlockedPort

	checkers := []func(string, IntegrationsByDestPort, logger.Component) []BlockedPort{
		checkBlockedPortsIpTables,
		checkBlockedPortsNFTables,
		checkBlockedPortsUFW,
	}

	for _, checker := range checkers {
		blockedPorts = append(blockedPorts, checker(forProtocol, destPorts, log)...)
	}

	return []diagnose.Diagnosis{
		buildBlockedPortsDiagnosis(blockerDiagnosisNameLinux, forProtocol, blockedPorts),
	}
}

func checkBlockedPortsIpTables(forProtocol string, destPorts IntegrationsByDestPort, log logger.Component) []BlockedPort {
	cmd := exec.Command("iptables", "-S")
	output, err := cmd.Output()
	if err != nil {
		log.Warnf("Error executing command %s: %v", cmd.String(), err)
		return []BlockedPort{}
	}

	re := regexp.MustCompile(`(?i)-p (\S+)\b.*--dport (\d+)\b.*-j drop\b`)
	return checkBlockedPortsLinux(string(output), re, map[string]int{
		keyProtocolIndex: 1,
		keyDestPortIndex: 2,
	}, forProtocol, destPorts)
}

func checkBlockedPortsNFTables(forProtocol string, destPorts IntegrationsByDestPort, log logger.Component) []BlockedPort {
	cmd := exec.Command("nft", "list", "ruleset")
	output, err := cmd.Output()
	if err != nil {
		log.Warnf("Error executing command %s: %v", cmd.String(), err)
		return []BlockedPort{}
	}

	re := regexp.MustCompile(`(?i)\b(\S+)\b.*\bdport (\d+)\b.*\bdrop\b`)
	return checkBlockedPortsLinux(string(output), re, map[string]int{
		keyProtocolIndex: 1,
		keyDestPortIndex: 2,
	}, forProtocol, destPorts)
}

func checkBlockedPortsUFW(forProtocol string, destPorts IntegrationsByDestPort, log logger.Component) []BlockedPort {
	cmd := exec.Command("ufw", "status")
	output, err := cmd.Output()
	if err != nil {
		log.Warnf("Error executing command %s: %v", cmd.String(), err)
		return []BlockedPort{}
	}

	outputString := string(output)
	if strings.Contains(outputString, "Status: inactive") {
		return []BlockedPort{}
	}

	re := regexp.MustCompile(`(?i)\b(\d+)/(\S+)\b.*\bdeny\b`)
	return checkBlockedPortsLinux(outputString, re, map[string]int{
		keyProtocolIndex: 2,
		keyDestPortIndex: 1,
	}, forProtocol, destPorts)
}

func checkBlockedPortsLinux(outputString string, re *regexp.Regexp, valIndexes map[string]int, forProtocol string, destPorts IntegrationsByDestPort) []BlockedPort {
	var blockedPorts []BlockedPort

	rules := strings.Split(outputString, "\n")
	for _, rule := range rules {
		matches := re.FindStringSubmatch(rule)
		if matches == nil {
			continue
		}

		matchedProtocol := matches[valIndexes[keyProtocolIndex]]
		matchedDestPort := matches[valIndexes[keyDestPortIndex]]
		forIntegrations, portExists := destPorts[matchedDestPort]
		if strings.EqualFold(matchedProtocol, forProtocol) && portExists {
			blockedPorts = append(blockedPorts, BlockedPort{
				Port:            matchedDestPort,
				ForIntegrations: forIntegrations,
			})
		}
	}

	return blockedPorts
}
