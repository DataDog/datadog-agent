// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package flare

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

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

func zipCounterStrings(tempDir, hostname string) error {
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
	fname := filepath.Join(tempDir, hostname, "counter_strings.txt")
	err := ensureParentDirsExist(fname)
	if err != nil {
		return err
	}
	f, err := os.Create(fname)
	if err != nil {
		return err
	}
	defer f.Close()
	for i := 0; i < len(clist); i++ {
		_, err = f.WriteString(clist[i])
		if err != nil {
			return err
		}
		_, err = f.WriteString("\r\n")
		if err != nil {
			return err
		}
	}
	err = f.Sync()
	if err != nil {
		return err
	}
	return nil

}

func zipTypeperfData(tempDir, hostname string) error {
	cancelctx, cancelfunc := context.WithTimeout(context.Background(), execTimeout)
	defer cancelfunc()

	cmd := exec.CommandContext(cancelctx, "typeperf", "-qx")

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return err
	}
	f := filepath.Join(tempDir, hostname, "typeperf.txt")
	err = ensureParentDirsExist(f)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(f, out.Bytes(), os.ModePerm)
	if err != nil {
		return err
	}
	return nil
}
func zipLodctrOutput(tempDir, hostname string) error {
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
	f := filepath.Join(tempDir, hostname, "lodctr.txt")
	err = ensureParentDirsExist(f)
	if err != nil {
		log.Warnf("Error in ensureParentDirsExist %v", err)
		return err
	}

	err = ioutil.WriteFile(f, out.Bytes(), os.ModePerm)
	if err != nil {
		log.Warnf("Error writing file %v", err)
		return err
	}
	return nil
}

// zipWindowsEventLogs exports Windows event logs.
func zipWindowsEventLogs(tempDir, hostname string) error {
	var err error

	for eventLogChannel := range eventLogChannelsToExport {
		eventLogFileName := eventLogChannel + ".evtx"
		eventLogQuery := eventLogChannelsToExport[eventLogChannel]

		// Export one event log file to the temporary location.
		errExport := exportWindowsEventLog(
			eventLogChannel,
			eventLogQuery,
			eventLogFileName,
			tempDir,
			hostname)

		if errExport != nil {
			log.Warnf("could not export event log %v, error: %v", eventLogChannel, errExport)
			err = errExport
		}
	}

	return err
}

// exportWindowsEventLog exports one event log file to the temporary location.
// destFileName might contain a path.
func exportWindowsEventLog(eventLogChannel, eventLogQuery, destFileName, tempDir, hostname string) error {
	// Put all event logs under "eventlog" folder
	destFullFileName := filepath.Join(tempDir, hostname, "eventlog", destFileName)

	err := ensureParentDirsExist(destFullFileName)
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

type filePermsInfo struct{}

func (p permissionsInfos) add(filePath string) {}
func (p permissionsInfos) commit(tempDir, hostname string, mode os.FileMode) error {
	return nil
}

func zipServiceStatus(tempDir, hostname string) error {
	f := filepath.Join(tempDir, hostname, "servicestatus.txt")
	err := ensureParentDirsExist(f)
	if err != nil {
		return fmt.Errorf("Error in ensureParentDirsExist %v", err)
	}

	fh, err := os.Create(f)
	if err != nil {
		return fmt.Errorf("Error creating temp file %s %v", f, err)
	}
	defer fh.Close()
	cancelctx, cancelfunc := context.WithTimeout(context.Background(), execTimeout)
	defer cancelfunc()

	cmd := exec.CommandContext(cancelctx, "powershell", "-c", "get-service", "data*,ddnpm", "|", "fl")

	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		log.Warnf("Error running powershell command %v", err)
		// for some reason the lodctr command returns error 259 even when
		// it succeeds.  Log the error in case it's some other error,
		// but continue on.
	}

	_, err = fh.Write(out.Bytes())
	if err != nil {
		log.Warnf("Error writing file %v", err)
		return err
	}
	// compute the location of the driver
	ddroot, err := winutil.GetProgramFilesDirForProduct("DataDog Agent")
	if err == nil {
		pathtodriver := filepath.Join(ddroot, "bin", "agent", "driver", "ddnpm.sys")
		fi, err := os.Stat(pathtodriver)
		if err != nil {
			_, _ = fh.WriteString(fmt.Sprintf("Failed to stat file %v %v\n", pathtodriver, err))
		} else {
			_, _ = fh.WriteString(fmt.Sprintf("Driver last modification time : %v\n", fi.ModTime().Format(time.UnixDate)))
		}
	} else {
		return fmt.Errorf("Error getting path to datadog agent binaries %v", err)
	}
	return nil
}

// zipDatadogRegistry function saves all Datadog registry keys and values from HKLM\Software\Datadog.
// The implementation is based on the invoking Windows built-in reg.exe command, which does all
// heavy lifting (instead of relying on explicit and recursive Registry API calls).
// More technical details can be found in the PR https://github.com/DataDog/datadog-agent/pull/11290
func zipDatadogRegistry(tempDir, hostname string) error {
	// Generate raw exported registry file which we will scrub just in case
	rawf := filepath.Join(tempDir, hostname, "datadog-raw.reg")
	err := ensureParentDirsExist(rawf)
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
	data, err := ioutil.ReadFile(rawf)
	if err != nil {
		return err
	}

	// ... scrub it and write it back
	f := filepath.Join(tempDir, hostname, "datadog.reg")
	err = writeScrubbedFile(f, data)
	if err != nil {
		return err
	}

	return nil
}
