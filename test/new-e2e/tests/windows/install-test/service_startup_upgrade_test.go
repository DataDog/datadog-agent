// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

//go:embed fixtures/service-startup-checkpoints.ps1
var serviceStartupCheckpoints string

func TestUpgradeServiceProtection(t *testing.T) {
	Run(t, &testUpgradeServiceProtectionSuite{})
}

type testUpgradeServiceProtectionSuite struct {
	baseAgentMSISuite
}

func (s *testUpgradeServiceProtectionSuite) TestUpgradeServiceProtection() {
	vm := s.Env().RemoteHost
	upgrade, err := windowsAgent.GetUpgradeTestPackageFromEnv()
	s.Require().NoError(err)

	for _, scenario := range []struct{ name, oldFailure, newFailure string }{
		{"removal_rollback", "UninstallUser", ""},
		{"configuration_rollback", "", "ConfigureServices"},
		{"late_rollback", "", "StartDDServices"},
		{"success", "", ""},
	} {
		if !s.Run(scenario.name, func() {
			directory := strings.TrimSpace(vm.MustExecute(`[System.IO.Path]::Combine($env:TEMP, [guid]::NewGuid().ToString('N'))`))
			s.Require().NoError(vm.MkdirAll(directory))
			defer vm.RemoveAll(directory) //nolint:errcheck
			scriptPath := directory + `\checkpoints.ps1`
			_, err = vm.WriteFile(scriptPath, []byte(serviceStartupCheckpoints))
			s.Require().NoError(err)

			prepare := func(role string, pkg *windowsAgent.Package, failAfter string) string {
				path := directory + `\` + role + ".msi"
				s.Require().NoError(windowsCommon.PutOrDownloadFile(vm, pkg.URL, path))
				_, err := vm.Execute(fmt.Sprintf(`& '%s' -MsiPath '%s' -OutputDirectory '%s' -Role '%s' -FailAfter '%s'`, scriptPath, path, directory, role, failAfter))
				s.Require().NoError(err)
				return path
			}
			oldMSI := prepare("old", s.AgentPackage, scenario.oldFailure)
			newMSI := prepare("new", upgrade, scenario.newFailure)
			installArgs := `APIKEY="00000000000000000000000000000000" DD_INSTALL_ONLY=1`
			s.Require().NoError(windowsCommon.InstallMSI(vm, oldMSI, installArgs, filepath.Join(s.SessionOutputDir(), "install.log")))
			oldProduct, err := windowsAgent.GetDatadogAgentProductCode(vm)
			s.Require().NoError(err)
			// Nondefault settings distinguish snapshot restoration from package defaults.
			vm.MustExecute(`Set-Service datadogagent -StartupType Manual; Set-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Services\datadogagent' -Name DelayedAutoStart -Value 0; Set-Service 'Datadog Installer' -StartupType Disabled`)
			original := s.captureProtectedServiceStartup()
			s.Require().Len(original, 3)

			err = windowsCommon.InstallMSI(vm, newMSI, installArgs, filepath.Join(s.SessionOutputDir(), "upgrade.log"))
			failed := scenario.oldFailure != "" || scenario.newFailure != ""
			if failed {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}

			checkpoint := func(role, action string, enabled bool) {
				data, err := vm.ReadFile(directory + `\` + role + "-" + action + ".txt")
				s.Require().NoError(err, "execution must reach %s/%s", role, action)
				lines := strings.FieldsFunc(strings.TrimSpace(string(data)), func(r rune) bool { return r == '\r' || r == '\n' })
				s.Require().Len(lines, 3)
				for _, line := range lines {
					// ConfigureUser precedes InstallServices: removed services are also safe.
					if action == "ConfigureUser" && strings.HasSuffix(line, "=absent") {
						continue
					}
					start := "4"
					if enabled {
						start = "3"
						if strings.HasPrefix(line, "datadogagent=") {
							start = "2"
						}
					}
					s.Require().True(strings.HasSuffix(line, "="+start), "unexpected startup setting at %s/%s: %s", role, action, line)
				}
			}
			checkpoint("old", "StopDDServices", false)
			checkpoint("old", "UninstallUser", false)
			if scenario.oldFailure == "" {
				checkpoint("new", "ConfigureUser", false)
				checkpoint("new", "ConfigureServices", false)
				if scenario.newFailure != "ConfigureServices" {
					checkpoint("new", "StartDDServices", true)
				}
			}
			product, err := windowsAgent.GetDatadogAgentProductCode(vm)
			s.Require().NoError(err)
			if failed {
				s.Require().Equal(oldProduct, product, "rollback must restore the outgoing product")
				s.assertProtectedServiceStartup(original)
				// Startup restoration must leave a usable installation after permissions rollback.
				vm.MustExecute(`Set-Service 'Datadog Installer' -StartupType Manual`)
				s.Require().NoError(windowsCommon.StartService(vm, "datadogagent"))
				RequireAgentVersionRunningWithNoErrors(s.T(), s.NewTestClientForHost(vm), s.AgentPackage.AgentVersion())
			} else {
				s.Require().NotEqual(oldProduct, product, "must exercise a major upgrade, not repair")
			}
			s.Require().NoError(windowsCommon.UninstallMSI(vm, product, "", filepath.Join(s.SessionOutputDir(), "uninstall.log")))
		}) {
			s.Require().FailNow("stopping after failed upgrade scenario")
		}
	}
}
