// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package flare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

var (
	modWinEvtAPI     = windows.NewLazySystemDLL("wevtapi.dll")
	procEvtExportLog = modWinEvtAPI.NewProc("EvtExportLog")

	eventLogChannelsToExport = map[string]string{
		"System":      "Event/System/Provider[@Name=\"Service Control Manager\"]",
		"Application": "Event/System/Provider[@Name=\"datadog-trace-agent\" or @Name=\"DatadogAgent\" or @Name=\".NET Runtime\" or @Name=\"Application Error\"]",
		"Microsoft-Windows-WMI-Activity/Operational": "*",
	}
	execTimeout = 30 * time.Second
)

// commandContextForFlare returns a context for a subprocess bounded by both the provider ctx and perCommandTimeout.
func commandContextForFlare(parent context.Context, perCommandTimeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, perCommandTimeout)
}

const (
	evtExportLogChannelPath uint32 = 0x1
)

// getCounterStrings does not accept a context because it reads the Windows
// performance counter registry directly via RegQueryValueEx (a Win32 API call),
// which cannot be canceled through a Go context.
func getCounterStrings(fb flaretypes.FlareBuilder) error {
	return fb.AddFileFromFunc("counter_strings.txt",
		func() ([]byte, error) {
			bufferIncrement := uint32(1024)
			bufferSize := bufferIncrement
			var counterlist []uint16
			for {
				var regtype uint32
				counterlist = make([]uint16, bufferSize)
				sz := bufferSize
				regerr := windows.RegQueryValueEx(windows.HKEY_PERFORMANCE_DATA,
					windows.StringToUTF16Ptr("Counter 009"),
					nil, // reserved
					&regtype,
					(*byte)(unsafe.Pointer(&counterlist[0])),
					&sz)
				if regerr == error(windows.ERROR_MORE_DATA) {
					// buffer's not big enough
					bufferSize += bufferIncrement
					continue
				}
				// must set the length of the slice to the actual amount of data
				// sz is in bytes, but it's a slice of uint16s, so divide the returned
				// buffer size by two.
				counterlist = counterlist[:(sz / 2)]
				break
			}
			clist := winutil.ConvertWindowsStringList(counterlist)

			f := &bytes.Buffer{}
			for i := 0; i < len(clist); i++ {
				f.Write([]byte(clist[i])) //nolint:errcheck
				f.Write([]byte("\r\n"))   //nolint:errcheck
			}
			return f.Bytes(), nil
		},
	)
}

func getTypeperfData(ctx context.Context, fb flaretypes.FlareBuilder) error {
	cancelctx, cancelfunc := commandContextForFlare(ctx, execTimeout)
	defer cancelfunc()

	cmd := exec.CommandContext(cancelctx, "typeperf", "-qx")

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Errorf("Could not write typeperf data: %s", err)
	}

	return fb.AddFile("typeperf.txt", out.Bytes())
}

func getLodctrOutput(ctx context.Context, fb flaretypes.FlareBuilder) error {
	cancelctx, cancelfunc := commandContextForFlare(ctx, execTimeout)
	defer cancelfunc()

	cmd := exec.CommandContext(cancelctx, "lodctr.exe", "/q")

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Warnf("Error running lodctr command %v", err)
		// for some reason the lodctr command returns error 259 even when
		// it succeeds.  Log the error in case it's some other error,
		// but continue on.
	}

	return fb.AddFile("lodctr.txt", out.Bytes())
}

// getWindowsEventLogs exports Windows event logs. It does not accept a context
// because it uses the EvtExportLog Win32 API directly, which cannot be canceled
// through a Go context.
func getWindowsEventLogs(fb flaretypes.FlareBuilder) error {
	var err error

	for eventLogChannel := range eventLogChannelsToExport {
		eventLogFileName := eventLogChannel + ".evtx"
		eventLogQuery := eventLogChannelsToExport[eventLogChannel]

		// Export one event log file to the temporary location.
		errExport := exportWindowsEventLog(
			fb,
			eventLogChannel,
			eventLogQuery,
			eventLogFileName)

		if errExport != nil {
			log.Warnf("could not export event log %v, error: %v", eventLogChannel, errExport)
			err = errExport
		}
	}
	if err != nil {
		return log.Errorf("Could not export Windows event logs: %v", err)
	}
	return nil
}

// exportWindowsEventLog exports one event log file to the temporary location.
// destFileName might contain a path.
func exportWindowsEventLog(fb flaretypes.FlareBuilder, eventLogChannel, eventLogQuery, destFileName string) error {
	// Put all event logs under "eventlog" folder
	destFullFileName, err := fb.PrepareFilePath(filepath.Join("eventlog", destFileName))
	if err != nil {
		log.Warnf("cannot create folder for %s: %v", destFullFileName, err)
		return err
	}

	eventLogChannelUtf16, _ := windows.UTF16PtrFromString(eventLogChannel)
	eventLogQueryUtf16, _ := windows.UTF16PtrFromString(eventLogQuery)
	destFullFileNameUtf16, _ := windows.UTF16PtrFromString(destFullFileName)

	ret, _, evtExportLogError := procEvtExportLog.Call(
		uintptr(unsafe.Pointer(nil)),                   // Machine name, NULL for local machine
		uintptr(unsafe.Pointer(eventLogChannelUtf16)),  // Channel name
		uintptr(unsafe.Pointer(eventLogQueryUtf16)),    // Query
		uintptr(unsafe.Pointer(destFullFileNameUtf16)), // Destination file name
		uintptr(evtExportLogChannelPath))               // DWORD. Specify that the second parameter is a channel name

	// ret is a DWORD, TRUE for success, FALSE for failure.
	if ret == 0 {
		log.Warnf(
			"could not export event log from channel %s to file %s, LastError: %v",
			eventLogChannel,
			destFullFileName,
			evtExportLogError)

		err = evtExportLogError
	} else {
		log.Infof("successfully exported event channel %v to %v", eventLogChannel, destFullFileName)
	}

	return err
}

// getServiceStatus does not accept a context because it queries the Windows
// Service Control Manager via Win32 API calls, which cannot be canceled through
// a Go context.
func getServiceStatus(fb flaretypes.FlareBuilder) error {
	return fb.AddFileFromFunc(
		"servicestatus.json",
		func() ([]byte, error) {
			manager, err := winutil.OpenSCManager(scManagerAccess)
			if err != nil {
				log.Warnf("Error connecting to service control manager %v", err)
				return nil, err
			}
			defer manager.Disconnect()

			ddServices, err := getDDServices(manager)
			if err != nil {
				log.Warnf("Error getting service information %v", err)
				return nil, err
			}

			ddJSON, err := json.MarshalIndent(ddServices, "", "  ")
			if err != nil {
				log.Warnf("Error Marshaling to JSON %v", err)
				return nil, err
			}

			return ddJSON, err
		},
	)
}

// getDatadogRegistry function saves all Datadog registry keys and values from HKLM\Software\Datadog.
// The implementation is based on the invoking Windows built-in reg.exe command, which does all
// heavy lifting (instead of relying on explicit and recursive Registry API calls).
// More technical details can be found in the PR https://github.com/DataDog/datadog-agent/pull/11290
func getDatadogRegistry(ctx context.Context, fb flaretypes.FlareBuilder) error {
	// Generate raw exported registry file which we will scrub just in case
	rawf, err := fb.PrepareFilePath("datadog-raw.reg")
	if err != nil {
		return fmt.Errorf("Error in ensureParentDirsExist %v", err)
	}

	// reg.exe is built in Windows utility which will be always present
	// https://docs.microsoft.com/en-us/windows-server/administration/windows-commands/reg
	regCtx, regCancel := commandContextForFlare(ctx, execTimeout)
	defer regCancel()
	cmd := exec.CommandContext(regCtx, "reg", "export", "HKLM\\Software\\Datadog", rawf, "/y")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("Error getting Datadog registry exported via reg command. %v [%s]", stderr.String(), err)
		}
		return fmt.Errorf("Error getting Datadog registry exported via reg command. %v", err)
	}
	// Temporary datadog-raw.reg is created. Remove it when the function exits
	defer os.Remove(rawf)

	// Read raw registry file in memory ...
	data, err := os.ReadFile(rawf)
	if err != nil {
		return err
	}

	return fb.AddFile("datadog.reg", data)
}

func getEventLogConfig(ctx context.Context, fb flaretypes.FlareBuilder) error {
	var out bytes.Buffer
	// creating a buffer to append all cmd outputs
	fullOutput := &bytes.Buffer{}
	channels := [3]string{"Application", "System", "Security"}

	for _, channel := range channels {
		chCtx, chCancel := commandContextForFlare(ctx, execTimeout)
		cmd := exec.CommandContext(chCtx, "wevtutil", "gl", channel)
		cmd.Stdout = &out
		err := cmd.Run()
		if err != nil {
			chCancel()
			log.Warnf("Error getting config for %s: %v", channel, err)
			return err
		}
		_, err = fullOutput.Write(out.Bytes())
		chCancel()
		if err != nil {
			log.Warnf("Error writing file %v", err)
			return err
		}

		// adding a newline character to make the file easier to read
		_, err = fullOutput.Write([]byte("\n"))
		if err != nil {
			log.Warnf("Error writing file %v", err)
			return err
		}

		out.Reset()
	}

	return fb.AddFile("eventlogconfig.txt", fullOutput.Bytes())

}

// getIISData collects IIS web application information grouped by application pool.
// This helps diagnose IIS-related issues by showing the distribution of web applications
// across application pools.
func getIISData(ctx context.Context, fb flaretypes.FlareBuilder) error {
	cancelctx, cancelfunc := commandContextForFlare(ctx, execTimeout)
	defer cancelfunc()

	// PowerShell command to get web applications grouped by application pool
	psCommand := "Get-WebApplication | Group-Object ApplicationPool | Select-Object Name, Count | Format-Table -AutoSize"

	cmd := exec.CommandContext(cancelctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", psCommand)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// IIS may not be installed, which is expected on many systems
		log.Debugf("Could not collect IIS data (IIS may not be installed): %s, stderr: %s", err, stderr.String())
		// Still add the file with error info so support knows we tried
		errorMsg := fmt.Sprintf("Could not collect IIS data: %s\nStderr: %s\nNote: This is expected if IIS is not installed.", err, stderr.String())
		return fb.AddFile("iis_application_pools.txt", []byte(errorMsg))
	}

	return fb.AddFile("iis_application_pools.txt", out.Bytes())
}

func getDDUserGroupsAndRightsStatus(fb flaretypes.FlareBuilder) error {
	return fb.AddFileFromFunc(
		"agent_user_info.json",
		func() ([]byte, error) {
			// Get user groups and rights status
			var hasDesiredGroups, hasDesiredRights bool
			var groupsErr, rightsErr error

			// We don't use/report actualGroups or actualRights in this flare function
			_, hasDesiredGroups, groupsErr = winutil.DoesAgentUserHaveDesiredGroups()
			_, hasDesiredRights, rightsErr = winutil.DoesAgentUserHaveDesiredRights()

			// Helper function to convert bool + error to simple status
			getStatusString := func(hasDesired bool, err error) string {
				if err != nil {
					return "ERROR"
				}
				if hasDesired {
					return "YES"
				}
				return "NO"
			}

			// Simple result structure - just YES/NO
			result := map[string]interface{}{
				"hasDesiredGroups": hasDesiredGroups,
				"hasDesiredRights": hasDesiredRights,
				"groupsStatus":     getStatusString(hasDesiredGroups, groupsErr),
				"rightsStatus":     getStatusString(hasDesiredRights, rightsErr),
			}

			// Marshal to JSON
			ddUserStatusJSON, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				log.Warnf("Error marshaling DDAgent user status to JSON %v", err)
				return nil, err
			}

			return ddUserStatusJSON, nil
		},
	)
}

func getWindowsData(ctx context.Context, fb flaretypes.FlareBuilder) error {
	getTypeperfData(ctx, fb)           //nolint:errcheck
	getLodctrOutput(ctx, fb)           //nolint:errcheck
	getCounterStrings(fb)              //nolint:errcheck
	getWindowsEventLogs(fb)            //nolint:errcheck
	getServiceStatus(fb)               //nolint:errcheck
	getDatadogRegistry(ctx, fb)        //nolint:errcheck
	getEventLogConfig(ctx, fb)         //nolint:errcheck
	getIISData(ctx, fb)                //nolint:errcheck
	getDDUserGroupsAndRightsStatus(fb) //nolint:errcheck
	return nil
}
