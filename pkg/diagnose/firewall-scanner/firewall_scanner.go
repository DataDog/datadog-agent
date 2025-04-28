// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package firewall_scanner

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	snmpTrapsConfig "github.com/DataDog/datadog-agent/comp/snmptraps/config"
)

type integrationsByDestPort map[string][]string

type blockedPort struct {
	Port            string
	ForIntegrations []string
}

type firewallScanner interface {
	DiagnoseBlockedPorts(forProtocol string, destPorts integrationsByDestPort, log log.Component) []diagnose.Diagnosis
}

// DiagnoseBlockers checks for firewall rules that may block SNMP traps and NetFlow packets.
func DiagnoseBlockers(log log.Component) []diagnose.Diagnosis {
	destPorts := getDestPorts()
	if len(destPorts) == 0 {
		return []diagnose.Diagnosis{}
	}

	scanner, err := getFirewallScanner()
	if err != nil {
		log.Warnf("Error diagnosing firewall: %v", err)
		return []diagnose.Diagnosis{}
	}

	return scanner.DiagnoseBlockedPorts("UDP", destPorts, log)
}

func getDestPorts() integrationsByDestPort {
	type DestPort struct {
		Port            uint16
		FromIntegration string
	}

	var allDestPorts []DestPort
	allDestPorts = append(allDestPorts, DestPort{Port: snmpTrapsConfig.GetLastReadPort(), FromIntegration: "snmp_traps"})
	allDestPorts = append(allDestPorts, DestPort{Port: snmpTrapsConfig.GetLastReadPort(), FromIntegration: "netflow"})

	destPorts := make(integrationsByDestPort)
	for _, destPort := range allDestPorts {
		if destPort.Port != 0 {
			portString := strconv.Itoa(int(destPort.Port))
			destPorts[portString] = append(destPorts[portString], destPort.FromIntegration)
		}
	}
	return destPorts
}

func getFirewallScanner() (firewallScanner, error) {
	var scanner firewallScanner
	switch runtime.GOOS {
	case "windows":
		scanner = &windowsFirewallScanner{}
	case "linux":
		scanner = &linuxFirewallScanner{}
	case "darwin":
		scanner = &darwinFirewallScanner{}
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
	return scanner, nil
}

func buildBlockedPortsDiagnosis(name string, forProtocol string, blockedPorts []blockedPort) diagnose.Diagnosis {
	if len(blockedPorts) == 0 {
		return diagnose.Diagnosis{
			Status:    diagnose.DiagnosisSuccess,
			Name:      name,
			Diagnosis: "No blocking firewall rules were found",
		}
	}

	var msgBuilder strings.Builder
	msgBuilder.WriteString("Blocking firewall rules were found:\n")
	for _, blockedPort := range blockedPorts {
		msgBuilder.WriteString(
			fmt.Sprintf("%s packets might be blocked because destination port %s is blocked for protocol %s\n",
				strings.Join(blockedPort.ForIntegrations, ", "), blockedPort.Port, forProtocol))
	}
	return diagnose.Diagnosis{
		Status:    diagnose.DiagnosisWarning,
		Name:      name,
		Diagnosis: msgBuilder.String(),
	}
}
