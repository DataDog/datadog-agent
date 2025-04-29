// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package firewallscanner

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	netflowConfig "github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
)

type ruleToCheck struct {
	Protocol string
	DestPort string
	Source   string
}

type rulesToCheckForPort struct {
	ProtocolsSet map[string]struct{}
	Sources      []string
}

type rulesToCheckByPort map[string]rulesToCheckForPort

type blockingRule struct {
	Protocol string
	DestPort string
	Sources  []string
}

type firewallScanner interface {
	DiagnoseBlockingRules(rulesToCheck rulesToCheckByPort, log log.Component) []diagnose.Diagnosis
}

// DiagnoseBlockers checks for firewall rules that may block SNMP traps and Netflow packets.
func DiagnoseBlockers(config config.Component, log log.Component) []diagnose.Diagnosis {
	rulesToCheck := getRulesToCheck(config)
	if len(rulesToCheck) == 0 {
		return []diagnose.Diagnosis{}
	}

	scanner, err := getFirewallScanner()
	if err != nil {
		return []diagnose.Diagnosis{}
	}

	return scanner.DiagnoseBlockingRules(rulesToCheck, log)
}

func getRulesToCheck(config config.Component) rulesToCheckByPort {
	var rulesToCheck []ruleToCheck
	rulesToCheck = append(rulesToCheck, getSNMPTrapsRulesToCheck(config)...)
	rulesToCheck = append(rulesToCheck, getNetflowRulesToCheck(config)...)

	rules := make(rulesToCheckByPort)
	for _, rule := range rulesToCheck {
		entry, exists := rules[rule.DestPort]

		if !exists {
			entry = rulesToCheckForPort{
				ProtocolsSet: make(map[string]struct{}),
				Sources:      []string{},
			}
		}

		entry.ProtocolsSet[rule.Protocol] = struct{}{}
		entry.Sources = append(entry.Sources, rule.Source)

		rules[rule.DestPort] = entry
	}
	return rules
}

func getSNMPTrapsRulesToCheck(config config.Component) []ruleToCheck {
	if !config.GetBool("network_devices.snmp_traps.enabled") {
		return []ruleToCheck{}
	}

	return []ruleToCheck{
		{
			Protocol: "UDP",
			DestPort: config.GetString("network_devices.snmp_traps.port"),
			Source:   "snmp_traps",
		},
	}
}

func getNetflowRulesToCheck(config config.Component) []ruleToCheck {
	if !config.GetBool("network_devices.netflow.enabled") {
		return []ruleToCheck{}
	}

	var listeners []netflowConfig.ListenerConfig
	err := structure.UnmarshalKey(config, "network_devices.netflow.listeners", &listeners)
	if err != nil {
		return []ruleToCheck{}
	}

	var rulesToCheck []ruleToCheck
	for _, listener := range listeners {
		flowTypeDetail, err := common.GetFlowTypeByName(listener.FlowType)
		if err != nil {
			continue
		}

		if listener.Port == 0 {
			rulesToCheck = append(rulesToCheck, ruleToCheck{
				Protocol: "UDP",
				DestPort: fmt.Sprintf("%d", flowTypeDetail.DefaultPort()),
				Source:   fmt.Sprintf("netflow (%s)", flowTypeDetail.Name()),
			})
			continue
		}

		rulesToCheck = append(rulesToCheck, ruleToCheck{
			Protocol: "UDP",
			DestPort: fmt.Sprintf("%d", listener.Port),
			Source:   fmt.Sprintf("netflow (%s)", flowTypeDetail.Name()),
		})
	}
	return rulesToCheck
}

func getFirewallScanner() (firewallScanner, error) {
	var scanner firewallScanner
	switch runtime.GOOS {
	case "windows":
		scanner = &windowsFirewallScanner{}
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
	return scanner, nil
}

func buildBlockingRulesDiagnosis(name string, blockingRules []blockingRule) diagnose.Diagnosis {
	if len(blockingRules) == 0 {
		return diagnose.Diagnosis{
			Status:    diagnose.DiagnosisSuccess,
			Name:      name,
			Diagnosis: "No blocking firewall rules were found",
		}
	}

	var msgBuilder strings.Builder
	msgBuilder.WriteString("Blocking firewall rules were found:\n")
	for _, blockingRule := range blockingRules {
		msgBuilder.WriteString(
			fmt.Sprintf("%s packets might be blocked because destination port %s is blocked for protocol %s\n",
				strings.Join(blockingRule.Sources, ", "), blockingRule.DestPort, blockingRule.Protocol))
	}
	return diagnose.Diagnosis{
		Status:    diagnose.DiagnosisWarning,
		Name:      name,
		Diagnosis: msgBuilder.String(),
	}
}
