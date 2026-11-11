// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

// parScriptConfigCustom uses a script name that never appears in the packaged example.
const parScriptConfigCustom = `schemaId: script-credentials-v1
runPredefinedPowershellScript:
  e2eMsiUpgradePreserved:
    script: |
      Write-Output "preserved"
`

func parScriptConfigPath(s *baseAgentMSISuite, vm *components.RemoteHost) string {
	configRoot, err := windowsAgent.GetConfigRootFromRegistry(vm)
	s.Require().NoError(err)
	return filepath.Join(configRoot, "private-action-runner", "powershell-script-config.yaml")
}

// customizePARScriptConfig replaces the installed Private Action Runner script config
// with a customer-style edit, after checking the install created it from the template.
//
// Regression coverage for incident 61851: from 7.77.0 until 7.84.3 the MSI owned the live
// script config, so every upgrade replaced customer edits with the packaged example.
func (s *baseAgentMSISuite) customizePARScriptConfig(vm *components.RemoteHost) {
	s.Run("customize PAR script config", func() {
		path := parScriptConfigPath(s, vm)
		live, err := vm.ReadFile(path)
		s.Require().NoError(err, "install should create the live PAR script config")
		s.Require().NotEmpty(live)
		example, err := vm.ReadFile(path + ".example")
		s.Require().NoError(err, "install should ship the PAR script config template")
		s.Require().NotEmpty(example)

		_, err = vm.WriteFile(path, []byte(parScriptConfigCustom))
		s.Require().NoError(err)
	})
}

// assertPARScriptConfigPreserved checks the edit from customizePARScriptConfig survived.
func (s *baseAgentMSISuite) assertPARScriptConfigPreserved(vm *components.RemoteHost) {
	s.Run("PAR script config preserved", func() {
		path := parScriptConfigPath(s, vm)
		live, err := vm.ReadFile(path)
		s.Require().NoError(err)
		s.Assert().Equal(strings.TrimSpace(parScriptConfigCustom), strings.TrimSpace(string(live)),
			"PAR script config was modified by the upgrade")
		example, err := vm.ReadFile(path + ".example")
		s.Require().NoError(err, "upgraded package should still ship the PAR script config template")
		s.Assert().NotEmpty(example)
	})
}
