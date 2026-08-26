// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package powershellcheck contains E2E tests for the Windows PowerShell core check.
package powershellcheck

import (
	_ "embed"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	perms "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams/filepermissions"
	scenwindows "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2/windows"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	checktest "github.com/DataDog/datadog-agent/test/e2e-framework/testing/testcommon/check"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const (
	allowlistPath        = `C:/ProgramData/Datadog/protected/powershell_allowlist.yaml`
	allowlistExamplePath = `C:/ProgramData/Datadog/protected/powershell_allowlist.yaml.example`
	metricName           = "e2e.powershell.service.running"

	administratorsSID = "S-1-5-32-544"
	usersSID          = "S-1-5-32-545"

	cmdletInjectionMarker    = `C:/Windows/Temp/dd-powershell-cmdlet-injection.txt`
	parameterInjectionMarker = `C:/Windows/Temp/dd-powershell-parameter-injection.txt`
	whereInjectionMarker     = `C:/Windows/Temp/dd-powershell-where-injection.txt`
)

var injectionMarkers = []string{
	cmdletInjectionMarker,
	parameterInjectionMarker,
	whereInjectionMarker,
}

//go:embed fixtures/allowlist.yaml
var allowlistConfig string

//go:embed fixtures/valid.yaml
var validCheckConfig string

//go:embed fixtures/cmdlet_injection.yaml
var cmdletInjectionConfig string

//go:embed fixtures/parameter_injection.yaml
var parameterInjectionConfig string

//go:embed fixtures/where_injection.yaml
var whereInjectionConfig string

var adminOwnedAllowlist = perms.NewWindowsPermissions(
	perms.WithIcaclsCommand(fmt.Sprintf(`/setowner "*%s"`, administratorsSID)),
)

type powershellCheckWindowsSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
}

func TestPowerShellCheckWindows(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &powershellCheckWindowsSuite{}, e2e.WithProvisioner(powerShellProvisioner("")))
}

func powerShellProvisioner(checkConfig string) provisioners.Provisioner {
	agentOptions := []agentparams.Option{
		agentparams.WithFileWithPermissions(allowlistPath, allowlistConfig, true, adminOwnedAllowlist),
	}
	if checkConfig != "" {
		agentOptions = append(agentOptions, agentparams.WithIntegration("powershell.d", checkConfig))
	}

	return winawshost.Provisioner(
		winawshost.WithRunOptions(
			scenwindows.WithAgentOptions(agentOptions...),
		),
	)
}

func (s *powershellCheckWindowsSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	s.removeInjectionMarkers()
	s.T().Cleanup(s.removeInjectionMarkers)
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
}

func (s *powershellCheckWindowsSuite) TestAllowlistExampleIsPackaged() {
	exists, err := s.Env().RemoteHost.FileExists(allowlistExamplePath)
	require.NoError(s.T(), err)
	assert.True(s.T(), exists, "%s was not installed by the Agent MSI", allowlistExamplePath)
}

func (s *powershellCheckWindowsSuite) TestRejectsNonAdminOwnedAllowlist() {
	s.UpdateEnv(powerShellProvisioner(validCheckConfig))
	s.setAllowlistOwner(usersSID)
	s.T().Cleanup(func() { s.setAllowlistOwner(administratorsSID) })
	assert.Equal(s.T(), usersSID, s.allowlistOwnerSID())

	_, err := s.Env().Agent.Client.CheckWithError(agentclient.WithArgs([]string{"powershell", "--json"}))
	require.Error(s.T(), err)
	assert.ErrorContains(s.T(), err, "no valid check found")
}

func (s *powershellCheckWindowsSuite) TestReportsServiceMetric() {
	s.UpdateEnv(powerShellProvisioner(validCheckConfig))
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	output, err := s.Env().Agent.Client.CheckWithError(agentclient.WithArgs([]string{"powershell", "--json"}))
	require.NoError(s.T(), err)
	results := checktest.ParseJSONOutput(s.T(), []byte(output))
	require.Len(s.T(), results, 1)
	require.Equal(s.T(), 1, results[0].Runner.TotalRuns)
	require.Zero(s.T(), results[0].Runner.TotalErrors)

	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			metricName,
			fakeintakeclient.WithTags[*aggregator.MetricSeries]([]string{
				"service:Dnscache",
				"e2e:powershell-check",
			}),
			fakeintakeclient.WithMetricValueInRange(0.5, 1.5),
		)
		require.NoError(c, err)
		assert.NotEmpty(c, metrics, "no %s metric with the expected value and tags received yet", metricName)
	}, 5*time.Minute, 10*time.Second)
}

func (s *powershellCheckWindowsSuite) TestRejectsCmdletInjection() {
	s.UpdateEnv(powerShellProvisioner(cmdletInjectionConfig))

	_, err := s.Env().Agent.Client.CheckWithError(agentclient.WithArgs([]string{"powershell", "--json"}))
	require.Error(s.T(), err)
	assert.ErrorContains(s.T(), err, "no valid check found")
	s.assertMarkerDoesNotExist(cmdletInjectionMarker)
}

func (s *powershellCheckWindowsSuite) TestBindsParameterInjectionAsData() {
	s.UpdateEnv(powerShellProvisioner(parameterInjectionConfig))

	output, err := s.Env().Agent.Client.CheckWithError(agentclient.WithArgs([]string{"powershell", "--json"}))
	require.NoError(s.T(), err)
	results := checktest.ParseJSONOutput(s.T(), []byte(output))
	require.Len(s.T(), results, 1)
	assert.Equal(s.T(), 1, results[0].Runner.TotalRuns)
	assert.Equal(s.T(), 1, results[0].Runner.TotalErrors,
		"the hostile service name should be passed to Get-Service as one invalid value")
	s.assertMarkerDoesNotExist(parameterInjectionMarker)
}

func (s *powershellCheckWindowsSuite) TestBindsWhereInjectionAsData() {
	s.UpdateEnv(powerShellProvisioner(whereInjectionConfig))

	_, err := s.Env().Agent.Client.CheckWithError(agentclient.WithArgs([]string{"powershell", "--json"}))
	require.NoError(s.T(), err, "the hostile where value should be compared as data and match no rows")
	s.assertMarkerDoesNotExist(whereInjectionMarker)
}

func (s *powershellCheckWindowsSuite) setAllowlistOwner(sid string) {
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`icacls "%s" /setowner "*%s"`, allowlistPath, sid))
	require.NoError(s.T(), err, "could not set the PowerShell allowlist owner to %s", sid)
}

func (s *powershellCheckWindowsSuite) allowlistOwnerSID() string {
	owner, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
		`([System.Security.Principal.NTAccount](Get-Acl "%s").Owner).Translate([System.Security.Principal.SecurityIdentifier]).Value`,
		allowlistPath,
	))
	require.NoError(s.T(), err, "could not read the PowerShell allowlist owner")
	return strings.TrimSpace(owner)
}

func (s *powershellCheckWindowsSuite) assertMarkerDoesNotExist(path string) {
	exists, err := s.Env().RemoteHost.FileExists(path)
	require.NoError(s.T(), err)
	assert.False(s.T(), exists, "PowerShell injection created sentinel file %s", path)
}

func (s *powershellCheckWindowsSuite) removeInjectionMarkers() {
	for _, path := range injectionMarkers {
		exists, err := s.Env().RemoteHost.FileExists(path)
		require.NoError(s.T(), err, "could not inspect injection sentinel %s", path)
		if exists {
			require.NoError(s.T(), s.Env().RemoteHost.Remove(path), "could not remove injection sentinel %s", path)
		}
	}
}
