package system

import (
	"testing"
)

func uptimeSampler() (uint64, error) {
	return 555, nil
}

func TestUptimeCheckLinux(t *testing.T) {
	uptime = uptimeSampler
	uptimeCheck := new(UptimeCheck)
	uptimeCheck.Configure(nil, nil)

	mock := new(MockSender)
	uptimeCheck.sender = mock

	mock.On("Gauge", "system.uptime", 555.0, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)

	uptimeCheck.Run()
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 1)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}
