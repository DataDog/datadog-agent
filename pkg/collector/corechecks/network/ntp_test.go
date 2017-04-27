package network

import (
	"testing"

	"github.com/stretchr/testify/mock"
)

var ntpCfgString = `
offset_threshold: 60
port: ntp
version: 3
timeout: 5
`

func TestNTP(t *testing.T) {
	var ntpCfg = []byte(ntpCfgString)
	var ntpInitCfg = []byte("")

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
