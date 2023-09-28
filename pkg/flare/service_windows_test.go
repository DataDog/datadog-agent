//go:build windows

package flare

import (
	"testing"

	"golang.org/x/sys/windows"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func TestWindowsService(t *testing.T) {
	manager, err := winutil.OpenSCManager(SC_MANAGER_ACCESS)
	if err != nil {
		log.Warnf("Error connecting to service control manager %v", err)
		manager.Disconnect()
		return
	}
	defer manager.Disconnect()

	evtlog, npm_err := winutil.OpenService(manager, "EventLog", windows.GENERIC_READ)
	if npm_err != nil {
		return
	}
	evtlogConf, err := GetServiceInfo(evtlog)
	if err != nil {
		return
	}

	assert.Contains(t, evtlogConf.ServiceName, "EventLog")

	assert.Contains(t, evtlogConf.Config.ServiceType, "Win32ShareProcess")

	var zero uint32 = 0
	assert.Equal(t, evtlogConf.TriggersCount, zero)

	assert.NotNil(t, evtlogConf.ServiceFailureActions.RecoveryActions)
	assert.NotContains(t, evtlogConf.ServiceState, "Unknown")

	if evtlogConf.ServiceState == "Running" {
		assert.NotEqual(t, evtlogConf.ProcessId, 0)
	}

}
