// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package firewallscanner

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
)

type linuxFirewallScanner struct{}

func (scanner *linuxFirewallScanner) DiagnoseBlockedPorts(forProtocol string, destPorts integrationsByDestPort, log logger.Component) []diagnose.Diagnosis {
	if os.Geteuid() != 0 {
		log.Warn("Cannot check firewall rules without admin/root access")
		return []diagnose.Diagnosis{}
	}

	type cmdWithCheck struct {
		cmd               *exec.Cmd
		checkBlockedPorts func([]byte, string, integrationsByDestPort) []blockedPort
	}

	cmdWithCheckList := []cmdWithCheck{
		{
			cmd:               exec.Command("iptables", "-S"),
			checkBlockedPorts: checkBlockedPortsIPTables,
		},
		{
			cmd:               exec.Command("nft", "list", "ruleset"),
			checkBlockedPorts: checkBlockedPortsNFTables,
		},
		{
			cmd:               exec.Command("ufw", "status"),
			checkBlockedPorts: checkBlockedPortsUFW,
		},
	}

	var blockedPorts []blockedPort

	for _, cmdWithCheck := range cmdWithCheckList {
		output, err := cmdWithCheck.cmd.Output()
		if err != nil {
			log.Warnf("Error executing command %s: %v", cmdWithCheck.cmd.String(), err)
			continue
		}

		blockedPorts = append(blockedPorts, cmdWithCheck.checkBlockedPorts(output, forProtocol, destPorts)...)
	}

	return []diagnose.Diagnosis{
		buildBlockedPortsDiagnosis(blockerDiagnosisNameLinux, forProtocol, blockedPorts),
	}
}

func checkBlockedPortsIPTables(output []byte, forProtocol string, destPorts integrationsByDestPort) []blockedPort {
	var blockedPorts []blockedPort

	rules := strings.Split(string(output), "\n")
	re := regexp.MustCompile(`(?i)-p (\S+)\b.*--dport (\d+)\b.*-j drop\b`)
	for _, rule := range rules {
		matches := re.FindStringSubmatch(rule)
		if matches == nil {
			continue
		}

		matchedProtocol := matches[1]
		matchedDestPort := matches[2]
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

func checkBlockedPortsNFTables(output []byte, forProtocol string, destPorts integrationsByDestPort) []blockedPort {
	var blockedPorts []blockedPort

	rules := strings.Split(string(output), "\n")
	re := regexp.MustCompile(`(?i)\b(\S+)\b.*\bdport (\d+)\b.*\bdrop\b`)
	for _, rule := range rules {
		matches := re.FindStringSubmatch(rule)
		if matches == nil {
			continue
		}

		matchedProtocol := matches[1]
		matchedDestPort := matches[2]
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

func checkBlockedPortsUFW(output []byte, forProtocol string, destPorts integrationsByDestPort) []blockedPort {
	outputString := string(output)
	if strings.Contains(outputString, "Status: inactive") {
		return []blockedPort{}
	}

	var blockedPorts []blockedPort

	rules := strings.Split(outputString, "\n")
	re := regexp.MustCompile(`(?i)\b(\d+)/(\S+)\b.*\bdeny\b`)
	for _, rule := range rules {
		matches := re.FindStringSubmatch(rule)
		if matches == nil {
			continue
		}

		matchedProtocol := matches[2]
		matchedDestPort := matches[1]
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
