// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package embedded

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSystemdUnits(t *testing.T) {
	assert.NotNil(t, Units.DatadogAgentService)
	assert.NotNil(t, Units.DatadogAgentExpService)
	assert.NotNil(t, Units.DatadogAgentInstallerService)
	assert.NotNil(t, Units.DatadogAgentInstallerExpService)
	assert.NotNil(t, Units.DatadogAgentTraceService)
	assert.NotNil(t, Units.DatadogAgentTraceExpService)
	assert.NotNil(t, Units.DatadogAgentProcessService)
	assert.NotNil(t, Units.DatadogAgentProcessExpService)
	assert.NotNil(t, Units.DatadogAgentSecurityService)
	assert.NotNil(t, Units.DatadogAgentSecurityExpService)
	assert.NotNil(t, Units.DatadogAgentSysprobeService)
	assert.NotNil(t, Units.DatadogAgentSysprobeExpService)

	os.WriteFile("/Users/arthur.bellal/go/src/github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded/datadog-agent.service", []byte(Units.DatadogAgentService), 0644)
	os.WriteFile("/Users/arthur.bellal/go/src/github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded/datadog-agent-exp.service", []byte(Units.DatadogAgentExpService), 0644)
	os.WriteFile("/Users/arthur.bellal/go/src/github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded/datadog-agent-installer.service", []byte(Units.DatadogAgentInstallerService), 0644)
	os.WriteFile("/Users/arthur.bellal/go/src/github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded/datadog-agent-installer-exp.service", []byte(Units.DatadogAgentInstallerExpService), 0644)
	os.WriteFile("/Users/arthur.bellal/go/src/github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded/datadog-agent-trace.service", []byte(Units.DatadogAgentTraceService), 0644)
	os.WriteFile("/Users/arthur.bellal/go/src/github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded/datadog-agent-trace-exp.service", []byte(Units.DatadogAgentTraceExpService), 0644)
	os.WriteFile("/Users/arthur.bellal/go/src/github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded/datadog-agent-process.service", []byte(Units.DatadogAgentProcessService), 0644)
	os.WriteFile("/Users/arthur.bellal/go/src/github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded/datadog-agent-process-exp.service", []byte(Units.DatadogAgentProcessExpService), 0644)
	os.WriteFile("/Users/arthur.bellal/go/src/github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded/datadog-agent-security.service", []byte(Units.DatadogAgentSecurityService), 0644)
	os.WriteFile("/Users/arthur.bellal/go/src/github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded/datadog-agent-security-exp.service", []byte(Units.DatadogAgentSecurityExpService), 0644)
	os.WriteFile("/Users/arthur.bellal/go/src/github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded/datadog-agent-sysprobe.service", []byte(Units.DatadogAgentSysprobeService), 0644)
	os.WriteFile("/Users/arthur.bellal/go/src/github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded/datadog-agent-sysprobe-exp.service", []byte(Units.DatadogAgentSysprobeExpService), 0644)
}
