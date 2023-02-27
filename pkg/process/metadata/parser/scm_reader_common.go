package parser

import (
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// A basic mock for `winutil.SCMMonitor`.
// It is included in the common package so the `process-agent.gen-mocks` job doesn't
// fail due to not being able to find the interface on linux.
type mockableSCM interface {
	GetServiceInfo(pid uint64) (*winutil.ServiceInfo, error)
}
