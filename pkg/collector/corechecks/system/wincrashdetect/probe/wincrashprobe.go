// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build windows

package probe

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"golang.org/x/sys/windows/registry"
)

// WinCrashProbe has no stored state.
type WinCrashProbe struct {
}

// NewWinCrashProbe returns an initialized WinCrashProbe
func NewWinCrashProbe(_ *config.Config) (*WinCrashProbe, error) {
	return &WinCrashProbe{}, nil
}

// Get returns the current crash, if any
func (p *WinCrashProbe) Get() *WinCrashStatus {
	wcs := &WinCrashStatus{}

	err := wcs.getCurrentCrashSettings()
	if err != nil {
		wcs.ErrString = err.Error()
		wcs.Success = false
		return wcs
	}

	if len(wcs.FileName) == 0 {
		// no filename means no crash dump
		wcs.Success = true // we succeeded
		return wcs
	}
	parseCrashDump(wcs)

	return wcs
}
func (wcs *WinCrashStatus) getCurrentCrashSettings() error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\CrashControl`,
		registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("Error opening CrashControl key %v", err)
	}
	defer k.Close()

	/// get the dump type
	v, _, err := k.GetIntegerValue("CrashDumpEnabled")
	if err != nil {
		return err
	}
	if v > uint64(DumpTypeAutomatic) {
		// this should never happen.  Unexpected type
		return fmt.Errorf("Unknown dump type %d", v)
	}
	wcs.Type = int(v)

	// minidump directory (by default), stored in MinidumpDir key
	//  small,
	if wcs.Type == DumpTypeHeader {
		// need to get directory
		dir, _, err := k.GetStringValue("MinidumpDir")
		if err != nil {
			return fmt.Errorf("unable to get minidumpdir %v", err)
		}
		dir, err = winutil.ExpandEnvironmentStrings(dir)
		if err != nil {
			return fmt.Errorf("error expanding directory string %s %v", dir, err)
		}
		// now need to find most recent file

		// since the configuration for small dump files allows only the directory
		// name, we're assuming it's safe to assume the `.dmp` extension
		dumpfiles := filepath.Join(dir, "*.dmp")

		var newestfile string
		var newesttime int64
		files, _ := filepath.Glob(dumpfiles)
		for _, fqn := range files {
			//fqn := filepath.Join(dir, f.Name())
			exists, err := os.Stat(fqn)
			if err != nil {
				// shouldn't happen
				return fmt.Errorf("Error enumerating dump files %s %v", fqn, err)
			}
			thistime := exists.ModTime().Unix()
			if thistime > newesttime {
				newesttime = thistime
				newestfile = fqn
			}
		}
		wcs.FileName = newestfile

	} else if wcs.Type == DumpTypeFull || wcs.Type == DumpTypeSummary || wcs.Type == DumpTypeAutomatic {

		// %systemroot%\memory.dmp (by default) stored in DumpFile key
		//  kernel, complete, automatic, active
		fn, _, err := k.GetStringValue("DumpFile")
		if err != nil {
			return fmt.Errorf("Error reading dump file name")
		}
		fn, err = winutil.ExpandEnvironmentStrings(fn)
		if err != nil {
			return fmt.Errorf("Error expanding dump file name %s %v", fn, err)
		}
		// check for existence
		_, err = os.Stat(fn)
		if err == nil {
			wcs.FileName = fn
		}

	}
	return nil
}
