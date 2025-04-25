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

type IntegrationsByDestPort map[string][]string

type BlockedPort struct {
	Port            string
	ForIntegrations []string
}

type FirewallScanner interface {
	DiagnoseBlockedPorts(forProtocol string, destPorts IntegrationsByDestPort, log log.Component) []diagnose.Diagnosis
}

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

func getDestPorts() IntegrationsByDestPort {
	type DestPort struct {
		Port            uint16
		FromIntegration string
	}

	var allDestPorts []DestPort
	allDestPorts = append(allDestPorts, DestPort{Port: snmpTrapsConfig.GetLastReadPort(), FromIntegration: "snmp_traps"})
	allDestPorts = append(allDestPorts, DestPort{Port: snmpTrapsConfig.GetLastReadPort(), FromIntegration: "netflow"})

	destPorts := make(IntegrationsByDestPort)
	for _, destPort := range allDestPorts {
		if destPort.Port != 0 {
			portString := strconv.Itoa(int(destPort.Port))
			destPorts[portString] = append(destPorts[portString], destPort.FromIntegration)
		}
	}
	return destPorts
}

func getFirewallScanner() (FirewallScanner, error) {
	var scanner FirewallScanner
	switch runtime.GOOS {
	case "windows":
		scanner = &WindowsFirewallScanner{}
	case "linux":
		scanner = &LinuxFirewallScanner{}
	case "darwin":
		scanner = &DarwinFirewallScanner{}
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
	return scanner, nil
}

func buildBlockedPortsDiagnosis(name string, forProtocol string, blockedPorts []BlockedPort) diagnose.Diagnosis {
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
