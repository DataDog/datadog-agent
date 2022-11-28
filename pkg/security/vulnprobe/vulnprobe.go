package vulnprobe

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/DataDog/datadog-agent/pkg/security/vulnprobe/config"
	"github.com/DataDog/datadog-agent/pkg/security/vulnprobe/rules"
)

func Init() error {
	config.InitVulnProbeConfig()

	if config.VulnProbeConfig.Enabled {
		logrus.Infof("VulnProbe enabled")
	} else {
		logrus.Infof("VulnProbe disabled")
		return nil
	}

	policy, err := rules.LoadPolicyFromFile(config.VulnProbeConfig.PolicyPath)
	if err != nil {
		return fmt.Errorf("failed to load vulnprobe policy file %s: %w", config.VulnProbeConfig.PolicyPath, err)
	}

	fmt.Printf("Loaded rules:\nName: %s Source: %s Version: %v\n", policy.Name, policy.Source, policy.Version)
	for _, rule := range policy.Rules {
		fmt.Printf("%+v\n", rule)
	}

	return nil
}
