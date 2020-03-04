// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build systemd

package systemd

import (
	"fmt"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/coreos/go-systemd/dbus"
	godbus "github.com/godbus/dbus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockSystemdStats struct {
	mock.Mock
}

func createDefaultMockSystemdStats() *mockSystemdStats {
	stats := &mockSystemdStats{}
	stats.On("PrivateSocketConnection", mock.Anything).Return(&dbus.Conn{}, nil)
	stats.On("SystemBusSocketConnection").Return(&dbus.Conn{}, nil)
	stats.On("SystemState", mock.Anything).Return(&dbus.Property{Name: "SystemState", Value: godbus.MakeVariant("running")}, nil)
	return stats
}

func (s *mockSystemdStats) PrivateSocketConnection(privateSocket string) (*dbus.Conn, error) {
	args := s.Mock.Called(privateSocket)
	return args.Get(0).(*dbus.Conn), args.Error(1)
}

func (s *mockSystemdStats) SystemBusSocketConnection() (*dbus.Conn, error) {
	args := s.Mock.Called()
	return args.Get(0).(*dbus.Conn), args.Error(1)
}

func (s *mockSystemdStats) SystemState(conn *dbus.Conn) (*dbus.Property, error) {
	args := s.Mock.Called(conn)
	return args.Get(0).(*dbus.Property), args.Error(1)
}

func (s *mockSystemdStats) CloseConn(c *dbus.Conn) {
}

func (s *mockSystemdStats) ListUnits(conn *dbus.Conn) ([]dbus.UnitStatus, error) {
	args := s.Mock.Called(conn)
	return args.Get(0).([]dbus.UnitStatus), args.Error(1)
}

func (s *mockSystemdStats) UnixNow() int64 {
	args := s.Mock.Called()
	return args.Get(0).(int64)
}

func (s *mockSystemdStats) GetUnitTypeProperties(conn *dbus.Conn, unitName string, unitType string) (map[string]interface{}, error) {
	args := s.Mock.Called(conn, unitName, unitType)
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func getCreatePropertieWithDefaults(props map[string]interface{}) map[string]interface{} {
	defaultProps := map[string]interface{}{
		"CPUAccounting":    true,
		"MemoryAccounting": true,
		"TasksAccounting":  true,
	}
	for k, v := range props {
		defaultProps[k] = v
	}
	return defaultProps
}

func TestBasicConfiguration(t *testing.T) {
	check := SystemdCheck{}
	rawInstanceConfig := []byte(`
unit_names:
 - ssh.service
 - syslog.socket
`)
	err := check.Configure(rawInstanceConfig, []byte(``), "test")

	assert.Nil(t, err)
	assert.ElementsMatch(t, []string{"ssh.service", "syslog.socket"}, check.config.instance.UnitNames)
}

func TestMissingUnitNamesShouldRaiseError(t *testing.T) {
	check := SystemdCheck{}
	err := check.Configure([]byte(``), []byte(``), "test")

	expectedErrorMsg := "instance config `unit_names` must not be empty"
	assert.EqualError(t, err, expectedErrorMsg)
}

func TestInvalidSubStateMappingName(t *testing.T) {
	check := SystemdCheck{}
	rawInstanceConfig := []byte(`
unit_names:
- foo
substate_status_mapping:
  bar:
    exited: critical
    running: ok
`)
	err := check.Configure(rawInstanceConfig, []byte(``), "test")

	expectedErrorMsg := "instance config specifies a custom substate mapping for unit 'bar' but this unit is not monitored. Please add 'bar' to 'unit_names'"
	assert.EqualError(t, err, expectedErrorMsg)
}

func TestInvalidSubStateMapping(t *testing.T) {
	check := SystemdCheck{}
	rawInstanceConfig := []byte(`
unit_names:
- foo
substate_status_mapping:
  foo:
    running: ok
    exited: Critical
`)
	err := check.Configure(rawInstanceConfig, []byte(``), "test")

	expectedErrorMsg := "Status 'Critical' for unit 'foo' in 'substate_status_mapping' is invalid. It should be one of 'ok, warning, critical, unknown'"
	assert.EqualError(t, err, expectedErrorMsg)
}

func TestValidSubStateMapping(t *testing.T) {
	check := SystemdCheck{}
	rawInstanceConfig := []byte(`
unit_names:
- foo
- bar
- baz
substate_status_mapping:
  foo:
    running: ok
    exited: warning
    mounted: unknown
  bar:
    exited: critical
    plugged: ok
    running: ok
`)
	err := check.Configure(rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)
}

func TestPrivateSocketConnection(t *testing.T) {
	stats := &mockSystemdStats{}
	stats.On("PrivateSocketConnection", mock.Anything).Return(&dbus.Conn{}, nil)

	rawInstanceConfig := []byte(`
unit_names:
- ssh.service
private_socket: /tmp/foo/private_socket
`)
	check := SystemdCheck{stats: stats}
	check.Configure(rawInstanceConfig, []byte(``), "test")
	conn, err := check.getDbusConnection()

	assert.Nil(t, err)
	assert.NotNil(t, conn)
	stats.AssertCalled(t, "PrivateSocketConnection", "/tmp/foo/private_socket")
	stats.AssertNotCalled(t, "SystemBusSocketConnection")
}

func TestPrivateSocketConnectionErrorCase(t *testing.T) {
	stats := &mockSystemdStats{}
	stats.On("PrivateSocketConnection", mock.Anything).Return((*dbus.Conn)(nil), fmt.Errorf("some error"))

	rawInstanceConfig := []byte(`
unit_names:
- ssh.service
private_socket: /tmp/foo/private_socket
`)
	check := SystemdCheck{stats: stats}
	check.Configure(rawInstanceConfig, []byte(``), "test")
	conn, err := check.getDbusConnection()

	assert.EqualError(t, err, "some error")
	assert.Nil(t, conn)
	stats.AssertCalled(t, "PrivateSocketConnection", "/tmp/foo/private_socket")
	stats.AssertNotCalled(t, "SystemBusSocketConnection")
}

func TestDefaultPrivateSocketConnection(t *testing.T) {
	stats := &mockSystemdStats{}
	stats.On("SystemBusSocketConnection").Return((*dbus.Conn)(nil), fmt.Errorf("some error"))
	stats.On("PrivateSocketConnection", mock.Anything).Return(&dbus.Conn{}, nil)

	rawInstanceConfig := []byte(`
unit_names:
- ssh.service
`)
	check := SystemdCheck{stats: stats}
	check.Configure(rawInstanceConfig, []byte(``), "test")
	conn, err := check.getDbusConnection()

	assert.Nil(t, err)
	assert.NotNil(t, conn)
	stats.AssertCalled(t, "SystemBusSocketConnection")
	stats.AssertCalled(t, "PrivateSocketConnection", "/run/systemd/private")
}

func TestDefaultSystemBusSocketConnection(t *testing.T) {
	stats := &mockSystemdStats{}
	stats.On("SystemBusSocketConnection").Return(&dbus.Conn{}, nil)

	rawInstanceConfig := []byte(`
unit_names:
- ssh.service
`)
	check := SystemdCheck{stats: stats}
	check.Configure(rawInstanceConfig, []byte(``), "test")
	conn, err := check.getDbusConnection()

	assert.Nil(t, err)
	assert.NotNil(t, conn)
	stats.AssertCalled(t, "SystemBusSocketConnection")
	stats.AssertNotCalled(t, "PrivateSocketConnection", "/run/systemd/private")
}

func TestDefaultDockerAgentPrivateSocketConnection(t *testing.T) {
	os.Setenv("DOCKER_DD_AGENT", "true")
	defer os.Unsetenv("DOCKER_DD_AGENT")

	stats := &mockSystemdStats{}
	stats.On("PrivateSocketConnection", mock.Anything).Return(&dbus.Conn{}, nil)

	rawInstanceConfig := []byte(`
unit_names:
- ssh.service
`)
	check := SystemdCheck{stats: stats}
	check.Configure(rawInstanceConfig, []byte(``), "test")
	conn, err := check.getDbusConnection()

	assert.Nil(t, err)
	assert.NotNil(t, conn)
	stats.AssertCalled(t, "PrivateSocketConnection", "/host/run/systemd/private")
	stats.AssertNotCalled(t, "SystemBusSocketConnection")
}

func TestDefaultDockerAgentSystemBusSocketConnectionNotCalled(t *testing.T) {
	os.Setenv("DOCKER_DD_AGENT", "true")
	defer os.Unsetenv("DOCKER_DD_AGENT")
	stats := &mockSystemdStats{}
	stats.On("PrivateSocketConnection", mock.Anything).Return((*dbus.Conn)(nil), fmt.Errorf("some error"))
	stats.On("SystemBusSocketConnection").Return(&dbus.Conn{}, nil)

	rawInstanceConfig := []byte(`
unit_names:
- ssh.service
`)
	check := SystemdCheck{stats: stats}
	check.Configure(rawInstanceConfig, []byte(``), "test")
	conn, err := check.getDbusConnection()

	assert.NotNil(t, err)
	assert.Nil(t, conn)
	stats.AssertCalled(t, "PrivateSocketConnection", "/host/run/systemd/private")
	stats.AssertNotCalled(t, "SystemBusSocketConnection")
}

func TestDbusConnectionErr(t *testing.T) {
	stats := &mockSystemdStats{}
	stats.On("PrivateSocketConnection", mock.Anything).Return((*dbus.Conn)(nil), fmt.Errorf("some error"))
	stats.On("SystemBusSocketConnection").Return((*dbus.Conn)(nil), fmt.Errorf("some error"))

	check := SystemdCheck{stats: stats}
	check.Configure([]byte(``), []byte(``), "test")

	mockSender := mocksender.NewMockSender(check.ID()) // required to initiate aggregator
	mockSender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	err := check.Run()

	expectedErrorMsg := "cannot create a connection: some error"
	assert.EqualError(t, err, expectedErrorMsg)
	mockSender.AssertCalled(t, "ServiceCheck", canConnectServiceCheck, metrics.ServiceCheckCritical, "", []string(nil), expectedErrorMsg)

}

func TestSystemStateCallFailGracefully(t *testing.T) {
	stats := &mockSystemdStats{}
	stats.On("SystemBusSocketConnection").Return(&dbus.Conn{}, nil)
	stats.On("SystemState", mock.Anything).Return((*dbus.Property)(nil), fmt.Errorf("some error"))
	stats.On("ListUnits", mock.Anything).Return([]dbus.UnitStatus{}, nil)

	check := SystemdCheck{stats: stats}
	check.Configure([]byte(``), []byte(``), "test")

	mockSender := mocksender.NewMockSender(check.ID()) // required to initiate aggregator
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err := check.Run()
	assert.Nil(t, err)

	mockSender.AssertNotCalled(t, "ServiceCheck")
}

func TestListUnitErr(t *testing.T) {
	stats := createDefaultMockSystemdStats()
	stats.On("ListUnits", mock.Anything).Return(([]dbus.UnitStatus)(nil), fmt.Errorf("some error"))

	check := SystemdCheck{stats: stats}
	check.Configure([]byte(``), []byte(``), "test")

	mockSender := mocksender.NewMockSender(check.ID()) // required to initiate aggregator
	mockSender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	err := check.Run()

	expectedErrorMsg := "error getting list of units: some error"
	assert.EqualError(t, err, expectedErrorMsg)
}

func TestCountMetrics(t *testing.T) {
	stats := createDefaultMockSystemdStats()
	stats.On("ListUnits", mock.Anything).Return([]dbus.UnitStatus{
		{Name: "unit1.service", ActiveState: "active", LoadState: "loaded"},
		{Name: "unit2.service", ActiveState: "active", LoadState: "loaded"},
		{Name: "unit3.service", ActiveState: "active", LoadState: "loaded"},
		{Name: "unit4.service", ActiveState: "inactive", LoadState: "not-found"},
		{Name: "unit5.service", ActiveState: "inactive", LoadState: "not-found"},
		{Name: "unit6.service", ActiveState: "activating", LoadState: "loaded"},
		{Name: "unit7.service", ActiveState: "deactivating", LoadState: "loaded"},
		{Name: "unit8.service", ActiveState: "failed", LoadState: "loaded"},
	}, nil)

	stats.On("GetUnitTypeProperties", mock.Anything, mock.Anything, dbusTypeMap[typeService]).Return(map[string]interface{}{
		"ActiveEnterTimestamp": uint64(1),
	}, nil)

	rawInstanceConfig := []byte(`
unit_names:
 - unit1.service
 - unit2.service
`)
	check := SystemdCheck{stats: stats}
	check.Configure(rawInstanceConfig, nil, "test")

	// setup expectations
	stats.On("GetUnitTypeProperties", mock.Anything, mock.Anything, mock.Anything).Return(map[string]interface{}{}, nil)

	mockSender := mocksender.NewMockSender(check.ID())
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	// run
	err := check.Run()
	assert.Nil(t, err)

	// assertions
	mockSender.AssertCalled(t, "ServiceCheck", canConnectServiceCheck, metrics.ServiceCheckOK, "", []string(nil), mock.Anything)
	mockSender.AssertCalled(t, "ServiceCheck", systemStateServiceCheck, metrics.ServiceCheckOK, "", []string(nil), mock.Anything)
	mockSender.AssertCalled(t, "Gauge", "systemd.units_loaded_count", float64(6), "", []string(nil))
	mockSender.AssertCalled(t, "Gauge", "systemd.units_monitored_count", float64(2), "", []string(nil))
	mockSender.AssertCalled(t, "Gauge", "systemd.units_total", float64(8), "", []string(nil))
	mockSender.AssertCalled(t, "Gauge", "systemd.units_by_state", float64(3), "", []string{"state:" + "active"})
	mockSender.AssertCalled(t, "Gauge", "systemd.units_by_state", float64(1), "", []string{"state:" + "activating"})
	mockSender.AssertCalled(t, "Gauge", "systemd.units_by_state", float64(2), "", []string{"state:" + "inactive"})
	mockSender.AssertCalled(t, "Gauge", "systemd.units_by_state", float64(1), "", []string{"state:" + "deactivating"})
	mockSender.AssertCalled(t, "Gauge", "systemd.units_by_state", float64(1), "", []string{"state:" + "failed"})
}

func TestMetricValues(t *testing.T) {
	rawInstanceConfig := []byte(`
unit_names:
 - unit1.service
 - unit2.service
`)

	stats := createDefaultMockSystemdStats()
	stats.On("ListUnits", mock.Anything).Return([]dbus.UnitStatus{
		{Name: "unit1.service", ActiveState: "active", LoadState: "loaded"},
		{Name: "unit2.service", ActiveState: "active", LoadState: "loaded"},
	}, nil)
	stats.On("UnixNow").Return(int64(1000))
	stats.On("GetUnitTypeProperties", mock.Anything, "unit1.service", dbusTypeMap[typeService]).Return(getCreatePropertieWithDefaults(map[string]interface{}{
		"CPUUsageNSec":  uint64(10),
		"MemoryCurrent": uint64(20),
		"TasksCurrent":  uint64(30),
		"NRestarts":     uint64(40),
	}), nil)
	stats.On("GetUnitTypeProperties", mock.Anything, "unit2.service", dbusTypeMap[typeService]).Return(getCreatePropertieWithDefaults(map[string]interface{}{
		"CPUUsageNSec":  uint64(110),
		"MemoryCurrent": uint64(120),
		"TasksCurrent":  uint64(130),
		"NRestarts":     uint64(140),
	}), nil)
	stats.On("GetUnitTypeProperties", mock.Anything, "unit1.service", dbusTypeMap[typeUnit]).Return(map[string]interface{}{
		"ActiveEnterTimestamp": uint64(100 * 1000 * 1000),
	}, nil)
	stats.On("GetUnitTypeProperties", mock.Anything, "unit2.service", dbusTypeMap[typeUnit]).Return(map[string]interface{}{
		"ActiveEnterTimestamp": uint64(100 * 1000 * 1000),
	}, nil)

	check := SystemdCheck{stats: stats}
	check.Configure(rawInstanceConfig, nil, "test")

	// setup expectation
	mockSender := mocksender.NewMockSender(check.ID())
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	// run
	check.Run()

	// assertions
	tags := []string{"unit:unit1.service"}
	mockSender.AssertCalled(t, "Gauge", "systemd.unit.uptime", float64(900), "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.unit.monitored", float64(1), "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.unit.active", float64(1), "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.unit.loaded", float64(1), "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.service.cpu_time_consumed", float64(10), "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.service.memory_usage", float64(20), "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.service.task_count", float64(30), "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.service.restart_count", float64(40), "", tags)

	tags = []string{"unit:unit2.service"}
	mockSender.AssertCalled(t, "Gauge", "systemd.service.cpu_time_consumed", float64(110), "", tags)

	expectedGaugeCalls := 8     /* overall metrics */
	expectedGaugeCalls += 2 * 8 /* unit/service metrics */
	mockSender.AssertNumberOfCalls(t, "Gauge", expectedGaugeCalls)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 4)
}

func TestSubmitMetricsConditionals(t *testing.T) {
	rawInstanceConfig := []byte(`
unit_names:
 - unit1.service
 - unit2.service
 - unit3.service
 - unit5.socket
`)

	stats := createDefaultMockSystemdStats()
	stats.On("ListUnits", mock.Anything).Return([]dbus.UnitStatus{
		{Name: "unit1.service", ActiveState: "active", LoadState: "loaded"},
		{Name: "unit2.service", ActiveState: "inactive", LoadState: "not-loaded"},
		{Name: "unit3.service", ActiveState: "failed", LoadState: "loaded"},
		{Name: "unit4.service", ActiveState: "active", LoadState: "loaded"},
		{Name: "unit5.socket", ActiveState: "active", LoadState: "loaded"},
	}, nil)
	stats.On("UnixNow").Return(int64(1))
	stats.On("GetUnitTypeProperties", mock.Anything, mock.Anything, dbusTypeMap[typeService]).Return(getCreatePropertieWithDefaults(map[string]interface{}{
		"CPUUsageNSec":  uint64(1),
		"MemoryCurrent": uint64(1),
		"TasksCurrent":  uint64(1),
		"NRestarts":     uint64(1),
	}), nil)
	stats.On("GetUnitTypeProperties", mock.Anything, mock.Anything, dbusTypeMap[typeSocket]).Return(getCreatePropertieWithDefaults(map[string]interface{}{
		"NAccepted":    uint64(1),
		"NConnections": uint64(1),
		"NRefused":     uint64(1),
	}), nil)
	stats.On("GetUnitTypeProperties", mock.Anything, mock.Anything, dbusTypeMap[typeUnit]).Return(map[string]interface{}{
		"ActiveEnterTimestamp": uint64(1),
	}, nil)

	check := SystemdCheck{stats: stats}
	check.Configure(rawInstanceConfig, nil, "test")

	// setup expectation
	mockSender := mocksender.NewMockSender(check.ID())
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	// run
	check.Run()

	// assertions
	tags := []string{"unit:unit1.service"}
	mockSender.AssertCalled(t, "ServiceCheck", unitStateServiceCheck, metrics.ServiceCheckOK, "", tags, "")
	mockSender.AssertCalled(t, "Gauge", "systemd.unit.active", float64(1), "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.unit.loaded", float64(1), "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.unit.uptime", mock.Anything, "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.service.cpu_time_consumed", mock.Anything, "", tags)

	tags = []string{"unit:unit2.service"}
	mockSender.AssertCalled(t, "ServiceCheck", unitStateServiceCheck, metrics.ServiceCheckCritical, "", tags, "")
	mockSender.AssertCalled(t, "Gauge", "systemd.unit.uptime", mock.Anything, "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.unit.active", float64(0), "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.unit.loaded", float64(0), "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.service.cpu_time_consumed", mock.Anything, "", tags)

	tags = []string{"unit:unit3.service"}
	mockSender.AssertCalled(t, "ServiceCheck", unitStateServiceCheck, metrics.ServiceCheckCritical, "", tags, "")
	mockSender.AssertCalled(t, "Gauge", "systemd.service.cpu_time_consumed", mock.Anything, "", tags)

	tags = []string{"unit:unit4.service"}
	mockSender.AssertNotCalled(t, "ServiceCheck", unitStateServiceCheck, metrics.ServiceCheckCritical, "", tags, "")
	mockSender.AssertNotCalled(t, "Gauge", "systemd.service.cpu_time_consumed", mock.Anything, "", tags)

	tags = []string{"unit:unit5.socket"}
	mockSender.AssertCalled(t, "Gauge", "systemd.socket.connection_accepted_count", mock.Anything, "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.socket.connection_count", mock.Anything, "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.socket.connection_refused_count", mock.Anything, "", tags)
}

func TestSubmitMonitoredServiceMetrics(t *testing.T) {
	rawInstanceConfig := []byte(`
unit_names:
 - unit1.service
 - unit2.service
`)

	stats := createDefaultMockSystemdStats()
	stats.On("ListUnits", mock.Anything).Return([]dbus.UnitStatus{
		{Name: "unit1.service", ActiveState: "active"},
		{Name: "unit2.service", ActiveState: "active"},
	}, nil)
	stats.On("UnixNow").Return(int64(1000 * 1000))
	stats.On("GetUnitTypeProperties", mock.Anything, mock.Anything, dbusTypeMap[typeUnit]).Return(map[string]interface{}{}, nil)

	stats.On("GetUnitTypeProperties", mock.Anything, "unit1.service", dbusTypeMap[typeService]).Return(map[string]interface{}{
		"CPUUsageNSec":     uint64(1),
		"CPUAccounting":    true,
		"MemoryCurrent":    uint64(1),
		"MemoryAccounting": true,
		"TasksCurrent":     uint64(1),
		"TasksAccounting":  true,
		"NRestarts":        uint32(1),
	}, nil)

	stats.On("GetUnitTypeProperties", mock.Anything, "unit2.service", dbusTypeMap[typeService]).Return(map[string]interface{}{
		"CPUUsageNSec":     uint64(1),
		"CPUAccounting":    true,
		"MemoryCurrent":    uint64(1),
		"MemoryAccounting": false,
		"TasksCurrent":     uint64(1),
	}, nil)

	check := SystemdCheck{stats: stats}
	check.Configure(rawInstanceConfig, nil, "test")

	// setup expectation
	mockSender := mocksender.NewMockSender(check.ID())
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	// run
	check.Run()

	// assertions
	mockSender.AssertCalled(t, "ServiceCheck", canConnectServiceCheck, metrics.ServiceCheckOK, "", []string(nil), mock.Anything)

	tags := []string{"unit:unit1.service"}
	mockSender.AssertCalled(t, "Gauge", "systemd.service.cpu_time_consumed", mock.Anything, "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.service.memory_usage", mock.Anything, "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.service.task_count", mock.Anything, "", tags)
	mockSender.AssertCalled(t, "Gauge", "systemd.service.restart_count", mock.Anything, "", tags)

	tags = []string{"unit:unit2.service"}
	mockSender.AssertCalled(t, "Gauge", "systemd.service.cpu_time_consumed", mock.Anything, "", tags)
	mockSender.AssertNotCalled(t, "Gauge", "systemd.service.memory_usage", mock.Anything, "", tags)
	mockSender.AssertNotCalled(t, "Gauge", "systemd.service.task_count", mock.Anything, "", tags)
}

func TestServiceCheckSystemStateAndCanConnect(t *testing.T) {
	data := []struct {
		systemStatus               interface{}
		expectedServiceCheckStatus metrics.ServiceCheckStatus
		expectedMessage            string
	}{
		{"initializing", metrics.ServiceCheckUnknown, "Systemd status is \"initializing\""},
		{"starting", metrics.ServiceCheckUnknown, "Systemd status is \"starting\""},
		{"running", metrics.ServiceCheckOK, "Systemd status is \"running\""},
		{"degraded", metrics.ServiceCheckCritical, "Systemd status is \"degraded\""},
		{"maintenance", metrics.ServiceCheckCritical, "Systemd status is \"maintenance\""},
		{"stopping", metrics.ServiceCheckCritical, "Systemd status is \"stopping\""},
		{999, metrics.ServiceCheckUnknown, "Systemd status is 999"},
	}
	for _, d := range data {
		t.Run(fmt.Sprintf("state %s should be mapped to %s", d.systemStatus, d.expectedServiceCheckStatus.String()), func(t *testing.T) {
			stats := &mockSystemdStats{}
			stats.On("SystemBusSocketConnection").Return(&dbus.Conn{}, nil)
			stats.On("SystemState", mock.Anything).Return(&dbus.Property{Name: "SystemState", Value: godbus.MakeVariant(d.systemStatus)}, nil)
			stats.On("ListUnits", mock.Anything).Return([]dbus.UnitStatus{}, nil)

			check := SystemdCheck{stats: stats}
			check.Configure([]byte(``), []byte(``), "test")

			mockSender := mocksender.NewMockSender(check.ID()) // required to initiate aggregator
			mockSender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			mockSender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			mockSender.On("Commit").Return()

			err := check.Run()
			assert.NoError(t, err)

			mockSender.AssertCalled(t, "ServiceCheck", canConnectServiceCheck, metrics.ServiceCheckOK, "", []string(nil), "")
			mockSender.AssertCalled(t, "ServiceCheck", systemStateServiceCheck, d.expectedServiceCheckStatus, "", []string(nil), d.expectedMessage)
		})
	}
}

func TestServiceCheckUnitState(t *testing.T) {
	rawInstanceConfig := []byte(`
unit_names:
 - unit1.service
 - unit2.service
`)

	stats := createDefaultMockSystemdStats()
	stats.On("ListUnits", mock.Anything).Return([]dbus.UnitStatus{
		{Name: "unit1.service", ActiveState: "active"},
		{Name: "unit2.service", ActiveState: "inactive"},
		{Name: "unit3.service", ActiveState: "active"},
	}, nil)
	stats.On("UnixNow").Return(int64(1000 * 1000))

	stats.On("GetUnitTypeProperties", mock.Anything, mock.Anything, dbusTypeMap[typeService]).Return(map[string]interface{}{
		"CPUUsageNSec":  uint64(1),
		"MemoryCurrent": uint64(1),
		"TasksCurrent":  uint64(1),
	}, nil)

	stats.On("GetUnitTypeProperties", mock.Anything, "unit1.service", dbusTypeMap[typeUnit]).Return(map[string]interface{}{
		"ActiveEnterTimestamp": uint64(100),
	}, nil)
	stats.On("GetUnitTypeProperties", mock.Anything, "unit2.service", dbusTypeMap[typeUnit]).Return(map[string]interface{}{
		"ActiveEnterTimestamp": uint64(200),
	}, nil)

	check := SystemdCheck{stats: stats}
	check.Configure(rawInstanceConfig, nil, "test")

	// setup expectation
	mockSender := mocksender.NewMockSender(check.ID())
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	// run
	check.Run()

	// assertions
	mockSender.AssertCalled(t, "ServiceCheck", canConnectServiceCheck, metrics.ServiceCheckOK, "", []string(nil), mock.Anything)
	mockSender.AssertCalled(t, "ServiceCheck", systemStateServiceCheck, metrics.ServiceCheckOK, "", []string(nil), mock.Anything)

	tags := []string{"unit:unit1.service"}
	mockSender.AssertCalled(t, "ServiceCheck", unitStateServiceCheck, metrics.ServiceCheckOK, "", tags, "")

	tags = []string{"unit:unit2.service"}
	mockSender.AssertCalled(t, "ServiceCheck", unitStateServiceCheck, metrics.ServiceCheckCritical, "", tags, "")

	tags = []string{"unit:unit3.service"}
	mockSender.AssertNotCalled(t, "ServiceCheck", unitStateServiceCheck, metrics.ServiceCheckCritical, "", tags, "")

	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 4)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestServiceCheckUnitStateCustomMapping(t *testing.T) {
	rawInstanceConfig := []byte(`
unit_names:
 - unit1.service
 - unit2.service
substate_status_mapping:
  unit1.service:
    running: ok
    exited: critical
  unit2.service:
    running: ok
    exited: critical
`)

	stats := createDefaultMockSystemdStats()
	stats.On("ListUnits", mock.Anything).Return([]dbus.UnitStatus{
		{Name: "unit1.service", SubState: "running"},
		{Name: "unit2.service", SubState: "exited"},
		{Name: "unit3.service", SubState: "running"},
	}, nil)
	stats.On("UnixNow").Return(int64(1000 * 1000))

	stats.On("GetUnitTypeProperties", mock.Anything, mock.Anything, dbusTypeMap[typeService]).Return(map[string]interface{}{
		"CPUUsageNSec":  uint64(1),
		"MemoryCurrent": uint64(1),
		"TasksCurrent":  uint64(1),
	}, nil)

	stats.On("GetUnitTypeProperties", mock.Anything, "unit1.service", dbusTypeMap[typeUnit]).Return(map[string]interface{}{
		"ActiveEnterTimestamp": uint64(100),
	}, nil)
	stats.On("GetUnitTypeProperties", mock.Anything, "unit2.service", dbusTypeMap[typeUnit]).Return(map[string]interface{}{
		"ActiveEnterTimestamp": uint64(200),
	}, nil)

	check := SystemdCheck{stats: stats}
	check.Configure(rawInstanceConfig, nil, "test")

	// setup expectation
	mockSender := mocksender.NewMockSender(check.ID())
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	// run
	check.Run()

	// assertions
	mockSender.AssertCalled(t, "ServiceCheck", canConnectServiceCheck, metrics.ServiceCheckOK, "", []string(nil), mock.Anything)
	mockSender.AssertCalled(t, "ServiceCheck", systemStateServiceCheck, metrics.ServiceCheckOK, "", []string(nil), mock.Anything)

	tags := []string{"unit:unit1.service"}
	mockSender.AssertCalled(t, "ServiceCheck", unitStateServiceCheck, metrics.ServiceCheckUnknown, "", tags, "")
	mockSender.AssertCalled(t, "ServiceCheck", unitSubStateServiceCheck, metrics.ServiceCheckOK, "", tags, "")

	tags = []string{"unit:unit2.service"}
	mockSender.AssertCalled(t, "ServiceCheck", unitStateServiceCheck, metrics.ServiceCheckUnknown, "", tags, "")
	mockSender.AssertCalled(t, "ServiceCheck", unitSubStateServiceCheck, metrics.ServiceCheckCritical, "", tags, "")

	tags = []string{"unit:unit3.service"}
	mockSender.AssertNotCalled(t, "ServiceCheck", unitStateServiceCheck, metrics.ServiceCheckUnknown, "", tags, "")
	mockSender.AssertNotCalled(t, "ServiceCheck", unitSubStateServiceCheck, metrics.ServiceCheckCritical, "", tags, "")

	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 6)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestGetServiceCheckStatusDefaultMapping(t *testing.T) {
	data := []struct {
		activeState    string
		expectedStatus metrics.ServiceCheckStatus
	}{
		{"active", metrics.ServiceCheckOK},
		{"inactive", metrics.ServiceCheckCritical},
		{"failed", metrics.ServiceCheckCritical},
		{"activating", metrics.ServiceCheckUnknown},
		{"deactivating", metrics.ServiceCheckUnknown},
		{"does not exist", metrics.ServiceCheckUnknown},
	}

	for _, d := range data {
		t.Run(fmt.Sprintf("expected mapping from %s to %s", d.activeState, d.expectedStatus), func(t *testing.T) {
			assert.Equal(t, d.expectedStatus, getServiceCheckStatus(d.activeState, serviceCheckStateMapping))
		})
	}
}

func TestGetServiceCheckStatusCustomMapping(t *testing.T) {
	mapping := map[string]string{
		"foo": "critical",
		"bar": "ok",
		"baz": "warning",
		"sth": "unknown",
	}

	data := []struct {
		subState       string
		expectedStatus metrics.ServiceCheckStatus
	}{
		{"foo", metrics.ServiceCheckCritical},
		{"bar", metrics.ServiceCheckOK},
		{"baz", metrics.ServiceCheckWarning},
		{"sth", metrics.ServiceCheckUnknown},
		{"xyz", metrics.ServiceCheckUnknown},
	}

	for _, d := range data {
		t.Run(fmt.Sprintf("expected mapping from %s to %s", d.subState, d.expectedStatus), func(t *testing.T) {
			assert.Equal(t, d.expectedStatus, getServiceCheckStatus(d.subState, mapping))
		})
	}
}

func TestSendServicePropertyAsGaugeSkipAndWarnOnMissingProperty(t *testing.T) {
	serviceProperties := getCreatePropertieWithDefaults(map[string]interface{}{
		"CPUUsageNSec": uint64(110),
	})
	serviceUnitConfigCPU := metricConfigItem{metricName: "systemd.service.cpu_time_consumed", propertyName: "CPUUsageNSec", accountingProperty: "CPUAccounting", optional: false}
	serviceUnitConfigNRestart := metricConfigItem{metricName: "systemd.service.restart_count", propertyName: "NRestarts", accountingProperty: "", optional: false}

	check := SystemdCheck{}
	mockSender := mocksender.NewMockSender(check.ID())
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	sendServicePropertyAsGauge(mockSender, serviceProperties, serviceUnitConfigCPU, nil)
	sendServicePropertyAsGauge(mockSender, serviceProperties, serviceUnitConfigNRestart, nil)

	mockSender.AssertCalled(t, "Gauge", "systemd.service.cpu_time_consumed", float64(110), "", []string(nil))
	mockSender.AssertNotCalled(t, "Gauge", "systemd.service.restart_count", mock.Anything, mock.Anything, mock.Anything)
}

func TestIsMonitored(t *testing.T) {
	rawInstanceConfig := []byte(`
unit_names:
  - unit1.service
  - unit2.service
`)

	check := SystemdCheck{}
	check.Configure(rawInstanceConfig, nil, "test")

	data := []struct {
		unitName              string
		expectedToBeMonitored bool
	}{
		{"unit1.service", true},
		{"unit2.service", true},
		{"unit3.service", false},
	}
	for _, d := range data {
		t.Run(fmt.Sprintf("check.isMonitored('%s') expected to be %v", d.unitName, d.expectedToBeMonitored), func(t *testing.T) {
			assert.Equal(t, d.expectedToBeMonitored, check.isMonitored(d.unitName))
		})
	}
}

func TestIsMonitoredEmptyConfigShouldNone(t *testing.T) {
	rawInstanceConfig := []byte(``)
	check := SystemdCheck{}
	check.Configure(rawInstanceConfig, nil, "test")

	data := []struct {
		unitName              string
		expectedToBeMonitored bool
	}{
		{"unit1.service", false},
		{"xyz.socket", false},
	}
	for _, d := range data {
		t.Run(fmt.Sprintf("check.isMonitored('%s') expected to be %v", d.unitName, d.expectedToBeMonitored), func(t *testing.T) {
			assert.Equal(t, d.expectedToBeMonitored, check.isMonitored(d.unitName))
		})
	}
}

func TestComputeUptime(t *testing.T) {
	data := map[string]struct {
		activeState     string
		activeEnterTime uint64
		nanoNow         int64
		expectedUptime  int64
	}{
		"active happy path":              {"active", 1000 * 1000 * 1000, 2500, 1500},
		"inactive with valid enter time": {"inactive", 1000 * 1000 * 1000, 2500, 0},
		"inactive zero":                  {"inactive", 0, 0, 0},
		"invalid enter time after now":   {"active", 1000 * 1000 * 1000, 500, 0},
	}
	for name, d := range data {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, d.expectedUptime, computeUptime(d.activeState, d.activeEnterTime, d.nanoNow))
		})
	}
}

func TestGetPropertyUint64(t *testing.T) {
	properties := map[string]interface{}{
		"prop_uint":   uint(3),
		"prop_uint32": uint32(5),
		"prop_uint64": uint64(10),
		"prop_int64":  int64(20),
		"prop_string": "foo bar",
	}

	data := map[string]struct {
		propertyName   string
		expectedNumber uint64
		expectedError  error
	}{
		"prop_uint property retrieved": {"prop_uint", 3, nil},
		"uint32 property retrieved":    {"prop_uint32", 5, nil},
		"uint64 property retrieved":    {"prop_uint64", 10, nil},
		"error int64 not valid":        {"prop_int64", 0, fmt.Errorf("property prop_int64 (int64) cannot be converted to uint64")},
		"error string not valid":       {"prop_string", 0, fmt.Errorf("property prop_string (string) cannot be converted to uint64")},
		"error prop not exist":         {"prop_not_exist", 0, fmt.Errorf("property prop_not_exist not found")},
	}
	for name, d := range data {
		t.Run(name, func(t *testing.T) {
			num, err := getPropertyUint64(properties, d.propertyName)
			assert.Equal(t, d.expectedNumber, num)
			assert.Equal(t, d.expectedError, err)
		})
	}
}

func TestGetPropertyString(t *testing.T) {
	properties := map[string]interface{}{
		"prop_uint":   uint(3),
		"prop_string": "foo bar",
	}

	data := map[string]struct {
		propertyName   string
		expectedString string
		expectedError  error
	}{
		"valid string":         {"prop_string", "foo bar", nil},
		"prop_uint not valid":  {"prop_uint", "", fmt.Errorf("property prop_uint (uint) cannot be converted to string")},
		"error prop not exist": {"prop_not_exist", "", fmt.Errorf("property prop_not_exist not found")},
	}
	for name, d := range data {
		t.Run(name, func(t *testing.T) {
			num, err := getPropertyString(properties, d.propertyName)
			assert.Equal(t, d.expectedString, num)
			assert.Equal(t, d.expectedError, err)
		})
	}
}

func TestGetPropertyBool(t *testing.T) {
	properties := map[string]interface{}{
		"prop_uint":       uint(3),
		"prop_bool_true":  true,
		"prop_bool_false": false,
	}

	data := map[string]struct {
		propertyName      string
		expectedBoolValue bool
		expectedError     error
	}{
		"valid bool true":      {"prop_bool_true", true, nil},
		"valid bool false":     {"prop_bool_false", false, nil},
		"prop_uint not valid":  {"prop_uint", false, fmt.Errorf("property prop_uint (uint) cannot be converted to bool")},
		"error prop not exist": {"prop_not_exist", false, fmt.Errorf("property prop_not_exist not found")},
	}
	for name, d := range data {
		t.Run(name, func(t *testing.T) {
			num, err := getPropertyBool(properties, d.propertyName)
			assert.Equal(t, d.expectedBoolValue, num)
			assert.Equal(t, d.expectedError, err)
		})
	}
}
