package other

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/stretchr/testify/mock"
)

//MockSender allows mocking of the checks sender
type MockSender struct {
	mock.Mock
}

//Rate adds a rate type to the mock calls.
func (m *MockSender) Rate(metric string, value float64, hostname string, tags []string) {
	m.Called(metric, value, hostname, tags)
}

//Count adds a count type to the mock calls.
func (m *MockSender) Count(metric string, value float64, hostname string, tags []string) {
	m.Called(metric, value, hostname, tags)
}

//MonotonicCount adds a monotonic count type to the mock calls.
func (m *MockSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	m.Called(metric, value, hostname, tags)
}

//Histogram adds a histogram type to the mock calls.
func (m *MockSender) Histogram(metric string, value float64, hostname string, tags []string) {
	m.Called(metric, value, hostname, tags)
}

//Gauge adds a gauge type to the mock calls.
func (m *MockSender) Gauge(metric string, value float64, hostname string, tags []string) {
	m.Called(metric, value, hostname, tags)
}

//ServiceCheck enables the service check mock call.
func (m *MockSender) ServiceCheck(checkName string, status aggregator.ServiceCheckStatus, hostname string, tags []string, message string) {
	m.Called(checkName, status, hostname, tags, message)
}

//Event enables the event mock call.
func (m *MockSender) Event(e aggregator.Event) {
	m.Called(e)
}

//Commit enables the commit mock call.
func (m *MockSender) Commit() {
	m.Called()
}

var ntpCfgString = `
offset_threshold: 60
port: ntp
version: 3
timeout: 5
`

var ntpCfg = []byte(ntpCfgString)
var ntpInitCfg = []byte("")

func TestNTP(t *testing.T) {
	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(ntpCfg, ntpInitCfg)

	mockSender := new(MockSender)
	ntpCheck.sender = mockSender

	mockSender.On("Gauge", "ntp.offset", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	mockSender.On("ServiceCheck",
		"ntp.in_sync", mock.AnythingOfType("aggregator.ServiceCheckStatus"), "",
		[]string(nil), mock.AnythingOfType("string")).Return().Times(1)

	mockSender.On("Commit").Return().Times(1)
	ntpCheck.Run()

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Gauge", 1)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}
