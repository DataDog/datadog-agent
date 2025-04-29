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

type integrationsByDestPort map[string][]string

type destPort struct {
	Port            string
	FromIntegration string
}

type blockingRule struct {
	Protocol        string
	Port            string
	ForIntegrations []string
}

type firewallScanner interface {
	DiagnoseBlockingRules(forProtocol string, destPorts integrationsByDestPort, log log.Component) []diagnose.Diagnosis
}

// DiagnoseBlockers checks for firewall rules that may block SNMP traps and NetFlow packets.
func DiagnoseBlockers(config config.Component, log log.Component) []diagnose.Diagnosis {
	destPorts := getDestPorts(config)
	if len(destPorts) == 0 {
		return []diagnose.Diagnosis{}
	}

	scanner, err := getFirewallScanner()
	if err != nil {
		return []diagnose.Diagnosis{}
	}

	return scanner.DiagnoseBlockingRules("UDP", destPorts, log)
}

func getDestPorts(config config.Component) integrationsByDestPort {
	var allDestPorts []destPort
	allDestPorts = append(allDestPorts, getSNMPTrapsDestPorts(config)...)
	allDestPorts = append(allDestPorts, getNetFlowDestPorts(config)...)

	destPorts := make(integrationsByDestPort)
	for _, destPort := range allDestPorts {
		destPorts[destPort.Port] = append(destPorts[destPort.Port], destPort.FromIntegration)
	}
	return destPorts
}

func getSNMPTrapsDestPorts(config config.Component) []destPort {
	if !config.GetBool("network_devices.snmp_traps.enabled") {
		return []destPort{}
	}

	return []destPort{
		{
			Port:            config.GetString("network_devices.snmp_traps.port"),
			FromIntegration: "snmp_traps",
		},
	}
}

func getNetFlowDestPorts(config config.Component) []destPort {
	if !config.GetBool("network_devices.netflow.enabled") {
		return []destPort{}
	}

	var listeners []netflowConfig.ListenerConfig
	err := structure.UnmarshalKey(config, "network_devices.netflow.listeners", &listeners)
	if err != nil {
		return []destPort{}
	}

	var destPorts []destPort

	for _, listener := range listeners {
		flowTypeDetail, err := common.GetFlowTypeByName(listener.FlowType)
		if err != nil {
			continue
		}

		if listener.Port == 0 {
			destPorts = append(destPorts, destPort{
				Port:            fmt.Sprintf("%d", flowTypeDetail.DefaultPort()),
				FromIntegration: fmt.Sprintf("netflow (%s)", flowTypeDetail.Name()),
			})
			continue
		}

		destPorts = append(destPorts, destPort{
			Port:            fmt.Sprintf("%d", listener.Port),
			FromIntegration: fmt.Sprintf("netflow (%s)", flowTypeDetail.Name()),
		})
	}

	return destPorts
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
				strings.Join(blockingRule.ForIntegrations, ", "), blockingRule.Port, blockingRule.Protocol))
	}
	return diagnose.Diagnosis{
		Status:    diagnose.DiagnosisWarning,
		Name:      name,
		Diagnosis: msgBuilder.String(),
	}
}
