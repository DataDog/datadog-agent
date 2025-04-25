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

type WindowsFirewallScanner struct{}

type windowsRule struct {
	Direction RuleDirection `json:"direction"`
	Protocol  string        `json:"protocol"`
	LocalPort string        `json:"localPort"`
}

type RuleDirection int

const (
	Inbound RuleDirection = 1
)

func (scanner *WindowsFirewallScanner) DiagnoseBlockedPorts(forProtocol string, destPorts IntegrationsByDestPort, log log.Component) []diagnose.Diagnosis {
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

func checkBlockedPortsWindows(output []byte, forProtocol string, destPorts IntegrationsByDestPort) ([]BlockedPort, error) {
	var blockedPorts []BlockedPort

	var rules []windowsRule
	err := json.Unmarshal(output, &rules)
	if err != nil {
		return nil, err
	}

	for _, rule := range rules {
		forIntegrations, portExists := destPorts[rule.LocalPort]
		if rule.Direction == Inbound && strings.EqualFold(rule.Protocol, forProtocol) && portExists {
			blockedPorts = append(blockedPorts, BlockedPort{
				Port:            rule.LocalPort,
				ForIntegrations: forIntegrations,
			})
		}
	}

	return blockedPorts, nil
}
