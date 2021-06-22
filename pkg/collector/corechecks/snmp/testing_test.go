package snmp

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/mock"
	"path/filepath"
)

type mockSession struct {
	mock.Mock
	connectErr error
	closeErr   error
	version    gosnmp.SnmpVersion
}

func (s *mockSession) Configure(config snmpConfig) error {
	return nil
}

func (s *mockSession) Connect() error {
	return s.connectErr
}

func (s *mockSession) Close() error {
	return s.closeErr
}

func (s *mockSession) Get(oids []string) (result *gosnmp.SnmpPacket, err error) {
	args := s.Mock.Called(oids)
	return args.Get(0).(*gosnmp.SnmpPacket), args.Error(1)
}

func (s *mockSession) GetBulk(oids []string, bulkMaxRepetitions uint32) (result *gosnmp.SnmpPacket, err error) {
	args := s.Mock.Called(oids, bulkMaxRepetitions)
	return args.Get(0).(*gosnmp.SnmpPacket), args.Error(1)
}

func (s *mockSession) GetNext(oids []string) (result *gosnmp.SnmpPacket, err error) {
	args := s.Mock.Called(oids)
	return args.Get(0).(*gosnmp.SnmpPacket), args.Error(1)
}

func (s *mockSession) GetVersion() gosnmp.SnmpVersion {
	return s.version
}

func (s *mockSession) Copy() sessionAPI {
	return s
}

func (s *mockSession) GetNumGetCalls() int {
	return 0
}

func (s *mockSession) GetNumGetBulkCalls() int {
	return 0
}

func (s *mockSession) GetNumGetNextCalls() int {
	return 0
}

func (s *mockSession) ResetCallCounts() {
}

func createMockSession() *mockSession {
	session := &mockSession{}
	session.version = gosnmp.Version2c
	return session
}

func setConfdPathAndCleanProfiles() {
	globalProfileConfigMap = nil // make sure from the new confd path will be reloaded
	file, _ := filepath.Abs(filepath.Join(".", "test", "conf.d"))
	config.Datadog.Set("confd_path", file)
}
