// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build windows

package probe

/*
#cgo LDFLAGS: -l dbgeng -static
#include "crashdump.h"
*/
import "C"
import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// allow us to change for testing
var readfn = doReadCrashDump

type logCallbackContext struct {
	loglines       []string
	hasSeenRetAddr bool
	unfinished     string
}

// maximum number of stack trace lines we'll look through, looking for non-"NT!" lines
const maxLinesToScan = int(200)

const (
	bugcheckCodePrefix     = "Bugcheck code"
	debugSessionTimePrefix = "Debug session time"
	retAddrPrefix          = "RetAddr"
	unableToLoadPrefix     = "Unable to"
	ntBangPrefix           = "nt!"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

/*
 * extra layer of indirection so that we can call the go parsing code
 * (logLineCallbackGo) straight from the test function so that we can
 * test out the parser with known input rather than actually calling
 * the debugger
 */
//export logLineCallback
func logLineCallback(voidctx C.PVOID, str C.PCSTR) {
	ctx := (*logCallbackContext)(unsafe.Pointer(uintptr(voidctx)))
	line := C.GoString(str)
	logLineCallbackGo(ctx, line)
}

func logLineCallbackGo(ctx *logCallbackContext, line string) {
	if !strings.Contains(line, "\n") {
		ctx.unfinished = ctx.unfinished + line
		return
	}
	if len(ctx.unfinished) != 0 {
		line = ctx.unfinished + line
		ctx.unfinished = ""
	}
	lines := strings.Split(line, "\n")

	// if the last line is _not_ empty, that means it did not end with a `\n`.  So save that
	// away for the next round
	numlines := len(lines)
	if len(lines[numlines-1]) != 0 {
		ctx.unfinished = lines[numlines-1]
		lines[numlines-1] = ""
	}
	start := int(0)
	if !ctx.hasSeenRetAddr {
		for idx, l := range lines {
			if strings.HasPrefix(l, bugcheckCodePrefix) {
				ctx.loglines = append(ctx.loglines, l)
			}
			if strings.HasPrefix(l, debugSessionTimePrefix) {
				ctx.loglines = append(ctx.loglines, l)
			}
			if strings.HasPrefix(l, retAddrPrefix) {
				ctx.hasSeenRetAddr = true
				start = idx
			}
		}
		if !ctx.hasSeenRetAddr {
			return
		}

	}
	ctx.loglines = append(ctx.loglines, lines[start:]...)
}

// this extra layer of indirection so that we can swap out test code which skips the actual debugger.
func doReadCrashDump(filename string, ctx *logCallbackContext, exterr *uint32) error {
	fnasCString := C.CString(filename)
	err := C.readCrashDump(fnasCString, unsafe.Pointer(ctx), (*C.long)(unsafe.Pointer(exterr)))

	C.free(unsafe.Pointer(fnasCString))

	if err != C.RCD_NONE {
		return fmt.Errorf("Error reading crash dump file %v", err)
	}
	return nil
}

func parseCrashDump(wcs *WinCrashStatus) {
	var ctx logCallbackContext
	var extendedError uint32

	err := readfn(wcs.FileName, &ctx, &extendedError)

	if err != nil {
		wcs.Success = false
		wcs.ErrString = fmt.Sprintf("Failed to load crash dump file %v %x", err, extendedError)
		log.Errorf("Failed to open crash dump %s: %v %x", wcs.FileName, err, extendedError)
		return
	}

	if len(ctx.loglines) < 2 {
		wcs.ErrString = fmt.Sprintf("Invalid crash dump file %s", wcs.FileName)
		wcs.Success = false
		return
	}

	// set a maximum of how many lines we'll scan looking for NT!.  The loglinecallback
	// above should strip off all the lines until the first `RetAddr` line.  So the number of
	// lines we need to see "should" be on the order of 5.  Set an (arbitrary) max that if
	// we don't find anything, we're not going to.

	/* expect the lines to look something like:
	Arguments ffffffff`c0000005 fffff806`f7e010e6 ffffb481`789326a8 ffffb481`78931ef0

	RetAddr           : Args to Child                                                           : Call Site
	fffff800`457f4db0 : 00000000`0000007e ffffffff`c0000005 fffff806`f7e010e6 ffffb481`789326a8 : nt!KeBugCheckEx
	fffff800`457cb7bf : 00000000`00000004 00000000`00000000 00007fff`ffff0000 ffffc582`1b4e3800 : nt!memset+0x5530
	fffff800`457e602d : ffffb481`78933000 ffffb481`789318c0 00000000`00000000 00000000`00000050 : nt!_C_specific_handler+0x9
	f
	fffff800`457742a1 : ffffb481`78933000 00000000`00000000 ffffb481`7892d000 00000000`00000000 : nt!_chkstk+0x5d
	fffff800`457730c4 : ffffb481`789326a8 ffffb481`789323f0 ffffb481`789326a8 ffffb481`78932570 : nt!KeQuerySystemTimePrecis
	e+0x27d1
	fffff800`457ee482 : 00003c74`00000000 fffff800`458a1d00 00000000`00000000 fffff800`45d940c4 : nt!KeQuerySystemTimePrecis
	e+0x15f4
	fffff800`457eafc0 : 00000000`00000000 fffff800`45a97fe0 ffff8301`2c077220 ffffc582`1bb72c30 : nt!setjmpex+0x7622
	fffff806`f7e010e6 : 00000000`00000001 00000000`00000000 ffffb481`76e2e000 fffff800`456e6511 : nt!setjmpex+0x4160
	*** ERROR: Module load completed but symbols could not be loaded for ddapmcrash.sys
	fffff806`f7e07020 : ffffc582`1bb72c30 ffffc582`19f18000 ffffc582`1bb72c30 ffff3ac8`f399d666 : ddapmcrash+0x10e6

	So, even though the loglinecallback will strip out, we might see the "symbols could not be loaded line".  We could
	also see additional RetAddr headers.*/

	end := min(len(ctx.loglines)-1, maxLinesToScan)
	for _, line := range ctx.loglines[:end] {

		if strings.HasPrefix(line, debugSessionTimePrefix) {
			_, after, found := strings.Cut(line, ":")
			if found {
				wcs.DateString = strings.TrimSpace(after)
			}
			continue
		}

		if strings.HasPrefix(line, bugcheckCodePrefix) {
			codeAsString := strings.TrimSpace(line[len(bugcheckCodePrefix)+1:])
			wcs.BugCheck = codeAsString
			continue
		}
		// skip lines that start with RetAddr, that's just the header
		if strings.HasPrefix(line, retAddrPrefix) {
			continue
		}
		// as shown above, there might be a stray "symbols could not be loaded line".  This would then
		// cause the split on ":" below  to not work, and then things would get worse from there.  so
		// just skip this line because it's expected.
		if strings.HasPrefix(line, unableToLoadPrefix) { // "Unable to load image, which is ok
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) != 3 {
			continue
		}
		callsite := strings.TrimSpace(parts[2])
		if strings.HasPrefix(callsite, ntBangPrefix) {
			// we're still in ntoskernel, keep looking
			continue
		}
		wcs.Offender = callsite
		break
	}
	wcs.Success = true
}
