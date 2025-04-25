package firewall_scanner

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

type DarwinFirewallScanner struct{}

func (scanner *DarwinFirewallScanner) DiagnoseBlockedPorts(forProtocol string, destPorts IntegrationsByDestPort, log log.Component) []diagnose.Diagnosis {
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

	blockedPorts := checkBlockedPortsDarwin(string(output), forProtocol, destPorts)

	return []diagnose.Diagnosis{
		buildBlockedPortsDiagnosis(blockerDiagnosisNameDarwin, forProtocol, blockedPorts),
	}
}

func checkBlockedPortsDarwin(outputString string, forProtocol string, destPorts IntegrationsByDestPort) []BlockedPort {
	var blockedPorts []BlockedPort

	rules := strings.Split(outputString, "\n")
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
			blockedPorts = append(blockedPorts, BlockedPort{
				Port:            matchedDestPort,
				ForIntegrations: forIntegrations,
			})
		}
	}

	return blockedPorts
}
