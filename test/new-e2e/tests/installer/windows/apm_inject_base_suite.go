// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !e2eunit

package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cenkalti/backoff/v7"
	"go.yaml.in/yaml/v3"

	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

type baseAPMInjectSuite struct {
	BaseSuite
	currentAPMInjectVersion  PackageVersion
	previousAPMInjectVersion PackageVersion
}

func (s *baseAPMInjectSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	s.currentAPMInjectVersion = NewVersionFromPackageVersion(os.Getenv("CURRENT_APM_INJECT_VERSION"))
	if s.currentAPMInjectVersion.PackageVersion() == "" {
		s.currentAPMInjectVersion = NewVersionFromPackageVersion("0.52.0-dev.b282e14.glci1291404213.g7ff18a26-1")
	}
	s.previousAPMInjectVersion = NewVersionFromPackageVersion(os.Getenv("PREVIOUS_APM_INJECT_VERSION"))
	if s.previousAPMInjectVersion.PackageVersion() == "" {
		s.previousAPMInjectVersion = NewVersionFromPackageVersion("0.50.0-dev.ba30ecb.glci1208428525.g594e53fe-1")
	}
}
func (s *baseAPMInjectSuite) assertSuccessfulPromoteExperiment() {
	s.Require().Host(s.Env().RemoteHost).HasDatadogInstaller().Status().
		HasPackage("datadog-apm-inject")
	// verify the driver is running by checking the service status
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"ddinjector"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))
}

func (s *baseAPMInjectSuite) assertDriverInjections(enabled bool) {
	script := `
# We copy whoami.exe to another directory because System32 is ignored by the driver
$dst = "$env:TEMP\where.exe"
Copy-Item "C:\Windows\System32\whoami.exe" $dst -Force

$env:DD_INJECT_LOG_SINKS = "stdout"
$env:DD_INJECT_LOG_LEVEL = "debug"

& $dst
`
	host := s.Env().RemoteHost
	output, err := host.Execute(script)
	s.Require().NoError(err)
	if enabled {
		s.Require().Contains(output, "main executable path")
	} else {
		s.Require().NotContains(output, "main executable path")
	}
}

// queryInjectorStats queries the system-probe for injector stats using NamedPipeCmd.exe
func (s *baseAPMInjectSuite) queryInjectorStats(forceRefresh bool) map[string]interface{} {

	if forceRefresh {
		// Query system-probe's /telemetry endpoint to trigger the telemetry scheduler to collect stats.
		_, _ = s.querySystemProbe("/telemetry")

		// The scheduler has a delay start before running down the Collect callbacks.
		// Give enough time for the delay start and collection.
		time.Sleep(10 * time.Second)
	}

	// Query system-probe's /debug/stats endpoint.
	// We cannot use the /telemetry endpoint because it returns a compressed output.
	output, err := s.querySystemProbe("/debug/stats")
	if output != "" {
		s.T().Logf("system-probe output:\n\n%s\n", output)
	}
	s.Require().NoErrorf(err, "failed to query system-probe: %s", output)

	// Parse JSON response
	var jsonOutput map[string]interface{}
	err = json.Unmarshal([]byte(output), &jsonOutput)
	s.Require().NoErrorf(err, "failed to parse JSON response: %s", output)

	stats, ok := jsonOutput["injector"].(map[string]interface{})
	s.Require().True(ok, "injector stats not found in JSON response")

	return stats
}

// enableInjectorTelemetry enables reporting of injector telemetry via system-probe config
// Uses the read/modify/write pattern to preserve existing config settings
func (s *baseAPMInjectSuite) enableInjectorTelemetry() {
	host := s.Env().RemoteHost
	configRoot, err := windowsAgent.GetConfigRootFromRegistry(host)
	s.Require().NoError(err)
	configPath := filepath.Join(configRoot, "system-probe.yaml")

	// Read existing config (or create empty map if file doesn't exist)
	config, err := s.readYamlConfig(configPath)
	if err != nil {
		// If file doesn't exist, start with empty config
		config = make(map[string]interface{})
	}

	// Explicitly enable injector telemetry.
	// If /telemetry supports uncompressed output, make sure to also enable
	// RAR with remote_agent.registry.enabled = true.
	config["injector"] = map[string]interface{}{
		"enable_telemetry": true,
	}

	// Write back the modified config
	err = s.writeYamlConfig(configPath, config)
	s.Require().NoErrorf(err, "failed to write system-probe config")

	// Restart system-probe to pick up the config. dd-procmgr supervises it, so restarting the
	// legacy SCM service would leave the running process on the old config.
	s.restartUnderProcmgr(sysprobeProcmgrProcess)

	s.waitForServiceRunning()
}

// readYamlConfig reads and unmarshals a YAML config file from the remote host
func (s *baseAPMInjectSuite) readYamlConfig(path string) (map[string]interface{}, error) {
	host := s.Env().RemoteHost
	configBytes, err := host.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := make(map[string]interface{})
	err = yaml.Unmarshal(configBytes, &config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// writeYamlConfig marshals and writes a YAML config file to the remote host
func (s *baseAPMInjectSuite) writeYamlConfig(path string, config map[string]interface{}) error {
	host := s.Env().RemoteHost
	configYaml, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	_, err = host.WriteFile(path, configYaml)
	return err
}

// installCurrentAgentVersionWithAPMInject installs the current agent version with APM inject via script
func (s *baseAPMInjectSuite) installCurrentAgentVersionWithAPMInject(opts ...Option) {
	output, err := s.InstallScript().Run(opts...)
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		})

	s.waitForServiceRunning()
}

func (s *baseAPMInjectSuite) waitForServiceRunning() {
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"ddinjector"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))
}

func (s *baseAPMInjectSuite) querySystemProbe(queryPath string) (string, error) {
	// PowerShell script with inline C# to query system-probe with a named pipe.
	scriptTemplate := `
$code = @"
using System;
using System.IO;
using System.IO.Pipes;
using System.Text;

public class NamedPipeClient
{
    public static string QuerySystemProbe(string pipeName, string httpPath)
    {
        using (var pipe = new NamedPipeClientStream(".", pipeName, PipeDirection.InOut))
        {
            pipe.Connect(5000); // 5 second timeout

            // Send HTTP GET request
            string request = string.Format("GET {0} HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n", httpPath);
            byte[] requestBytes = Encoding.UTF8.GetBytes(request);
            pipe.Write(requestBytes, 0, requestBytes.Length);
            pipe.Flush();

            // Read response
            using (var reader = new StreamReader(pipe, Encoding.UTF8))
            {
                string response = reader.ReadToEnd();

                // Extract JSON body from HTTP response (after headers)
                int bodyStart = response.IndexOf("\r\n\r\n");
                if (bodyStart > 0)
                {
                    return response.Substring(bodyStart + 4);
                }

                throw new Exception("Failed to parse HTTP response");
            }
        }
    }
}
"@

Add-Type -TypeDefinition $code -Language CSharp

try {
    $result = [NamedPipeClient]::QuerySystemProbe("dd_system_probe", "%s")
    Write-Output $result
} catch {
    Write-Error "Failed to query system-probe: $_"
    exit 1
}`

	script := fmt.Sprintf(scriptTemplate, queryPath)
	host := s.Env().RemoteHost
	return host.Execute(script)
}
