// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"github.com/stretchr/testify/suite"
	"testing"
)

type agentUserTestCase interface {
	TC() *staticAgentUserTestCase
}

// staticAgentUserTestCase is a test case that can be fully defined without information from the host
type staticAgentUserTestCase struct {
	name           string
	username       string
	expectedDomain string
	expectedUser   string
}

func (tc *staticAgentUserTestCase) TC() *staticAgentUserTestCase {
	return tc
}

// agentUserTestCaseWithHostInfo is a test case that needs information from the host before
// it can be fully defined.
type agentUserTestCaseWithHostInfo struct {
	staticAgentUserTestCase
	HostInfoUpdater func(tc *agentUserTestCaseWithHostInfo, hostInfo *windowsCommon.HostInfo)
}

func (tc *agentUserTestCaseWithHostInfo) HostReady(host *components.RemoteHost) error {
	hostinfo, err := windowsCommon.GetHostInfo(host)
	if err != nil {
		return err
	}

	tc.HostInfoUpdater(tc, hostinfo)

	return nil
}

// hostReadyUpdater is an interface for test cases that need to update their state when
// the test host becomes available.
type hostReadyUpdater interface {
	HostReady(host *components.RemoteHost) error
}

type agentUserSuite struct {
	baseAgentMSISuite

	tc agentUserTestCase
}

func (s *agentUserSuite) SetupSuite() {
	if setupSuite, ok := any(&s.BaseAgentInstallerSuite).(suite.SetupAllSuite); ok {
		setupSuite.SetupSuite()
	}

	// provide the test case with the host if it needs it
	if u, ok := s.tc.(hostReadyUpdater); ok {
		err := u.HostReady(s.Env().RemoteHost)
		s.Require().NoError(err)
	}
}

func (s *agentUserSuite) TestAgentUser() {
	tc := s.tc.TC()
	vm := s.Env().RemoteHost
	// initialize test helper
	t := s.newTester(vm,
		WithExpectedAgentUser(tc.expectedDomain, tc.expectedUser),
	)

	// install the agent
	_ = s.installAgentPackage(vm, s.AgentPackage,
		windowsAgent.WithAgentUser(tc.username),
	)

	// run tests
	if !t.TestInstallExpectations(s.T()) {
		s.T().FailNow()
	}

	s.uninstallAgentAndRunUninstallTests(t)
}

// TC-INS-006
//
// TestAgentUser tests the supported formats for providing the agent usre to the Windows Agent Installer.
//
// In general, we want to support running each installer test case in a clean environment and in parallel. But E2E does not currently
// export environment management outside of e2e.Suite. Even e2e.Suite doesn't directly export create/teardown, that behavior
// is only indirectly exported through testify.Suite SetupSuite/TearDownSuite callbacks. There is also a 1:1 relationship
// between e2e.Suite and the environment. So we can't create multiple environments to run each test case within the suite itself.
// Even if we could, testify.Suite does not support parallelism on its Test* methods.
//
// Thus, in order to support clean environments for each test case and running test cases in parallel we need to
// run the whole suite for each individual test case.
// Unfortunately, some of our test cases require information from the environment before they can be fully defined,
// so it can't be as simple as passing some parameters to the suite. We must implement a callback to update the test case
// with the necessary information from the environment once e2e.Suite has created the environment.
func TestAgentUser(t *testing.T) {
	tcs := []agentUserTestCase{
		&agentUserTestCaseWithHostInfo{
			staticAgentUserTestCase{name: "user_only"},
			func(tc *agentUserTestCaseWithHostInfo, hostInfo *windowsCommon.HostInfo) {
				tc.username = "testuser"
				tc.expectedDomain = windowsCommon.NameToNetBIOSName(hostInfo.Hostname)
				tc.expectedUser = "testuser"
			}},
		&agentUserTestCaseWithHostInfo{
			staticAgentUserTestCase{name: "dotslash_user"},
			func(tc *agentUserTestCaseWithHostInfo, hostInfo *windowsCommon.HostInfo) {
				tc.username = ".\\testuser"
				tc.expectedDomain = windowsCommon.NameToNetBIOSName(hostInfo.Hostname)
				tc.expectedUser = "testuser"
			}},
		&agentUserTestCaseWithHostInfo{
			staticAgentUserTestCase{name: "hostname_user"},
			func(tc *agentUserTestCaseWithHostInfo, hostInfo *windowsCommon.HostInfo) {
				h := windowsCommon.NameToNetBIOSName(hostInfo.Hostname)
				tc.username = fmt.Sprintf("%s\\testuser", h)
				tc.expectedDomain = h
				tc.expectedUser = "testuser"
			}},
		&staticAgentUserTestCase{"LocalSystem", "LocalSystem", "NT AUTHORITY", "SYSTEM"},
		&staticAgentUserTestCase{"SYSTEM", "SYSTEM", "NT AUTHORITY", "SYSTEM"},
	}
	for _, tc := range tcs {
		s := &agentUserSuite{
			tc: tc,
		}
		t.Run(tc.TC().name, func(t *testing.T) {
			run(t, s)
		})
	}
}
