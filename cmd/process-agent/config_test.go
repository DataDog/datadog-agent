package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/config"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
)

func TestProcessDiscovery(t *testing.T) {
	scfg, ocfg := &sysconfig.Config{}, &oconfig.OrchestratorConfig{}

	cfg := config.Mock()

	// Make sure the process_discovery check can be enabled
	cfg.Set("process_config.process_discovery.enabled", true)
	enabledChecks := getChecks(scfg, ocfg, false)
	assert.Contains(t, enabledChecks, checks.ProcessDiscovery)

	// Make sure the process_discovery check can be disabled
	cfg.Set("process_config.process_discovery.enabled", false)
	enabledChecks = getChecks(scfg, ocfg, true)
	assert.NotContains(t, enabledChecks, checks.ProcessDiscovery)

	// Make sure the process and process_discovery checks are mutually exclusive
	cfg.Set("process_config.process_discovery.enabled", true)
	cfg.Set("process_config.process_collection.enabled", true)
	enabledChecks = getChecks(scfg, ocfg, true)
	assert.NotContains(t, enabledChecks, checks.ProcessDiscovery)
}

func TestContainerCheck(t *testing.T) {
	scfg, ocfg := &sysconfig.Config{}, &oconfig.OrchestratorConfig{}

	cfg := config.Mock()

	// Make sure the container check can be enabled if the process check is disabled
	cfg.Set("process_config.process_collection.enabled", false)
	cfg.Set("process_config.container_collection.enabled", true)
	enabledChecks := getChecks(scfg, ocfg, true)
	assert.Contains(t, enabledChecks, checks.Container)
	assert.Contains(t, enabledChecks, checks.RTContainer)
	assert.NotContains(t, enabledChecks, checks.Process)

	// Make sure that disabling RT disables the rt container check
	cfg.Set("process_config.disable_realtime_checks", true)
	enabledChecks = getChecks(scfg, ocfg, true)
	assert.Contains(t, enabledChecks, checks.Container)
	assert.NotContains(t, enabledChecks, checks.RTContainer)

	// Make sure the container check cannot be enabled if we cannot access containers
	enabledChecks = getChecks(scfg, ocfg, false)
	assert.NotContains(t, enabledChecks, checks.Container)

	// Make sure the container and process check are mutually exclusive
	cfg.Set("process_config.process_collection.enabled", true)
	cfg.Set("process_config.container_collection.enabled", true)
	enabledChecks = getChecks(scfg, ocfg, true)
	assert.Contains(t, enabledChecks, checks.Process)
	assert.NotContains(t, enabledChecks, checks.Container)
	assert.NotContains(t, enabledChecks, checks.RTContainer)

}

func TestProcessCheck(t *testing.T) {
	cfg := config.Mock()
	cfg.Set("system_probe_config.enabled", true)
	cfg.Set("system_probe_config.process_config.enabled", true)

	scfg, err := sysconfig.New("")
	assert.NoError(t, err)

	ocfg := &oconfig.OrchestratorConfig{}

	// Make sure the process check can be enabled
	cfg.Set("process_config.process_collection.enabled", true)
	enabledChecks := getChecks(scfg, ocfg, true)
	assert.Contains(t, enabledChecks, checks.Process)

	// Make sure that the process check recognizes the system probe is available
	assert.True(t, checks.Process.SysprobeProcessModuleEnabled)
}

func TestConnectionsCheck(t *testing.T) {
	cfg := config.Mock()
	cfg.Set("system_probe_config.enabled", true)
	cfg.Set("network_config.enabled", true)

	scfg, err := sysconfig.New("")
	assert.NoError(t, err)

	ocfg := &oconfig.OrchestratorConfig{}

	enabledChecks := getChecks(scfg, ocfg, true)
	assert.Contains(t, enabledChecks, checks.Connections)
}

func TestPodCheck(t *testing.T) {
	cfg := config.Mock()
	cfg.Set("orchestrator_explorer.enabled", true)

	ocfg := oconfig.NewDefaultOrchestratorConfig()
	ocfg.KubeClusterName = "test" // We can't reliably detect a kubernetes cluster in a test
	assert.NoError(t, ocfg.Load())

	enabledChecks := getChecks(&sysconfig.Config{}, ocfg, true)
	assert.Contains(t, enabledChecks, checks.Pod)
}
