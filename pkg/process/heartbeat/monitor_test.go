package heartbeat

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
)

func TestModuleMonitor(t *testing.T) {
	stats := func() (map[string]interface{}, error) {
		return map[string]interface{}{
			"network_tracer":   nil,
			"security_runtime": nil,
			"oom_kill_probe":   nil,
		}, nil
	}

	flusher := &flusherMock{}
	monitor := &ModuleMonitor{
		statsFn: statsFn(stats),
		flusher: flusher,
	}

	t.Run("reporting all modules", func(t *testing.T) {
		flusher.On("Flush", []string{"network_tracer", "oom_kill_probe", "security_runtime"}, mock.AnythingOfType("time.Time"))
		monitor.Heartbeat()
		flusher.AssertExpectations(t)
	})
	t.Run("reporting a subset of modules", func(t *testing.T) {
		flusher.On("Flush", []string{"network_tracer", "security_runtime"}, mock.AnythingOfType("time.Time"))
		monitor.Heartbeat("abc", "security_runtime", "network_tracer")
		flusher.AssertExpectations(t)
	})
	t.Run("reporting a module that isn't enabled", func(t *testing.T) {
		monitor.Heartbeat("abc")
		flusher.AssertNotCalled(t, "Flush")
	})
}

type flusherMock struct {
	mock.Mock
}

var _ flusher = &flusherMock{}

func (f *flusherMock) Flush(modules []string, now time.Time) {
	f.Called(modules, now)
}

func (f *flusherMock) Stop() {
	f.Called()
}
