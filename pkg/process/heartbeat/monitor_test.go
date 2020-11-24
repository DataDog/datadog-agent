package heartbeat

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
)

func TestModuleMonitor(t *testing.T) {
	enabledModulesFn := func() ([]string, error) {
		return []string{"network_tracer", "security_runtime", "oom_kill_probe"}, nil
	}

	flusher := &flusherMock{}
	monitor := &ModuleMonitor{
		enabledModulesFn: enabledModulesFn,
		metricNameFn:     func(moduleName string) string { return fmt.Sprintf("heartbeat.%s", moduleName) },
		flusher:          flusher,
	}

	t.Run("reporting all modules", func(t *testing.T) {
		flusher.On(
			"Flush",
			[]string{"heartbeat.network_tracer", "heartbeat.oom_kill_probe", "heartbeat.security_runtime"},
			mock.AnythingOfType("time.Time"),
		)
		monitor.Heartbeat()
		flusher.AssertExpectations(t)
	})

	t.Run("reporting a subset of modules", func(t *testing.T) {
		flusher.On(
			"Flush",
			[]string{"heartbeat.network_tracer", "heartbeat.security_runtime"},
			mock.AnythingOfType("time.Time"),
		)
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
