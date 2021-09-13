package session

import (
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
)

// MockSession mocks a connection session
type MockSession struct {
	mock.Mock
	ConnectErr error
	CloseErr   error
	Version    gosnmp.SnmpVersion
}

// Configure configures the session
func (s *MockSession) Configure(config checkconfig.CheckConfig) error {
	return nil
}

// Connect is used to create a new connection
func (s *MockSession) Connect() error {
	return s.ConnectErr
}

// Close is used to close the connection
func (s *MockSession) Close() error {
	return s.CloseErr
}

// Get will send a SNMPGET command
func (s *MockSession) Get(oids []string) (result *gosnmp.SnmpPacket, err error) {
	args := s.Mock.Called(oids)
	return args.Get(0).(*gosnmp.SnmpPacket), args.Error(1)
}

// GetBulk will send a SNMP BULKGET command
func (s *MockSession) GetBulk(oids []string, bulkMaxRepetitions uint32) (result *gosnmp.SnmpPacket, err error) {
	args := s.Mock.Called(oids, bulkMaxRepetitions)
	return args.Get(0).(*gosnmp.SnmpPacket), args.Error(1)
}

// GetNext will send a SNMP GETNEXT command
func (s *MockSession) GetNext(oids []string) (result *gosnmp.SnmpPacket, err error) {
	args := s.Mock.Called(oids)
	return args.Get(0).(*gosnmp.SnmpPacket), args.Error(1)
}

// GetVersion returns the snmp version used
func (s *MockSession) GetVersion() gosnmp.SnmpVersion {
	return s.Version
}

// CreateMockSession creates a mock session
func CreateMockSession() *MockSession {
	session := &MockSession{}
	session.Version = gosnmp.Version2c
	return session
}

// NewMockSession creates a mock session
func NewMockSession() Session {
	return CreateMockSession()
}
