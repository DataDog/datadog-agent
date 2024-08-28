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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type onStopServiceInvokeType int

const (
	onBeforeStopService onStopServiceInvokeType = iota
	onAfterEnumDependentServices
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

type StartStopServiceRaceConditionTestSuite struct {
	suite.Suite
	manager     *mgr.Mgr
	mainSvc     *mgr.Service
	dep1Svc     *mgr.Service
	dep2Svc     *mgr.Service
	mainSvcName string
	dep1SvcName string
	dep2SvcName string

	onInvokeStartService                bool
	onInvokeType                        onStopServiceInvokeType
	onInvokeWaitStartToComplete         bool
	onInvokeCount                       int
	onInvokeSkipCountBeforeStartService int
	onInvokeStartServiceTimes           int
	onInvokeNameOfStoppingService       string
	onInvokeNameOfStartingService       string
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
		windows.DELETE | windows.SERVICE_QUERY_STATUS | windows.SERVICE_ENUMERATE_DEPENDENTS)

	// SCM
	m, err := OpenSCManager(windows.SC_MANAGER_CONNECT | windows.SC_MANAGER_CREATE_SERVICE)
	assert.Nilf(s.T(), err, "Unexpected error: %v", err)
	if !assert.NotNil(s.T(), m, "Expected OpenSCManager to return a valid manager handle") {
		s.T().Fatalf("Failed to open Service Manager")
	}
	defer m.Disconnect()

	// keep read-only service manager for later use
	s.manager, err = OpenSCManager(windows.SC_MANAGER_CONNECT)
	assert.Nilf(s.T(), err, "Unexpected error: %v", err)
	assert.NotNil(s.T(), s.manager, "Expected OpenSCManager to return a valid manager handle")

	// Main service
	c := mgr.Config{
		StartType:   mgr.StartManual,
		DisplayName: fmt.Sprintf(displayNameTemp, "Main"),
		Description: fmt.Sprintf(descriptionNameTemp, "main"),
	}
	install(s.T(), m, s.mainSvcName, svcExePath, c)
	s.mainSvc, err = OpenService(m, s.mainSvcName, svcFlags)
	assert.Nil(s.T(), err, assertErrNilTempl, "main", err)
	if !assert.NotNil(s.T(), s.mainSvc, assertSvcNilTempl, "main") {
		s.T().Fatalf("Failed to open service %s", s.mainSvcName)
	}

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
	if !assert.NotNil(s.T(), s.mainSvc, assertSvcNilTempl, "dep1") {
		s.T().Fatalf("Failed to open service %s", s.dep1SvcName)
	}

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
	if !assert.NotNil(s.T(), s.mainSvc, assertSvcNilTempl, "dep2") {
		s.T().Fatalf("Failed to open service %s", s.dep1SvcName)
	}
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
	if s.manager != nil {
		s.manager.Disconnect()
	}
}

// nothing special to do
func (s *StartStopServiceRaceConditionTestSuite) SetupTest() {
	s.onInvokeStartService = false
	s.onInvokeType = onBeforeStopService
	s.onInvokeWaitStartToComplete = false
	s.onInvokeCount = 0
	s.onInvokeSkipCountBeforeStartService = 0
	s.onInvokeStartServiceTimes = 0
	s.onInvokeNameOfStoppingService = ""
	s.onInvokeNameOfStartingService = ""
}

// stop all services which may be running after each test
func (s *StartStopServiceRaceConditionTestSuite) TearDownTest() {
	StopService(s.dep2SvcName)
	StopService(s.dep1SvcName)
	StopService(s.mainSvcName)
}

func (s *StartStopServiceRaceConditionTestSuite) onStopServiceInvoke() {
	s.onInvokeCount++

	// if we should start service before is notificatthe check
	if (s.onInvokeCount >= s.onInvokeSkipCountBeforeStartService+1) &&
		s.onInvokeCount <= (s.onInvokeSkipCountBeforeStartService+s.onInvokeStartServiceTimes) {

		err := StartService(s.onInvokeNameOfStartingService)
		assert.Nil(s.T(), err, "Unexpected error of starting `%s` service: %v", s.onInvokeNameOfStartingService, err)
		if s.onInvokeWaitStartToComplete {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			err = WaitForState(ctx, s.onInvokeNameOfStartingService, svc.Running)
			assert.Nil(s.T(), err, "`%s` service should be running: %v", s.onInvokeNameOfStartingService, err)
		}
	}
}

// general callback to stop service to be used in setting up edge cases for race conditions
func (s *StartStopServiceRaceConditionTestSuite) beforeStopService(serviceName string) {
	// if we should start service before is notificatthe check
	if s.onInvokeStartService && s.onInvokeType == onBeforeStopService {
		if strings.Compare(serviceName, s.onInvokeNameOfStoppingService) == 0 {
			s.onStopServiceInvoke()
		}
	}
}

func (s *StartStopServiceRaceConditionTestSuite) afterDependentsEnumeration() {
	// if we should start service before is notificatthe check
	if s.onInvokeStartService && s.onInvokeType == onAfterEnumDependentServices {
		s.onStopServiceInvoke()
	}
}

func (s *StartStopServiceRaceConditionTestSuite) validateAllServicesStopped() {
	// VALIDATION: All  services should be stopped
	status, err := s.mainSvc.Query()
	assert.Nil(s.T(), err, "Unexpected error getting `main` service status: %v", err)
	assert.Equal(s.T(), status.State, svc.Stopped, "`Main` service should be stopped")

	status, err = s.dep1Svc.Query()
	assert.Nil(s.T(), err, "Unexpected error getting `dep1` service status: %v", err)
	assert.Equal(s.T(), status.State, svc.Stopped, "`Dep1` service should be stopped")

	status, err = s.dep2Svc.Query()
	assert.Nil(s.T(), err, "Unexpected error getting `dep2` service status: %v", err)
	assert.Equal(s.T(), status.State, svc.Stopped, "`Dep2` service should be stopped")
}

func (s *StartStopServiceRaceConditionTestSuite) startAllServices() {
	err := s.mainSvc.Start()
	assert.Nil(s.T(), err, "Failed to start `main` service: %v", err)
	err = s.dep1Svc.Start()
	assert.Nil(s.T(), err, "Failed to start `dep1` service: %v", err)
	err = s.dep2Svc.Start()
	assert.Nil(s.T(), err, "Failed to start `dep2` service: %v", err)
	ctx1, cancel1 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel1()
	err = WaitForState(ctx1, s.mainSvcName, svc.Running)
	assert.Nil(s.T(), err, "`Main` service should be running: %v", err)
	err = WaitForState(ctx1, s.dep1SvcName, svc.Running)
	assert.Nil(s.T(), err, "`Dep1` service should be running: %v", err)
	err = WaitForState(ctx1, s.dep2SvcName, svc.Running)
	assert.Nil(s.T(), err, "`Dep2` service should be running: %v", err)
}

func (s *StartStopServiceRaceConditionTestSuite) TestBasicRestartWhenNoSvcRunning() {
	err := RestartService(s.mainSvcName)
	assert.Nil(s.T(), err, "Main service should restar successfully: %v", err)

	var status svc.Status
	status, err = s.mainSvc.Query()
	assert.Nil(s.T(), err, "Main service should restar successfully: %v", err)
	assert.True(s.T(), status.State == svc.StartPending || status.State == svc.Running,
		"Main service should be starting or running: %v", status)
}

func (s *StartStopServiceRaceConditionTestSuite) TestBasicRestartWhenOnlyMainIsRunning() {
	// SETUP: Start main service
	err := s.mainSvc.Start()
	assert.Nil(s.T(), err, "Failed to start `main` service: %v", err)
	ctx1, cancel1 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel1()
	err = WaitForState(ctx1, s.mainSvcName, svc.Running)
	assert.Nil(s.T(), err, "`Main` service should be running successfully: %v", err)

	// TEST: Restart main service
	err = RestartService(s.mainSvcName)
	assert.Nil(s.T(), err, "`Main` service should restart successfully: %v", err)
	// VALIDATION
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()
	err = WaitForState(ctx2, s.mainSvcName, svc.Running)
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
	ctx1, cancel1 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel1()
	err = WaitForState(ctx1, s.mainSvcName, svc.Running)
	assert.Nil(s.T(), err, "`Main` service should be running: %v", err)
	err = WaitForState(ctx1, s.dep1SvcName, svc.Running)
	assert.Nil(s.T(), err, "`Dep1` service should be running: %v", err)
	err = WaitForState(ctx1, s.dep2SvcName, svc.Running)
	assert.Nil(s.T(), err, "`Dep2` service should be running: %v", err)

	// TEST: Restart only main service
	err = RestartService(s.mainSvcName)
	assert.Nil(s.T(), err, "Main service should restart successfully: %v", err)

	// VALIDATION: Two dependent services should be stopped because nobody
	// starts them like "datadogagent" service does). But they both should not
	// be running and it will be indication that main service was able to be
	// restarted.
	var status svc.Status
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()
	err = WaitForState(ctx2, s.mainSvcName, svc.Running)
	assert.Nil(s.T(), err, "Unexpected error of making `main` service running: %v", err)

	status, err = s.dep1Svc.Query()
	assert.Nil(s.T(), err, "Unexpected error getting `dep1` service status: %v", err)
	assert.Equal(s.T(), status.State, svc.Stopped, "`Dep1` service should be stopped")

	status, err = s.dep2Svc.Query()
	assert.Nil(s.T(), err, "Unexpected error getting `dep2` service status: %v", err)
	assert.Equal(s.T(), status.State, svc.Stopped, "`Dep2` service should be stopped")
}

func (s *StartStopServiceRaceConditionTestSuite) TestStopMainWhenDependentServiceStartingAfterEnumeration() {
	// SETUP: Start only main service
	err := s.mainSvc.Start()
	assert.Nil(s.T(), err, "Failed to start `main` service: %v", err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = WaitForState(ctx, s.mainSvcName, svc.Running)
	assert.Nil(s.T(), err, "`Main` service should be running: %v", err)

	// Instruct to start `dep1` service after dependency enumeration
	// and just before it will be attempted to be started.
	// Note: Do not wait to `dep1` service to be started before proceeding
	// with the callback, menaing doStopServiceWithDependencies will continue
	// with stopping `dep1` service when it is starting
	s.onInvokeStartService = true
	s.onInvokeWaitStartToComplete = false
	s.onInvokeType = onBeforeStopService
	s.onInvokeSkipCountBeforeStartService = 0
	s.onInvokeStartServiceTimes = 1
	s.onInvokeNameOfStoppingService = s.dep1Svc.Name
	s.onInvokeNameOfStartingService = s.dep1Svc.Name

	// Call stop service with dependencies
	err = doStopServiceWithDependencies(s.manager, s.mainSvc, svc.AnyActivity, s)
	assert.Nil(s.T(), err, "Main service should restart successfully: %v", err)

	s.validateAllServicesStopped()
}

func (s *StartStopServiceRaceConditionTestSuite) TestStopMainWhenDependentServiceStartedAfterEnumeration() {
	// SETUP: Start main service only
	err := s.mainSvc.Start()
	assert.Nil(s.T(), err, "Failed to start `main` service: %v", err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = WaitForState(ctx, s.mainSvcName, svc.Running)
	assert.Nil(s.T(), err, "`Main` service should be running: %v", err)

	// Instruct to start `dep1` service after dependency enumeration
	// and just before it will be attempted to be started.
	// Note: Wiat for `dep1` service to be started before proceeding (in contrast with test above)
	// with the callback, menaing doStopServiceWithDependencies will continue
	// with `dep1` service is already  running
	s.onInvokeStartService = true
	s.onInvokeWaitStartToComplete = true
	s.onInvokeType = onBeforeStopService
	s.onInvokeSkipCountBeforeStartService = 0
	s.onInvokeStartServiceTimes = 1
	s.onInvokeNameOfStoppingService = s.dep1Svc.Name
	s.onInvokeNameOfStartingService = s.dep1Svc.Name

	err = doStopServiceWithDependencies(s.manager, s.mainSvc, svc.AnyActivity, s)
	assert.Nil(s.T(), err, "Main service should restart successfully: %v", err)

	s.validateAllServicesStopped()
}

func (s *StartStopServiceRaceConditionTestSuite) TestStopMainWhenDependentServiceStillRunningAfterAttemptsToStopThem() {
	s.startAllServices()

	// Instruct to start `dep1` service after stopping all dependent service
	// and just before stopping main service on first iteration to fail stopping
	// main service and start second iteration which should succeed
	s.onInvokeStartService = true
	s.onInvokeWaitStartToComplete = true
	s.onInvokeType = onBeforeStopService
	s.onInvokeSkipCountBeforeStartService = 0
	s.onInvokeStartServiceTimes = 1
	s.onInvokeNameOfStoppingService = s.mainSvcName
	s.onInvokeNameOfStartingService = s.dep1SvcName

	err := doStopServiceWithDependencies(s.manager, s.mainSvc, svc.Active, s)
	assert.Nil(s.T(), err, "Main service stop should succeed")
	assert.Equal(s.T(), 2, s.onInvokeCount, "Should have 2 iteration, first failed second succeeded")

	s.validateAllServicesStopped()
}

// The same as above except have 3 iterations to stop main services
// First two iterations should file and third should succeed
func (s *StartStopServiceRaceConditionTestSuite) TestStopMainWhenDependentServiceStillRunningAfterAttemptsToStopThemTwice() {
	s.startAllServices()

	// Instruct to start `dep1` service after stopping all dependent service
	// and just before stopping main service on first iteration to fail stopping
	// main service and start second iteration which should succeed
	s.onInvokeStartService = true
	s.onInvokeWaitStartToComplete = true
	s.onInvokeType = onBeforeStopService
	s.onInvokeSkipCountBeforeStartService = 0
	s.onInvokeStartServiceTimes = 2
	s.onInvokeNameOfStoppingService = s.mainSvcName
	s.onInvokeNameOfStartingService = s.dep1SvcName

	err := doStopServiceWithDependencies(s.manager, s.mainSvc, svc.Active, s)
	assert.Nil(s.T(), err, "Main service stop should succeed")
	assert.Equal(s.T(), 3, s.onInvokeCount, "Should have 2 iteration, first failed second succeeded")

	s.validateAllServicesStopped()
}

// This test is similar to the previous one, but it tests the old code path
// which should fail as a proof that we have fixed the problem not acidently
func (s *StartStopServiceRaceConditionTestSuite) TestStopMainWhenDependentServiceStartedAfterEnumerationShouldFailInOldCode() {
	// Disable skip to run this test if needed to observe old code behavior (it is a waste of time otherwise)
	s.T().Skip("skipping test which useful only to prove that old code logic would fail even after small code modification")

	// SETUP: Start main service only
	err := s.mainSvc.Start()
	assert.Nil(s.T(), err, "Failed to start `main` service: %v", err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = WaitForState(ctx, s.mainSvcName, svc.Running)
	assert.Nil(s.T(), err, "`Main` service should be running: %v", err)

	// Instruct to start `dep1` service IMMEDIATELY after dependency enumeration
	// This test is not needed all the time, it simulate common race condition
	// which had been allowed in the old code path
	s.onInvokeStartService = true
	s.onInvokeWaitStartToComplete = true
	s.onInvokeType = onAfterEnumDependentServices
	s.onInvokeSkipCountBeforeStartService = 0
	s.onInvokeStartServiceTimes = 1
	s.onInvokeNameOfStoppingService = s.dep1Svc.Name
	s.onInvokeNameOfStartingService = s.dep1Svc.Name

	err = doStopServiceWithDependencies(s.manager, s.mainSvc, svc.Active, s)
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

// Test start a service after it has been just started. This is to test
// that starting a service when it is in START_PENDING state is handled correctly
func (s *StartStopServiceRaceConditionTestSuite) TestStartServiceAfterStartService() {
	// SETUP: Start main service only
	err := s.mainSvc.Start()
	assert.Nil(s.T(), err, "Failed to start `main` service: %v", err)

	// Immediately start it again, since main service most likely will be in
	// the start pending state at the moment of the second start. It should start
	// successfully without any error
	err = StartService(s.mainSvcName)
	assert.Nil(s.T(), err, "`Main` service should start successfully: %v", err)
}

// Test start a service after it has been just stopped. This is to test
// that starting a service when it is in STOP_PENDING state is handled correctly
func (s *StartStopServiceRaceConditionTestSuite) TestStartServiceAfterStopService() {
	// SETUP: Start, wait for starting and stop externally main service
	// to make sure it is in the stop pending state
	err := s.mainSvc.Start()
	assert.Nil(s.T(), err, "Failed to start `main` service: %v", err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = WaitForState(ctx, s.mainSvcName, svc.Running)
	assert.Nil(s.T(), err, "`Main` service should be running: %v", err)
	_, err = s.mainSvc.Control(svc.Stop)
	assert.Nil(s.T(), err, "`Main` service should be stopping: %v", err)

	// Immediately start it again, since main service most likely will be in
	// the stop pending state at the moment of the second start. It should start
	// successfully without any error
	err = StartService(s.mainSvcName)
	assert.Nil(s.T(), err, "`Main` service should start successfully: %v", err)
}

// Test stop a service after it has been just started. This is to test
// that stopping a service when it is in START_PENDING state is handled correctly
func (s *StartStopServiceRaceConditionTestSuite) TestStopServiceAfterStartService() {
	// SETUP: Start main service only
	err := s.mainSvc.Start()
	assert.Nil(s.T(), err, "Failed to start `main` service: %v", err)

	// Immediately stop it, since main service most likely will be in
	// the start pending state at the moment of the start. It should stop
	// successfully without any error
	err = StopService(s.mainSvcName)
	assert.Nil(s.T(), err, "`Main` service should stop successfully: %v", err)
}

// Test stop a service after it has been just stopped. This is to test
// that stopping a service when it is in STOP_PENDING state is handled correctly
func (s *StartStopServiceRaceConditionTestSuite) TestStopServiceAfterStopService() {
	// SETUP: Start, wait for starting and stop externally main service
	// to make sure it is in the stop pending state
	err := s.mainSvc.Start()
	assert.Nil(s.T(), err, "Failed to start `main` service: %v", err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = WaitForState(ctx, s.mainSvcName, svc.Running)
	assert.Nil(s.T(), err, "`Main` service should be running: %v", err)
	_, err = s.mainSvc.Control(svc.Stop)
	assert.Nil(s.T(), err, "`Main` service should be stopping: %v", err)

	// Immediately stop it, since main service most likely will be in
	// the stop pending state at the moment of the second stop. It should stop
	// successfully without any error
	err = StopService(s.mainSvcName)
	assert.Nil(s.T(), err, "`Main` service should stop successfully: %v", err)
}

func TestStartStopServiceRaceConditionTestSuite(t *testing.T) {
	suite.Run(t, new(StartStopServiceRaceConditionTestSuite))
}
