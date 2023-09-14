// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package flare

import (
	"bytes"
	"context"
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
		"Application": "Event/System/Provider[@Name=\"datadog-trace-agent\" or @Name=\"DatadogAgent\"]",
		"Microsoft-Windows-WMI-Activity/Operational": "*",
	}
	execTimeout = 30 * time.Second
)

const (
	evtExportLogChannelPath uint32 = 0x1
)

func getCounterStrings(fb flaretypes.FlareBuilder) error {
	return fb.AddFileFromFunc("counter_strings.txt",
		func() ([]byte, error) {
			bufferIncrement := uint32(1024)
			bufferSize := bufferIncrement
			var counterlist []uint16
			for {
				var regtype uint32
				counterlist = make([]uint16, bufferSize)
				var sz uint32
				sz = bufferSize
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

func getTypeperfData(fb flaretypes.FlareBuilder) error {
	cancelctx, cancelfunc := context.WithTimeout(context.Background(), execTimeout)
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

func getLodctrOutput(fb flaretypes.FlareBuilder) error {
	cancelctx, cancelfunc := context.WithTimeout(context.Background(), execTimeout)
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

// getWindowsEventLogs exports Windows event logs.
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

func getServiceStatus(fb flaretypes.FlareBuilder) error {
	return fb.AddFileFromFunc(
		"servicestatus.txt",
		func() ([]byte, error) {
			cancelctx, cancelfunc := context.WithTimeout(context.Background(), execTimeout)
			defer cancelfunc()

			cmd := exec.CommandContext(cancelctx, "powershell", "-c", "get-service", "data*,ddnpm", "|", "fl")

			var out bytes.Buffer
			cmd.Stdout = &out
			err := cmd.Run()
			if err != nil {
				log.Warnf("Error running powershell command %v", err)
				// keep trying to get data even if this fails
			}

			f := &bytes.Buffer{}
			_, err = f.Write(out.Bytes())
			if err != nil {
				log.Warnf("Error writing file %v", err)
				return nil, err
			}
			out.Reset()
			// get the full driver configuration information
			cmd = exec.CommandContext(cancelctx, "powershell", "-c", "sc.exe", "qc", "ddnpm")

			cmd.Stdout = &out
			err = cmd.Run()
			if err != nil {
				log.Warnf("Error running powershell command %v", err)
				// don't fail if this command fails; there's still lots of good data here
			}

			_, err = f.Write(out.Bytes())
			if err != nil {
				log.Warnf("Error writing file %v", err)
				return nil, err
			}

			// compute the location of the driver
			ddroot, err := winutil.GetProgramFilesDirForProduct("DataDog Agent")
			if err == nil {
				pathtodriver := filepath.Join(ddroot, "bin", "agent", "driver", "ddnpm.sys")
				fi, err := os.Stat(pathtodriver)
				if err != nil {
					f.Write([]byte(fmt.Sprintf("Failed to stat file %v %v\n", pathtodriver, err))) //nolint:errcheck
				} else {
					f.Write([]byte(fmt.Sprintf("Driver last modification time : %v\n", fi.ModTime().Format(time.UnixDate)))) //nolint:errcheck
					// also show the file version resource
					out.Reset()

					quotedPath := fmt.Sprintf("\"%s\"", pathtodriver)
					cmd = exec.CommandContext(cancelctx, "powershell", "-c", "gci", quotedPath, "|", "fl")

					cmd.Stdout = &out
					err = cmd.Run()
					if err != nil {
						log.Warnf("Error running powershell command %v", err)
						// we've gotten a lot of data to this point.  don't fail just because this fails

					}

					_, err = f.Write(out.Bytes())
					if err != nil {
						log.Warnf("Error writing file %v", err)
						return nil, err
					}
				}
			} else {
				return nil, fmt.Errorf("Error getting path to datadog agent binaries %v", err)
			}

			return f.Bytes(), nil
		})
}

// getDatadogRegistry function saves all Datadog registry keys and values from HKLM\Software\Datadog.
// The implementation is based on the invoking Windows built-in reg.exe command, which does all
// heavy lifting (instead of relying on explicit and recursive Registry API calls).
// More technical details can be found in the PR https://github.com/DataDog/datadog-agent/pull/11290
func getDatadogRegistry(fb flaretypes.FlareBuilder) error {
	// Generate raw exported registry file which we will scrub just in case
	rawf, err := fb.PrepareFilePath("datadog-raw.reg")
	if err != nil {
		return fmt.Errorf("Error in ensureParentDirsExist %v", err)
	}

	// reg.exe is built in Windows utility which will be always present
	// https://docs.microsoft.com/en-us/windows-server/administration/windows-commands/reg
	cmd := exec.Command("reg", "export", "HKLM\\Software\\Datadog", rawf, "/y")
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

func getWindowsData(fb flaretypes.FlareBuilder) {
	getTypeperfData(fb)     //nolint:errcheck
	getLodctrOutput(fb)     //nolint:errcheck
	getCounterStrings(fb)   //nolint:errcheck
	getWindowsEventLogs(fb) //nolint:errcheck
	getServiceStatus(fb)    //nolint:errcheck
	getDatadogRegistry(fb)  //nolint:errcheck
}
