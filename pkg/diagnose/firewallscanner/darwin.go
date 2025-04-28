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
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

const (
	blockerDiagnosisNameDarwin = "Firewall blockers on Darwin"
)

type darwinFirewallScanner struct{}

func (scanner *darwinFirewallScanner) DiagnoseBlockedPorts(forProtocol string, destPorts integrationsByDestPort, log log.Component) []diagnose.Diagnosis {
	if os.Geteuid() != 0 {
		log.Warn("Cannot check firewall rules without admin/root access")
		return []diagnose.Diagnosis{}
	}

	cmd := exec.Command("pfctl", "-sr")
	output, err := cmd.Output()
	if err != nil {
		log.Warnf("Error executing command %s: %v", cmd.String(), err)
		return []diagnose.Diagnosis{}
	}

	blockedPorts := checkBlockedPortsDarwin(output, forProtocol, destPorts)

	return []diagnose.Diagnosis{
		buildBlockedPortsDiagnosis(blockerDiagnosisNameDarwin, forProtocol, blockedPorts),
	}
}

func checkBlockedPortsDarwin(output []byte, forProtocol string, destPorts integrationsByDestPort) []blockedPort {
	var blockedPorts []blockedPort

	rules := strings.Split(string(output), "\n")
	re := regexp.MustCompile(`(?i)\bblock\b.*\bproto (\S+)\b.*\bto\b.*\bport = (\d+)\b`)
	for _, rule := range rules {
		rule = strings.ToLower(rule)
		if strings.Contains(rule, " out ") {
			continue
		}

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
