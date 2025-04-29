// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package firewallscanner

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
	inbound ruleDirection = 1
)

func (scanner *windowsFirewallScanner) DiagnoseBlockingRules(forProtocol string, destPorts integrationsByDestPort, log log.Component) []diagnose.Diagnosis {
	cmd := exec.Command(
		"powershell",
		"-Command",
		`Get-NetFirewallRule -Action Block | ForEach-Object { $rule = $_; Get-NetFirewallPortFilter -AssociatedNetFirewallRule $rule | Select-Object @{Name="direction"; Expression={$rule.Direction}}, @{Name="protocol"; Expression={$_.Protocol}}, @{Name="localPort"; Expression={$_.LocalPort}} } | ConvertTo-Json`)
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputString := string(output)
		if strings.Contains(outputString, "No MSFT_NetFirewallRule objects found with property 'Action' equal to 'Block'") {
			// Windows returns an error when the firewall has no blocking rules
			// In this case, we return a successful diagnosis with no blocked ports
			return []diagnose.Diagnosis{
				buildBlockingRulesDiagnosis(blockerDiagnosisNameWindows, nil),
			}
		}

		log.Warnf("Error executing command %s: %v (%s)", cmd.String(), err, outputString)
		return []diagnose.Diagnosis{}
	}

	blockingRules, err := checkBlockingRulesWindows(output, forProtocol, destPorts)
	if err != nil {
		log.Warnf("Error checking blocked ports: %v", err)
		return []diagnose.Diagnosis{}
	}

	return []diagnose.Diagnosis{
		buildBlockingRulesDiagnosis(blockerDiagnosisNameWindows, blockingRules),
	}
}

func checkBlockingRulesWindows(output []byte, forProtocol string, destPorts integrationsByDestPort) ([]blockingRule, error) {
	if len(output) == 0 {
		return nil, nil
	}

	var rules []windowsRule
	err := json.Unmarshal(output, &rules)
	if err != nil {
		// Windows returns a single JSON object instead of an array when there is only one blocking rule
		var rule windowsRule
		err = json.Unmarshal(output, &rule)
		if err != nil {
			return nil, err
		}
		rules = []windowsRule{rule}
	}

	var blockingRules []blockingRule

	for _, rule := range rules {
		forIntegrations, portExists := destPorts[rule.LocalPort]
		if rule.Direction == inbound && strings.EqualFold(rule.Protocol, forProtocol) && portExists {
			blockingRules = append(blockingRules, blockingRule{
				Protocol:        rule.Protocol,
				Port:            rule.LocalPort,
				ForIntegrations: forIntegrations,
			})
		}
	}

	return blockingRules, nil
}
