// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

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

type linuxFirewallScanner struct{}

func (scanner *linuxFirewallScanner) DiagnoseBlockedPorts(forProtocol string, destPorts integrationsByDestPort, log logger.Component) []diagnose.Diagnosis {
	if os.Geteuid() != 0 {
		log.Warn("Cannot check firewall rules without admin/root access")
		return []diagnose.Diagnosis{}
	}

	var blockedPorts []blockedPort

	checkers := []func(string, integrationsByDestPort, logger.Component) []blockedPort{
		checkBlockedPortsIPTables,
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

func checkBlockedPortsIPTables(forProtocol string, destPorts integrationsByDestPort, log logger.Component) []blockedPort {
	cmd := exec.Command("iptables", "-S")
	output, err := cmd.Output()
	if err != nil {
		log.Warnf("Error executing command %s: %v", cmd.String(), err)
		return []blockedPort{}
	}

	re := regexp.MustCompile(`(?i)-p (\S+)\b.*--dport (\d+)\b.*-j drop\b`)
	return checkBlockedPortsLinux(string(output), re, map[string]int{
		keyProtocolIndex: 1,
		keyDestPortIndex: 2,
	}, forProtocol, destPorts)
}

func checkBlockedPortsNFTables(forProtocol string, destPorts integrationsByDestPort, log logger.Component) []blockedPort {
	cmd := exec.Command("nft", "list", "ruleset")
	output, err := cmd.Output()
	if err != nil {
		log.Warnf("Error executing command %s: %v", cmd.String(), err)
		return []blockedPort{}
	}

	re := regexp.MustCompile(`(?i)\b(\S+)\b.*\bdport (\d+)\b.*\bdrop\b`)
	return checkBlockedPortsLinux(string(output), re, map[string]int{
		keyProtocolIndex: 1,
		keyDestPortIndex: 2,
	}, forProtocol, destPorts)
}

func checkBlockedPortsUFW(forProtocol string, destPorts integrationsByDestPort, log logger.Component) []blockedPort {
	cmd := exec.Command("ufw", "status")
	output, err := cmd.Output()
	if err != nil {
		log.Warnf("Error executing command %s: %v", cmd.String(), err)
		return []blockedPort{}
	}

	outputString := string(output)
	if strings.Contains(outputString, "Status: inactive") {
		return []blockedPort{}
	}

	re := regexp.MustCompile(`(?i)\b(\d+)/(\S+)\b.*\bdeny\b`)
	return checkBlockedPortsLinux(outputString, re, map[string]int{
		keyProtocolIndex: 2,
		keyDestPortIndex: 1,
	}, forProtocol, destPorts)
}

func checkBlockedPortsLinux(outputString string, re *regexp.Regexp, valIndexes map[string]int, forProtocol string, destPorts integrationsByDestPort) []blockedPort {
	var blockedPorts []blockedPort

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
			blockedPorts = append(blockedPorts, blockedPort{
				Port:            matchedDestPort,
				ForIntegrations: forIntegrations,
			})
		}
	}

	return blockedPorts
}
