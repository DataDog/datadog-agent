// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"
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
