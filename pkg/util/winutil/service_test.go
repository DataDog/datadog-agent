// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// copied from go\pkg\mod\golang.org\x\sys@v0.4.0\windows\svc\mgr\mgr_test.go
func install(t *testing.T, m *mgr.Mgr, name, exepath string, c mgr.Config) {
	// Sometimes it takes a while for the service to get
	// removed after previous test run.
	for i := 0; ; i++ {
		s, err := m.OpenService(name)
		if err != nil {
			break
		}
		s.Close()

		if i > 10 {
			t.Fatalf("service %s already exists", name)
		}
		time.Sleep(300 * time.Millisecond)
	}

	s, err := m.CreateService(name, exepath, c)
	if err != nil {
		t.Fatalf("CreateService(%s) failed: %v", name, err)
	}
	defer s.Close()
}

// copied from go\pkg\mod\golang.org\x\sys@v0.4.0\windows\svc\mgr\mgr_test.go
func remove(t *testing.T, s *mgr.Service) {
	err := s.Delete()
	if err != nil {
		t.Fatalf("Delete failed: %s", err)
	}
}

func TestOpenSCManager(t *testing.T) {

	// test that OpenSCManager returns an error if the desired access is not valid
	_, err := OpenSCManager(777)
	assert.NotNil(t, err, "Expected OpenSCManager to return an error with invalid desired access")

	// test that OpenSCManager returns a valid manager handle with valid desired access
	m, err := OpenSCManager(windows.SC_MANAGER_CONNECT)
	assert.Nilf(t, err, "Unexpected error: %v", err)
	assert.NotNil(t, m, "Expected OpenSCManager to return a valid manager handle")
	defer m.Disconnect()
}

func TestOpenService(t *testing.T) {

	// open the SC manager with CONNECT and CREATE_SERVICE
	m, err := OpenSCManager(windows.SC_MANAGER_CONNECT | windows.SC_MANAGER_CREATE_SERVICE)
	assert.Nilf(t, err, "Unexpected error: %v", err)
	assert.NotNil(t, m, "Expected OpenSCManager to return a valid manager handle")
	defer m.Disconnect()

	// test that OpenService returns an error with non-existent service
	_, err = OpenService(m, "nottestingservice", windows.SERVICE_START)
	assert.NotNil(t, err, "Expected OpenService to return an error with invalid service name")

	c := mgr.Config{
		StartType:    mgr.StartDisabled,
		DisplayName:  "Test Service",
		Description:  "This is a test service",
		Dependencies: []string{"LanmanServer", "W32Time"},
	}
	install(t, m, "testingservice", "", c)

	// test that OpenService returns a valid service handle with a valid service name
	s, err := OpenService(m, "testingservice", windows.SERVICE_START|windows.DELETE)
	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.NotNil(t, s, "Expected OpenService to return a valid service handle")
	remove(t, s)
	s.Close()
}

func TestListDependentServices(t *testing.T) {

	// test that ListDependentServices returns an error if the service name is not valid
	notAServiceName := "notaservice"
	_, err := ListDependentServices(notAServiceName, windows.SERVICE_ACTIVE)
	assert.NotNil(t, err, "Expected ListDependentServices to return an error with invalid service name")

	// open the SC manager so we can create some test services
	m, err := OpenSCManager(windows.SC_MANAGER_CONNECT | windows.SC_MANAGER_CREATE_SERVICE)
	assert.Nilf(t, err, "Unexpected error: %v", err)
	assert.NotNil(t, m, "Expected OpenSCManager to return a valid manager handle")
	defer m.Disconnect()

	// install the base service, it will have no dependents, but several dependents
	baseServiceName := "baseservice"
	c := mgr.Config{
		StartType:   mgr.StartDisabled,
		DisplayName: "Base Test Service",
		Description: "This is the base test service",
	}
	install(t, m, baseServiceName, "", c)

	// install a first level dependent service, it will depend on the base service
	firstLevelServiceName := "dependentservice-firstlevel"
	c = mgr.Config{
		StartType:    mgr.StartDisabled,
		DisplayName:  "Dependent Test Service: First Level",
		Description:  "This service depends on the base service",
		Dependencies: []string{baseServiceName},
	}
	install(t, m, firstLevelServiceName, "", c)

	// install a second level service, it will depend on the first level
	secondLevelServiceName := "dependentservice-secondlevel"
	c = mgr.Config{
		StartType:    mgr.StartDisabled,
		DisplayName:  "Dependent Test Service: Second Level",
		Description:  "This service depends on the dependent service",
		Dependencies: []string{firstLevelServiceName},
	}
	install(t, m, secondLevelServiceName, "", c)

	// test that ListDependentServices returns an empty list for service with no dependents
	deps, err := ListDependentServices(secondLevelServiceName, windows.SERVICE_STATE_ALL)
	assert.Nilf(t, err, "Unexpected error: %v", err)
	assert.Zero(t, len(deps), "")

	// test that ListDependentServices returns a list of dependent services for service with dependents
	deps, err = ListDependentServices(firstLevelServiceName, windows.SERVICE_STATE_ALL)
	assert.Nilf(t, err, "Unexpected error: %v", err)
	assert.Equal(t, 1, len(deps), "Expected ListDependentServices to return a list of dependent services")

	// the deps
	t.Logf("%s has the following dependents", firstLevelServiceName)
	for _, dep := range deps {
		t.Log(dep.serviceName)
	}

	// test that ListDependentServices returns a list of dependent services for service with nested dependents
	deps, err = ListDependentServices(baseServiceName, windows.SERVICE_STATE_ALL)
	assert.Nilf(t, err, "Unexpected error: %v", err)
	assert.Equal(t, 2, len(deps), "Expected ListDependentServices to return a list of dependent services")

	// ensure that we get the deps back in reverse startup order
	// so that when this is used to stop services, we stop the outermost ones
	// first
	// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-enumdependentservicesa#parameters
	assert.EqualValues(t, secondLevelServiceName, deps[0].serviceName)
	assert.EqualValues(t, firstLevelServiceName, deps[1].serviceName)

	// the deps
	t.Logf("%s has the following dependents", baseServiceName)
	for _, dep := range deps {
		t.Log(dep.serviceName)
	}

	s, err := OpenService(m, baseServiceName, windows.SERVICE_START|windows.DELETE)
	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.NotNil(t, s, "Expected OpenService to return a valid service handle")
	remove(t, s)
	s.Close()

	s, err = OpenService(m, firstLevelServiceName, windows.SERVICE_START|windows.DELETE)
	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.NotNil(t, s, "Expected OpenService to return a valid service handle")
	remove(t, s)
	s.Close()

	s, err = OpenService(m, secondLevelServiceName, windows.SERVICE_START|windows.DELETE)
	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.NotNil(t, s, "Expected OpenService to return a valid service handle")
	remove(t, s)
	s.Close()
}

type StartStopServiceRaceConditionTestSuite struct {
	suite.Suite
	mainSvc     *mgr.Service
	dep1Svc     *mgr.Service
	dep2Svc     *mgr.Service
	mainSvcName string
	dep1SvcName string
	dep2SvcName string

	stopServiceCallback
	beforeStopServiceInvokeCount   int
	startingServiceOnFirstCallback bool
	startedServiceOnFirstCallback  bool

	afterDependentsEnumerationInvokeCount       int
	depSvcNameToStartAfterDependentsEnumeration string
}

func (s *StartStopServiceRaceConditionTestSuite) SetupSuite() {
	// ------------------------------
	//
	// Setup test's real main and two dependent services using existing "snmptrap.exe" as service entry point
	// Logistic to unit tests to make real functional services working is big pain because of the needs to
	// provide a real service executable just for testing purposes during CI. So using existing builtin
	// "snmptrap.exe" as a binary seems to be working just fine.
	svcExePath := "C:\\Windows\\System32\\snmptrap.exe"

	svcNameTemp := "datadog-test-service-%s"
	displayNameTemp := "Datadog Test Service %s"
	descriptionNameTemp := "This is a Datdog test service %s"
	assertErrNilTempl := "Unexpected error creating %s service: %v"
	assertSvcNilTempl := "Expected OpenService to return a valid service handle for %s service"

	s.mainSvcName = fmt.Sprintf(svcNameTemp, "main")
	s.dep1SvcName = fmt.Sprintf(svcNameTemp, "dep1")
	s.dep2SvcName = fmt.Sprintf(svcNameTemp, "dep2")

	svcFlags := uint32(windows.SERVICE_START | windows.SERVICE_STOP |
		windows.DELETE | windows.SERVICE_QUERY_STATUS)

	// SCM
	m, err := OpenSCManager(windows.SC_MANAGER_CONNECT | windows.SC_MANAGER_CREATE_SERVICE)
	assert.Nilf(s.T(), err, "Unexpected error: %v", err)
	assert.NotNil(s.T(), m, "Expected OpenSCManager to return a valid manager handle")
	defer m.Disconnect()

	// Main service
	c := mgr.Config{
		StartType:   mgr.StartManual,
		DisplayName: fmt.Sprintf(displayNameTemp, "Main"),
		Description: fmt.Sprintf(descriptionNameTemp, "main"),
	}
	install(s.T(), m, s.mainSvcName, svcExePath, c)
	s.mainSvc, err = OpenService(m, s.mainSvcName, svcFlags)
	assert.Nil(s.T(), err, assertErrNilTempl, "main", err)
	assert.NotNil(s.T(), s.mainSvc, assertSvcNilTempl, "main")

	// Dependent service 1
	c = mgr.Config{
		StartType:    mgr.StartManual,
		DisplayName:  fmt.Sprintf(displayNameTemp, "Dep1"),
		Description:  fmt.Sprintf(descriptionNameTemp, "dep1"),
		Dependencies: []string{s.mainSvcName},
	}
	install(s.T(), m, s.dep1SvcName, svcExePath, c)
	s.dep1Svc, err = OpenService(m, s.dep1SvcName, svcFlags)
	assert.Nil(s.T(), err, assertErrNilTempl, "dep1", err)
	assert.NotNil(s.T(), s.mainSvc, assertSvcNilTempl, "dep1")

	// Dependent service 2
	c = mgr.Config{
		StartType:    mgr.StartManual,
		DisplayName:  fmt.Sprintf(displayNameTemp, "Dep2"),
		Description:  fmt.Sprintf(descriptionNameTemp, "dep2"),
		Dependencies: []string{s.mainSvcName},
	}
	install(s.T(), m, s.dep2SvcName, svcExePath, c)
	s.dep2Svc, err = OpenService(m, s.dep2SvcName, svcFlags)
	assert.Nil(s.T(), err, assertErrNilTempl, "dep2", err)
	assert.NotNil(s.T(), s.mainSvc, assertSvcNilTempl, "dep2")
}

func (s *StartStopServiceRaceConditionTestSuite) TearDownSuite() {
	if s.dep2Svc != nil {
		remove(s.T(), s.dep2Svc)
		s.dep2Svc.Close()
	}
	if s.dep1Svc != nil {
		remove(s.T(), s.dep1Svc)
		s.dep1Svc.Close()
	}
	if s.mainSvc != nil {
		remove(s.T(), s.mainSvc)
		s.mainSvc.Close()
	}
}

// nothing special to do
func (s *StartStopServiceRaceConditionTestSuite) SetupTest() {
	s.beforeStopServiceInvokeCount = 0
	s.startingServiceOnFirstCallback = false
	s.startedServiceOnFirstCallback = false

	s.afterDependentsEnumerationInvokeCount = 0
	s.depSvcNameToStartAfterDependentsEnumeration = ""
}

// stop all services which may be running after each test
func (s *StartStopServiceRaceConditionTestSuite) TearDownTest() {
	StopService(s.dep2SvcName)
	StopService(s.dep1SvcName)
	StopService(s.mainSvcName)
}

func (s *StartStopServiceRaceConditionTestSuite) TestBasicRestartWhenNoSvcRunning() {
	err := RestartService(s.mainSvcName)
	assert.Nil(s.T(), err, "Main service should restar succesfully: %v", err)

	var status svc.Status
	status, err = s.mainSvc.Query()
	assert.Nil(s.T(), err, "Main service should restar succesfully: %v", err)
	assert.True(s.T(), status.State == svc.StartPending || status.State == svc.Running,
		"Main service should be starting or running: %v", status)
}

func (s *StartStopServiceRaceConditionTestSuite) TestBasicRestartWhenOnlyMainIsRunning() {
	// SETUP: Start main service
	err := s.mainSvc.Start()
	assert.Nil(s.T(), err, "Failed to start `main` service: %v", err)
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	err = WaitForState(ctx, s.mainSvcName, svc.Running)
	assert.Nil(s.T(), err, "`Main` service should be running succesfully: %v", err)

	// TEST: Restart main service
	err = RestartService(s.mainSvcName)
	assert.Nil(s.T(), err, "`Main` service should restart succesfully: %v", err)
	// VALIDATION
	ctx, _ = context.WithTimeout(context.Background(), 10*time.Second)
	err = WaitForState(ctx, s.mainSvcName, svc.Running)
	assert.Nil(s.T(), err, "`Main` service should be running: %v", err)
}

func (s *StartStopServiceRaceConditionTestSuite) TestBasicRestartWhenAllServicesAreRunning() {
	// SETUP: Start all services
	err := s.mainSvc.Start()
	assert.Nil(s.T(), err, "Failed to start `main` service: %v", err)
	err = s.dep1Svc.Start()
	assert.Nil(s.T(), err, "Failed to start `dep1` service: %v", err)
	err = s.dep2Svc.Start()
	assert.Nil(s.T(), err, "Failed to start `dep2` service: %v", err)
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	err = WaitForState(ctx, s.mainSvcName, svc.Running)
	assert.Nil(s.T(), err, "`Main` service should be running: %v", err)
	err = WaitForState(ctx, s.dep1SvcName, svc.Running)
	assert.Nil(s.T(), err, "`Dep1` service should be running: %v", err)
	err = WaitForState(ctx, s.dep2SvcName, svc.Running)
	assert.Nil(s.T(), err, "`Dep2` service should be running: %v", err)

	// TEST: Restart (only) main service
	err = RestartService(s.mainSvcName)
	assert.Nil(s.T(), err, "Main service should restart succesfully: %v", err)
	// VALIDATION: Two dependent services should be stopped because nobody
	// starts them like "datadogagent" service does). But they both should not
	// be running and it will be indication that main service was able to be
	// restarted.
	var status svc.Status
	ctx, _ = context.WithTimeout(context.Background(), 10*time.Second)
	err = WaitForState(ctx, s.mainSvcName, svc.Running)
	assert.Nil(s.T(), err, "Unexpected error of making `main` service running: %v", err)

	status, err = s.dep1Svc.Query()
	assert.Nil(s.T(), err, "Unexpected error getting `dep1` service status: %v", err)
	assert.Equal(s.T(), status.State, svc.Stopped, "`Dep1` service should be stopped")

	status, err = s.dep2Svc.Query()
	assert.Nil(s.T(), err, "Unexpected error getting `dep2` service status: %v", err)
	assert.Equal(s.T(), status.State, svc.Stopped, "`Dep2` service should be stopped")
}

// general callback to stop service to be used in setting up edge cases for race conditions
func (s *StartStopServiceRaceConditionTestSuite) beforeStopService(serviceName string) {
	s.beforeStopServiceInvokeCount++

	if (s.startingServiceOnFirstCallback || s.startedServiceOnFirstCallback) && s.beforeStopServiceInvokeCount == 1 {
		err := StartService(serviceName)
		assert.Nil(s.T(), err, "Unexpected error of starting `%s` service: %v", serviceName, err)
		if s.startedServiceOnFirstCallback {
			ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
			err = WaitForState(ctx, serviceName, svc.Running)
			assert.Nil(s.T(), err, "`%s` service should be running: %v", serviceName, err)
		}
	}
}

func (s *StartStopServiceRaceConditionTestSuite) afterDependentsEnumeration() {
	s.afterDependentsEnumerationInvokeCount++

	if s.afterDependentsEnumerationInvokeCount == 1 && len(s.depSvcNameToStartAfterDependentsEnumeration) > 0 {
		err := StartService(s.depSvcNameToStartAfterDependentsEnumeration)
		assert.Nil(s.T(), err, "Unexpected error of starting `%s` service: %v", s.depSvcNameToStartAfterDependentsEnumeration, err)
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		err = WaitForState(ctx, s.depSvcNameToStartAfterDependentsEnumeration, svc.Running)
		assert.Nil(s.T(), err, "`%s` service should be running: %v", s.depSvcNameToStartAfterDependentsEnumeration, err)
	}
}

func (s *StartStopServiceRaceConditionTestSuite) TestStopMainWhenDependentServiceStartingAfterEnumeration() {
	// SETUP: Start all services
	err := s.mainSvc.Start()
	assert.Nil(s.T(), err, "Failed to start `main` service: %v", err)
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	err = WaitForState(ctx, s.mainSvcName, svc.Running)
	assert.Nil(s.T(), err, "`Main` service should be running: %v", err)

	// TEST: Restart (only) main service
	s.startedServiceOnFirstCallback = true
	err = stopServiceInternal(s.mainSvcName, windows.SERVICE_STATE_ALL, s)
	assert.Nil(s.T(), err, "Main service should restart succesfully: %v", err)
	// VALIDATION: All  services should be stopped
	var status svc.Status
	status, err = s.mainSvc.Query()
	assert.Nil(s.T(), err, "Unexpected error getting `main` service status: %v", err)
	assert.Equal(s.T(), status.State, svc.Stopped, "`Main` service should be stopped")

	status, err = s.dep1Svc.Query()
	assert.Nil(s.T(), err, "Unexpected error getting `dep1` service status: %v", err)
	assert.Equal(s.T(), status.State, svc.Stopped, "`Dep1` service should be stopped")

	status, err = s.dep2Svc.Query()
	assert.Nil(s.T(), err, "Unexpected error getting `dep2` service status: %v", err)
	assert.Equal(s.T(), status.State, svc.Stopped, "`Dep2` service should be stopped")
}

func (s *StartStopServiceRaceConditionTestSuite) TestStopMainWhenDependentServiceStartedAfterEnumeration() {
	// SETUP: Start all services
	err := s.mainSvc.Start()
	assert.Nil(s.T(), err, "Failed to start `main` service: %v", err)
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	err = WaitForState(ctx, s.mainSvcName, svc.Running)
	assert.Nil(s.T(), err, "`Main` service should be running: %v", err)

	// TEST: Restart (only) main service
	s.startingServiceOnFirstCallback = true
	err = stopServiceInternal(s.mainSvcName, windows.SERVICE_STATE_ALL, s)
	assert.Nil(s.T(), err, "Main service should restart succesfully: %v", err)
	// VALIDATION: All  services should be stopped
	var status svc.Status
	status, err = s.mainSvc.Query()
	assert.Nil(s.T(), err, "Unexpected error getting `main` service status: %v", err)
	assert.Equal(s.T(), status.State, svc.Stopped, "`Main` service should be stopped")

	status, err = s.dep1Svc.Query()
	assert.Nil(s.T(), err, "Unexpected error getting `dep1` service status: %v", err)
	assert.Equal(s.T(), status.State, svc.Stopped, "`Dep1` service should be stopped")

	status, err = s.dep2Svc.Query()
	assert.Nil(s.T(), err, "Unexpected error getting `dep2` service status: %v", err)
	assert.Equal(s.T(), status.State, svc.Stopped, "`Dep2` service should be stopped")
}

// This test is similar to the previous one, but it tests the old code path
// which should fail as a proof that we have fixed the problem not acidently
func (s *StartStopServiceRaceConditionTestSuite) TestStopMainWhenDependentServiceStartedAfterEnumerationShouldFailInOldCode() {
	// Disable skip to run this test if needed to observe old code behavior (it is a waste of time otherwise)
	s.T().Skip("skipping test which useful only to prove that old code logic would fail even after small code modification")
	// SETUP: Start all services
	err := s.mainSvc.Start()
	assert.Nil(s.T(), err, "Failed to start `main` service: %v", err)
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	err = WaitForState(ctx, s.mainSvcName, svc.Running)
	assert.Nil(s.T(), err, "`Main` service should be running: %v", err)

	// TEST: Restart (only) main service
	s.depSvcNameToStartAfterDependentsEnumeration = s.dep1SvcName
	err = stopServiceInternal(s.mainSvcName, windows.SERVICE_ACTIVE, s)
	assert.NotNil(s.T(), err, "Main service stop should fail")
	assert.ErrorIs(s.T(), errors.Unwrap(err), error(windows.ERROR_DEPENDENT_SERVICES_RUNNING),
		"Main service stop should fail because of dependent services running")

	// VALIDATION: Main and Dep1 service should running and Dep2 should be stopped
	var status svc.Status
	status, err = s.mainSvc.Query()
	assert.Nil(s.T(), err, "Unexpected error getting `main` service status: %v", err)
	assert.Equal(s.T(), status.State, svc.Running, "`Main` service should be running")

	status, err = s.dep1Svc.Query()
	assert.Nil(s.T(), err, "Unexpected error getting `dep1` service status: %v", err)
	assert.Equal(s.T(), status.State, svc.Running, "`Dep1` service should be running")

	status, err = s.dep2Svc.Query()
	assert.Nil(s.T(), err, "Unexpected error getting `dep2` service status: %v", err)
	assert.Equal(s.T(), status.State, svc.Stopped, "`Dep2` service should be stopped")
}

func TestStartStopServiceRaceConditionTestSuite(t *testing.T) {
	suite.Run(t, new(StartStopServiceRaceConditionTestSuite))
}
