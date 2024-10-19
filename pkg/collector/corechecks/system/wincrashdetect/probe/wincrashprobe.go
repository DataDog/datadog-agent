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
	"sync"

	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"golang.org/x/sys/windows/registry"
)

type probeState uint32

const (
	// Idle indicates that the probe is waiting for a request
	idle probeState = iota

	// Busy indicates that the probe is currently processing a crash dump
	busy

	// Completed indicates that the probe finished processing a crash dump.
	completed

	// Failed indicates that the probe failed to process a crash dump.
	failed
)

// WinCrashProbe has no stored state.
type WinCrashProbe struct {
	state  probeState
	status *WinCrashStatus
	mu     sync.Mutex
}

// NewWinCrashProbe returns an initialized WinCrashProbe
func NewWinCrashProbe(_ *sysconfigtypes.Config) (*WinCrashProbe, error) {
	return &WinCrashProbe{
		state:  idle,
		status: nil,
	}, nil
}

// Handles crash dump parsing in a separate thread since this may take very long.
func (p *WinCrashProbe) parseCrashDumpAsync() {
	if p.status == nil {
		p.state = failed
		return
	}

	parseCrashDump(p.status)

	p.mu.Lock()
	defer p.mu.Unlock()
	p.state = completed
}

// Get returns the current crash, if any
func (p *WinCrashProbe) Get() *WinCrashStatus {
	wcs := &WinCrashStatus{}

	// Nothing in this method should take long.
	p.mu.Lock()
	defer p.mu.Unlock()

	switch p.state {
	case idle:
		if p.status == nil {
			// This is a new request.
			err := wcs.getCurrentCrashSettings()
			if err != nil {
				wcs.ErrString = err.Error()
				wcs.StatusCode = WinCrashStatusCodeFailed
			}
		} else {
			// Use cached settings, set by tests.
			// Make a copy to avoid side-effect modifications.
			*wcs = *(p.status)
		}

		// Transition to the next state.
		if wcs.StatusCode == WinCrashStatusCodeFailed {
			// Only try once and cache the failure.
			p.status = wcs
			p.state = failed
		} else if len(wcs.FileName) == 0 {
			// No filename means no crash dump
			p.status = wcs
			p.state = completed
			wcs.StatusCode = WinCrashStatusCodeSuccess
		} else {
			// Kick off the crash dump processing asynchronously.
			// The crash dump may be very large and we should not block for a response.
			p.state = busy
			wcs.StatusCode = WinCrashStatusCodeBusy

			// Make a new copy of the wcs for async processing while returning "Busy"
			// for the current response.
			p.status = &WinCrashStatus{
				FileName: wcs.FileName,
				Type:     wcs.Type,
			}

			go p.parseCrashDumpAsync()
		}

	case busy:
		// The crash dump processing is not done yet. Reply busy.
		if p.status != nil {
			wcs.FileName = p.status.FileName
			wcs.Type = p.status.Type
		}
		wcs.StatusCode = WinCrashStatusCodeBusy

	case failed:
		fallthrough
	case completed:
		// The crash dump processing was done, return the result.
		if p.status != nil {
			// This result is cached for all subsequent queries.
			wcs = p.status
		} else {
			wcs.StatusCode = WinCrashStatusCodeFailed
		}
	}

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
