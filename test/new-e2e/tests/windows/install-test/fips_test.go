// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package installtest

import (
	"fmt"
	"path/filepath"

	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/nettrace"

	"testing"

	"github.com/stretchr/testify/suite"
)

func TestFIPS(t *testing.T) {
	s := &testFIPSSuite{}
	run(t, s)
}

type testFIPSSuite struct {
	baseAgentMSISuite

	remoteMSIPath string
}

func (s *testFIPSSuite) SetupSuite() {
	if setupSuite, ok := any(&s.baseAgentMSISuite).(suite.SetupAllSuite); ok {
		setupSuite.SetupSuite()
	}

	host := s.Env().RemoteHost

	err := windowsCommon.EnableFIPSMode(host)
	s.Require().NoError(err)

	// Download the agent package before starting the network trace
	s.remoteMSIPath, err = windowsCommon.GetTemporaryFile(host)
	s.Require().NoError(err)
	err = windowsCommon.PutOrDownloadFile(host, s.AgentPackage.URL, s.remoteMSIPath)
	s.Require().NoError(err)
	s.T().Log("Agent package downloaded to ", s.remoteMSIPath)
}

func (s *testFIPSSuite) TearDownSuite() {
	if setupSuite, ok := any(&s.baseAgentMSISuite).(suite.TearDownAllSuite); ok {
		setupSuite.TearDownSuite()
	}

	s.cleanupOnSuccessInDevMode()
}

func (s *testFIPSSuite) TestFIPSInstall() {
	host := s.Env().RemoteHost

	var err error

	// start tracing
	nt := s.startNetTrace("nettrace-of-install")

	// install the agent
	s.T().Log("Installing the agent")
	_ = s.installAgentPackage(host, s.AgentPackage,
		windowsAgent.WithRemoteMSIPath(s.remoteMSIPath),
	)
	s.T().Log("Agent installed")

	// stop traces
	err = nt.Stop()
	s.Require().NoError(err)

	client := s.NewTestClientForHost(host)
	s.Require().True(client.AgentClient.IsReady(), "Agent should be running")

	// disable FIPS and ensure agent command fails
	err = windowsCommon.DisableFIPSMode(host)
	s.Require().NoError(err)
	installRoot, err := windowsAgent.GetInstallPathFromRegistry(host)
	s.Require().NoError(err)
	agentPath := filepath.Join(installRoot, "bin", "agent.exe")
	cmd := fmt.Sprintf("& '%s' version", agentPath)
	_, err = host.Execute(cmd)
	s.Require().ErrorContains(err, "panic: cngcrypto: not in FIPS mode", "agent command should fail after disabling FIPS")
}

func (s *testFIPSSuite) startNetTrace(traceName string) *nettrace.NetTrace {
	host := s.Env().RemoteHost
	nt, err := nettrace.New(host)
	s.Require().NoError(err)
	err = nt.Start()
	s.Require().NoError(err)
	// cleanup at the end of the test
	s.T().Cleanup(func() {
		_ = nt.Cleanup()
	})
	// collect traces at end of test
	s.T().Cleanup(func() {
		out := filepath.Join(s.OutputDir, traceName+".etl")
		err = host.GetFile(nt.TraceFile(), out)
		s.Require().NoError(err)
		s.T().Log("Trace file downloaded to ", out)
		// convert to pcap
		out = filepath.Join(s.OutputDir, traceName+".pcapng")
		err = nettrace.GetPCAPNG(nt, out)
		s.Require().NoError(err)
		s.T().Log("PCAPNG file downloaded to ", out)
	})
	s.T().Log("Tracing started")
	return nt
}
