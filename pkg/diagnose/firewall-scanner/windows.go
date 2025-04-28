// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package firewall_scanner

import (
	"encoding/json"
	"os/exec"
	"strings"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

const (
	blockerDiagnosisNameWindows = "Firewall blockers on Windows"
)

type windowsFirewallScanner struct{}

type windowsRule struct {
	Direction ruleDirection `json:"direction"`
	Protocol  string        `json:"protocol"`
	LocalPort string        `json:"localPort"`
}

type ruleDirection int

const (
	Inbound ruleDirection = 1
)

func (scanner *windowsFirewallScanner) DiagnoseBlockedPorts(forProtocol string, destPorts integrationsByDestPort, log log.Component) []diagnose.Diagnosis {
	cmd := exec.Command(
		"powershell",
		"-Command",
		`Get-NetFirewallRule -Action Block | ForEach-Object { $rule = $_; Get-NetFirewallPortFilter -AssociatedNetFirewallRule $rule | Select-Object @{Name="direction"; Expression={$rule.Direction}}, @{Name="protocol"; Expression={$_.Protocol}}, @{Name="localPort"; Expression={$_.LocalPort}} } | ConvertTo-Json`)
	output, err := cmd.Output()
	if err != nil {
		log.Warnf("Error executing command %s: %v", cmd.String(), err)
		return []diagnose.Diagnosis{}
	}

	blockedPorts, err := checkBlockedPortsWindows(output, forProtocol, destPorts)
	if err != nil {
		log.Warnf("Error checking blocked ports: %v", err)
		return []diagnose.Diagnosis{}
	}

	return []diagnose.Diagnosis{
		buildBlockedPortsDiagnosis(blockerDiagnosisNameWindows, forProtocol, blockedPorts),
	}
}

func checkBlockedPortsWindows(output []byte, forProtocol string, destPorts integrationsByDestPort) ([]blockedPort, error) {
	var blockedPorts []blockedPort

	var rules []windowsRule
	err := json.Unmarshal(output, &rules)
	if err != nil {
		return nil, err
	}

	for _, rule := range rules {
		forIntegrations, portExists := destPorts[rule.LocalPort]
		if rule.Direction == Inbound && strings.EqualFold(rule.Protocol, forProtocol) && portExists {
			blockedPorts = append(blockedPorts, blockedPort{
				Port:            rule.LocalPort,
				ForIntegrations: forIntegrations,
			})
		}
	}

	return blockedPorts, nil
}
