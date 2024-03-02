// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package installtest contains e2e tests for the Windows agent installer
package installtest

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"

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

// dynamicAgentUserTestCase is a test case that needs information from the host before
// it can be fully defined.
type dynamicAgentUserTestCase struct {
	staticAgentUserTestCase
	setHostname func(tc *dynamicAgentUserTestCase, hostname string)
}

func (tc *dynamicAgentUserTestCase) HostReady(host *components.RemoteHost) error {
	hostinfo, err := windowsCommon.GetHostInfo(host)
	if err != nil {
		return err
	}

	domainPart := windowsCommon.NameToNetBIOSName(hostinfo.Hostname)
	tc.setHostname(tc, domainPart)

	return nil
}

// hostReadyUpdater is an interface for test cases that need to update their state when
// the test host becomes available.
type hostReadyUpdater interface {
	HostReady(host *components.RemoteHost) error
}

type agentUserTestSuite struct {
	baseAgentMSISuite

	tc agentUserTestCase
}

func (s *agentUserTestSuite) GetStackNamePart() (string, error) {
	if s.mustUseNewStack() {
		// include the subtest name to differentiate the stacks
		return s.tc.TC().name, nil
	}

	// otherwise, keep the stack name the same so the same resource is used
	return "", nil
}

func (s *agentUserTestSuite) mustUseNewStack() bool {
	// if we're running in parallel, each test case must use its own stack
	return !s.shouldRunParallel()
}

func (s *agentUserTestSuite) shouldRunParallel() bool {
	// run the tests in parallel in the CI
	return os.Getenv("CI") != ""
}

// TC-INS-006
func TestAgentUser(t *testing.T) {
	tcs := []agentUserTestCase{
		&dynamicAgentUserTestCase{
			staticAgentUserTestCase{name: "user_only"},
			func(tc *dynamicAgentUserTestCase, hostname string) {
				tc.username = "testuser"
				tc.expectedDomain = hostname
				tc.expectedUser = "testuser"
			}},
		&dynamicAgentUserTestCase{
			staticAgentUserTestCase{name: "dotslash_user"},
			func(tc *dynamicAgentUserTestCase, hostname string) {
				tc.username = ".\\testuser"
				tc.expectedDomain = hostname
				tc.expectedUser = "testuser"
			}},
		&dynamicAgentUserTestCase{
			staticAgentUserTestCase{name: "domain_user"},
			func(tc *dynamicAgentUserTestCase, hostname string) {
				tc.username = fmt.Sprintf("%s\\testuser", hostname)
				tc.expectedDomain = hostname
				tc.expectedUser = "testuser"
			}},
		&staticAgentUserTestCase{"LocalSystem", "LocalSystem", "NT AUTHORITY", "SYSTEM"},
		&staticAgentUserTestCase{"SYSTEM", "SYSTEM", "NT AUTHORITY", "SYSTEM"},
	}
	for _, tc := range tcs {
		s := &agentUserTestSuite{
			tc: tc,
		}
		t.Run(tc.TC().name, func(t *testing.T) {
			if s.shouldRunParallel() {
				t.Parallel()
			}
			run(t, s)
		})
	}
}

func (s *agentUserTestSuite) TestAgentUser() {
	vm := s.Env().RemoteHost
	s.prepareHost()

	// provide the test case with the host if it needs it
	if tc, ok := s.tc.(hostReadyUpdater); ok {
		err := tc.HostReady(vm)
		s.Require().NoError(err)
	}

	tc := s.tc.TC()

	t := s.installAgent(vm, nil,
		WithInstallUser(tc.username),
		WithExpectedAgentUser(tc.expectedDomain, tc.expectedUser),
	)

	if !t.TestExpectations(s.T()) {
		s.T().FailNow()
	}

	t.TestUninstall(s.T(), filepath.Join(s.OutputDir, "uninstall.log"))
}
