// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This suite verifies that the logs registry survives abrupt Windows VM stops.
package registrydurability

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	awsec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	testos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const (
	agentServiceName      = "datadogagent"
	logFilePath           = `C:\logs\registry-durability.log`
	registryPath          = `C:\ProgramData\Datadog\run\registry.json`
	logService            = "registry-durability"
	powerCycleCount       = 3
	seededRegistryEntries = 4096
	minimumRegistrySize   = 256 * 1024
)

//go:embed config/config.yaml
var logConfig string

type registryDurabilitySuite struct {
	e2e.BaseSuite[environments.Host]
}

type registryState struct {
	Offset         int64 `json:"Offset"`
	FileLength     int64 `json:"FileLength"`
	RegistryLength int64 `json:"RegistryLength"`
}

// TestWindowsRegistryDurability verifies that an acknowledged registry update survives an abrupt
// machine stop. The Agent service is configured for manual startup so the test can inspect the
// registry after boot, before the Agent has a chance to recover or rewrite it.
func TestWindowsRegistryDurability(t *testing.T) {
	e2e.Run(t, &registryDurabilitySuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(testos.WindowsServerDefault)),
				ec2.WithAgentOptions(
					agentparams.WithLogs(),
					agentparams.WithIntegration("registry_durability.d", logConfig),
				),
			),
		),
	))
}

func (s *registryDurabilitySuite) TestRegistrySurvivesAbruptPowerLoss() {
	t := s.T()
	host := s.Env().RemoteHost

	_, err := host.Execute(fmt.Sprintf(`Set-Service -Name %s -StartupType Manual`, agentServiceName))
	require.NoError(t, err, "failed to configure the Agent service for manual startup")
	t.Cleanup(func() {
		_, cleanupErr := host.Execute(fmt.Sprintf(
			`Set-Service -Name %s -StartupType Automatic; if ((Get-Service -Name %s).Status -ne 'Running') { Start-Service -Name %s }`,
			agentServiceName,
			agentServiceName,
			agentServiceName,
		))
		if cleanupErr != nil {
			t.Logf("failed to restore the Agent service startup type: %v", cleanupErr)
		}
	})

	s.prepareLogFile()
	s.waitForRegistryOffset()
	s.seedLargeRegistry()

	ctx := t.Context()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	require.NoError(t, err, "failed to load AWS configuration")
	ec2Client := awsec2.NewFromConfig(awsCfg)
	instanceID, err := findInstanceID(ctx, ec2Client, host.Address)
	require.NoError(t, err, "failed to find the test EC2 instance")
	t.Logf("Testing registry durability with instance %s", instanceID)

	for cycle := 1; cycle <= powerCycleCount; cycle++ {
		t.Logf("Starting abrupt power cycle %d of %d", cycle, powerCycleCount)
		require.NoError(t, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

		bootTimeBefore := s.bootTime()
		line := fmt.Sprintf("registry-durability-cycle-%d", cycle)
		s.appendDurableLogLine(line)
		s.waitForLog(line)
		beforeCrash := s.waitForRegistryOffset()
		require.GreaterOrEqual(t, beforeCrash.RegistryLength, int64(minimumRegistrySize), "registry was not large enough to exercise a multi-block write")

		t.Logf("Abruptly stopping the instance after registry offset %d was observed", beforeCrash.Offset)
		require.NoError(t, hardPowerCycle(t.Context(), ec2Client, instanceID), "failed to power-cycle the test instance")
		s.waitForHostAfterPowerCycle(bootTimeBefore)

		// The service is intentionally still stopped. A valid state here proves that the
		// file survived the power loss, rather than being repaired by the Agent on startup.
		afterCrash, err := s.readRegistryState()
		require.NoError(t, err, "registry is not valid JSON after abrupt power loss")
		require.Equal(t, beforeCrash.Offset, afterCrash.Offset, "registry lost the latest acknowledged offset")
		require.Equal(t, beforeCrash.FileLength, afterCrash.FileLength, "durably written source log changed across power loss")
		require.GreaterOrEqual(t, afterCrash.RegistryLength, int64(minimumRegistrySize), "registry was truncated across power loss")

		s.startAgent()
	}
}

func (s *registryDurabilitySuite) prepareLogFile() {
	command := fmt.Sprintf(
		`New-Item -ItemType Directory -Path (Split-Path -Parent '%s') -Force | Out-Null; `+
			`New-Item -ItemType File -Path '%s' -Force | Out-Null; `+
			`icacls '%s' /grant 'ddagentuser:(R)' | Out-Null; `+
			`if ($LASTEXITCODE -ne 0) { throw "icacls failed with exit code $LASTEXITCODE" }`,
		logFilePath,
		logFilePath,
		logFilePath,
	)
	_, err := s.Env().RemoteHost.Execute(command)
	require.NoError(s.T(), err, "failed to prepare the source log file")

	// Give the tailer one durable line so there is a real registry entry to enlarge.
	s.appendDurableLogLine("registry-durability-initial")
	s.waitForLog("registry-durability-initial")
}

func (s *registryDurabilitySuite) appendDurableLogLine(line string) {
	command := fmt.Sprintf(
		`$bytes = [System.Text.Encoding]::UTF8.GetBytes('%s' + "`+"`n"+`"); `+
			`$stream = [System.IO.File]::Open('%s', [System.IO.FileMode]::Append, [System.IO.FileAccess]::Write, [System.IO.FileShare]::ReadWrite); `+
			`try { $stream.Write($bytes, 0, $bytes.Length); $stream.Flush($true) } finally { $stream.Dispose() }`,
		line,
		logFilePath,
	)
	_, err := s.Env().RemoteHost.Execute(command)
	require.NoError(s.T(), err, "failed to append and flush log line %q", line)
}

func (s *registryDurabilitySuite) waitForLog(line string) {
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs(logService, fakeintake.WithMessageContaining(line))
		require.NoError(c, err)
		require.Len(c, logs, 1, "expected exactly one copy of %q", line)
	}, 2*time.Minute, 5*time.Second)
}

func (s *registryDurabilitySuite) waitForRegistryOffset() registryState {
	var state registryState
	s.EventuallyWithT(func(c *assert.CollectT) {
		current, err := s.readRegistryState()
		require.NoError(c, err)
		require.Equal(c, current.FileLength, current.Offset, "registry has not recorded the complete source file")
		state = current
	}, 2*time.Minute, 250*time.Millisecond)
	return state
}

func (s *registryDurabilitySuite) readRegistryState() (registryState, error) {
	command := fmt.Sprintf(
		`$raw = [System.IO.File]::ReadAllText('%s'); `+
			`$registry = $raw | ConvertFrom-Json; `+
			`$entry = @($registry.Registry.PSObject.Properties | Where-Object { $_.Name -like '*registry-durability.log' })[0]; `+
			`if ($null -eq $entry) { throw 'source log is missing from the registry' }; `+
			`@{ Offset = [Int64]$entry.Value.Offset; FileLength = [Int64](Get-Item -LiteralPath '%s').Length; RegistryLength = [Int64]$raw.Length } | ConvertTo-Json -Compress`,
		registryPath,
		logFilePath,
	)
	output, err := s.Env().RemoteHost.Execute(command)
	if err != nil {
		return registryState{}, err
	}

	var state registryState
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &state); err != nil {
		return registryState{}, fmt.Errorf("parse registry state %q: %w", output, err)
	}
	return state, nil
}

func (s *registryDurabilitySuite) seedLargeRegistry() {
	_, err := s.Env().RemoteHost.Execute(`Stop-Service -Force -Name ` + agentServiceName)
	require.NoError(s.T(), err, "failed to stop the Agent before seeding the registry")

	command := fmt.Sprintf(
		`$registry = ([System.IO.File]::ReadAllText('%s') | ConvertFrom-Json); `+
			`$updated = [DateTime]::UtcNow.ToString('o'); `+
			`for ($i = 0; $i -lt %d; $i++) { `+
			`$entry = [PSCustomObject]@{ LastUpdated = $updated; Offset = '1'; TailingMode = 'end'; IngestionTimestamp = 0 }; `+
			`Add-Member -InputObject $registry.Registry -NotePropertyName ("file:C:\logs\registry-seed-{0:D4}.log" -f $i) -NotePropertyValue $entry }; `+
			`$json = $registry | ConvertTo-Json -Depth 8 -Compress; `+
			`$encoding = New-Object System.Text.UTF8Encoding $false; $bytes = $encoding.GetBytes($json); `+
			`$stream = [System.IO.File]::Open('%s', [System.IO.FileMode]::Create, [System.IO.FileAccess]::Write, [System.IO.FileShare]::Read); `+
			`try { $stream.Write($bytes, 0, $bytes.Length); $stream.Flush($true) } finally { $stream.Dispose() }`,
		registryPath,
		seededRegistryEntries,
		registryPath,
	)
	_, err = s.Env().RemoteHost.Execute(command)
	require.NoError(s.T(), err, "failed to seed a large registry")

	state, err := s.readRegistryState()
	require.NoError(s.T(), err, "seeded registry is invalid")
	require.GreaterOrEqual(s.T(), state.RegistryLength, int64(minimumRegistrySize), "seeded registry is unexpectedly small")
	s.startAgent()
}

func (s *registryDurabilitySuite) bootTime() int64 {
	output, err := s.Env().RemoteHost.Execute(`(Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToFileTimeUtc()`)
	require.NoError(s.T(), err, "failed to read Windows boot time")
	bootTime, err := strconv.ParseInt(strings.TrimSpace(output), 10, 64)
	require.NoError(s.T(), err, "failed to parse Windows boot time %q", output)
	return bootTime
}

func (s *registryDurabilitySuite) waitForHostAfterPowerCycle(previousBootTime int64) {
	s.EventuallyWithT(func(c *assert.CollectT) {
		require.NoError(c, s.Env().RemoteHost.Reconnect())
		output, err := s.Env().RemoteHost.Execute(`(Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToFileTimeUtc()`)
		require.NoError(c, err)
		currentBootTime, err := strconv.ParseInt(strings.TrimSpace(output), 10, 64)
		require.NoError(c, err)
		require.NotEqual(c, previousBootTime, currentBootTime, "Windows did not reboot")
		serviceStatus, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`(Get-Service -Name %s).Status.ToString()`, agentServiceName))
		require.NoError(c, err)
		require.Equal(c, "Stopped", strings.TrimSpace(serviceStatus), "Agent started before the registry could be inspected")
	}, 10*time.Minute, 10*time.Second)
}

func (s *registryDurabilitySuite) startAgent() {
	_, err := s.Env().RemoteHost.Execute(`Start-Service -Name ` + agentServiceName)
	require.NoError(s.T(), err, "failed to start the Agent")
	s.EventuallyWithT(func(c *assert.CollectT) {
		status, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`(Get-Service -Name %s).Status.ToString()`, agentServiceName))
		require.NoError(c, err)
		require.Equal(c, "Running", strings.TrimSpace(status))
	}, 2*time.Minute, 5*time.Second)
}

func findInstanceID(ctx context.Context, client *awsec2.Client, privateIP string) (string, error) {
	output, err := client.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{
		Filters: []awsec2types.Filter{
			{
				Name:   aws.String("private-ip-address"),
				Values: []string{privateIP},
			},
			{
				Name:   aws.String("instance-state-name"),
				Values: []string{"running"},
			},
		},
	})
	if err != nil {
		return "", err
	}

	var instanceIDs []string
	for _, reservation := range output.Reservations {
		for _, instance := range reservation.Instances {
			if instance.InstanceId != nil {
				instanceIDs = append(instanceIDs, *instance.InstanceId)
			}
		}
	}
	if len(instanceIDs) != 1 {
		return "", fmt.Errorf("expected one EC2 instance with private IP %s, found %d", privateIP, len(instanceIDs))
	}
	return instanceIDs[0], nil
}

func hardPowerCycle(ctx context.Context, client *awsec2.Client, instanceID string) error {
	_, err := client.StopInstances(ctx, &awsec2.StopInstancesInput{
		InstanceIds:    []string{instanceID},
		SkipOsShutdown: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("stop instance without OS shutdown: %w", err)
	}

	describeInput := &awsec2.DescribeInstancesInput{InstanceIds: []string{instanceID}}
	if err := awsec2.NewInstanceStoppedWaiter(client).Wait(ctx, describeInput, 5*time.Minute); err != nil {
		return fmt.Errorf("wait for instance to stop: %w", err)
	}
	if _, err := client.StartInstances(ctx, &awsec2.StartInstancesInput{InstanceIds: []string{instanceID}}); err != nil {
		return fmt.Errorf("start instance: %w", err)
	}
	if err := awsec2.NewInstanceRunningWaiter(client).Wait(ctx, describeInput, 5*time.Minute); err != nil {
		return fmt.Errorf("wait for instance to run: %w", err)
	}
	return nil
}
