// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package firewallscanner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
)

const (
	diagnosisNameWindows = "Firewall scan on Windows"
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

func (scanner *windowsFirewallScanner) DiagnoseBlockingRules(rulesToCheck sourcesByRule) []diagnose.Diagnosis {
	cmd := exec.Command(
		"powershell",
		"-Command",
		`try {
            $rules = Get-NetFirewallRule -Action Block -ErrorAction Stop
            $results = foreach ($rule in $rules) {
                Get-NetFirewallPortFilter -AssociatedNetFirewallRule $rule | ForEach-Object {
                    [PSCustomObject]@{
                        direction = $rule.Direction
                        protocol  = $_.Protocol
                        localPort = $_.LocalPort
                    }
                }
            }
            $results | ConvertTo-Json -Compress
        } catch {
            exit 1
        }`)

	output, err := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode := exitErr.ExitCode()
		if exitCode == 1 {
			// No blocking rules found
			return []diagnose.Diagnosis{
				buildBlockingRulesDiagnosis(diagnosisNameWindows, nil),
			}
		}

		return []diagnose.Diagnosis{
			{
				Status:    diagnose.DiagnosisUnexpectedError,
				Name:      diagnosisNameWindows,
				Diagnosis: fmt.Sprintf("PowerShell exited with code %d: %s", exitCode, string(output)),
			},
		}
	}

	blockingRules, err := checkBlockingRulesWindows(output, rulesToCheck)
	if err != nil {
		return []diagnose.Diagnosis{
			{
				Status:    diagnose.DiagnosisUnexpectedError,
				Name:      diagnosisNameWindows,
				Diagnosis: fmt.Sprintf("Error checking blocked ports: %v", err),
			},
		}
	}

	return []diagnose.Diagnosis{
		buildBlockingRulesDiagnosis(diagnosisNameWindows, blockingRules),
	}
}

func checkBlockingRulesWindows(output []byte, rulesToCheck sourcesByRule) (sourcesByRule, error) {
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

	blockingRules := make(sourcesByRule)

	for _, rule := range rules {
		if rule.Direction != inbound {
			continue
		}

		firewallRule := firewallRule{
			protocol: rule.Protocol,
			destPort: rule.LocalPort,
		}

		sources, exists := rulesToCheck[firewallRule]
		if exists {
			blockingRules[firewallRule] = sources
		}
	}

	return blockingRules, nil
}
