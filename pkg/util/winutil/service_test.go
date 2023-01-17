package winutil

import (
	"fmt"
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
	m, _ := OpenSCManager(windows.SC_MANAGER_CONNECT | windows.SC_MANAGER_CREATE_SERVICE)
	defer m.Disconnect()

	// test that OpenService returns an error with non-existent service
	_, err := OpenService(m, "nottestingservice", windows.SERVICE_START)
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

	nTestServices := 3

	// test that ListDependentServices returns an error if the service name is not valid
	_, err := ListDependentServices("notaservice", windows.SERVICE_ACTIVE)
	assert.NotNil(t, err, "Expected ListDependentServices to return an error with invalid service name")

	// open the SC manager so we can create some test services
	m, _ := OpenSCManager(windows.SC_MANAGER_CONNECT | windows.SC_MANAGER_CREATE_SERVICE)
	defer m.Disconnect()

	// install the test services
	for i := 0; i < nTestServices; i++ {

		// general config for all test services
		c := mgr.Config{
			StartType:   mgr.StartDisabled,
			DisplayName: "Test Service",
			Description: "This is a test service",
		}

		// all services will depend on the first
		if i != 0 {
			c.Dependencies = append(c.Dependencies, "service1")
		}

		// install the service
		serviceName := fmt.Sprintf("service%d", i+1)
		install(t, m, serviceName, "", c)
	}

	// test that ListDependentServices returns an empty list for service with no dependencies
	deps, err := ListDependentServices("service2", windows.SERVICE_STATE_ALL)
	assert.Nilf(t, err, "Unexpected error: %v", err)
	assert.Zero(t, len(deps), "")

	// test that ListDependentServices returns a list of dependent services for service with dependencies
	deps, err = ListDependentServices("service1", windows.SERVICE_STATE_ALL)
	assert.Nilf(t, err, "Unexpected error: %v", err)
	assert.Equal(t, 2, len(deps), "Expected ListDependentServices to return a list of dependent services")

	// the deps
	for _, dep := range deps {
		t.Logf("Dependent: %s", dep.serviceName)
	}

	// clean up test services
	for i := 0; i < nTestServices; i++ {
		serviceName := fmt.Sprintf("service%d", i+1)
		s, err := OpenService(m, serviceName, windows.SERVICE_START|windows.DELETE)
		assert.Nil(t, err, "Unexpected error: %v", err)
		assert.NotNil(t, s, "Expected OpenService to return a valid service handle")
		remove(t, s)
		s.Close()
	}
}
