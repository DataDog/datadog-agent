// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testtagger

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"unsafe"

	common "github.com/DataDog/datadog-agent/rtloader/test/common"
	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
)

/*
#include "rtloader_mem.h"
#include "datadog_agent_rtloader.h"

extern char **Tags(char*, int);

static void initTaggerTests(rtloader_t *rtloader) {
   set_cgo_free_cb(rtloader, _free);
   set_tags_cb(rtloader, Tags);
}

*/
import "C"

var (
	rtloader *C.rtloader_t
	tmpfile  *os.File
)

func setUp() error {
	// Initialize memory tracking
	helpers.InitMemoryTracker()

	rtloader = (*C.rtloader_t)(common.GetRtLoader())
	if rtloader == nil {
		return fmt.Errorf("make failed")
	}

	var err error
	tmpfile, err = os.CreateTemp("", "testout")
	if err != nil {
		return err
	}

	C.initTaggerTests(rtloader)

	// Updates sys.path so testing Check can be found
	C.add_python_path(rtloader, C.CString("../python"))

	if ok := C.init(rtloader); ok != 1 {
		return fmt.Errorf("`init` failed: %s", C.GoString(C.get_error(rtloader)))
	}

	return nil
}

func tearDown() {
	os.Remove(tmpfile.Name())
}

func run(call string) (string, error) {
	tmpfile.Truncate(0)
	code := (*C.char)(helpers.TrackedCString(fmt.Sprintf(`
try:
	import tagger
	%s
except Exception as e:
	with open(r'%s', 'w') as f:
		f.write("{}: {}\n".format(type(e).__name__, e))
`, call, tmpfile.Name())))
	defer C._free(unsafe.Pointer(code))

	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	ret := C.run_simple_string(rtloader, code) == 1

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	output, err := os.ReadFile(tmpfile.Name())
	return strings.TrimSpace(string(output)), err
}

//revive:disable
//export Tags
func Tags(id *C.char, cardinality C.int) **C.char {
	goID := C.GoString(id)

	if goID != "base" {
		return nil
	}

	length := 4
	cTags := C._malloc(C.size_t(length) * C.size_t(unsafe.Sizeof(uintptr(0))))
	// convert the C array to a Go Array so we can index it
	indexTag := (*[1<<29 - 1]*C.char)(cTags)[:length:length]
	indexTag[length-1] = nil

	// dummy value for each cardinality value
	switch cardinality {
	case C.DATADOG_AGENT_RTLOADER_TAGGER_LOW:
		indexTag[0] = (*C.char)(helpers.TrackedCString("a"))
		indexTag[1] = (*C.char)(helpers.TrackedCString("b"))
		indexTag[2] = (*C.char)(helpers.TrackedCString("c"))
	case C.DATADOG_AGENT_RTLOADER_TAGGER_HIGH:
		indexTag[0] = (*C.char)(helpers.TrackedCString("A"))
		indexTag[1] = (*C.char)(helpers.TrackedCString("B"))
		indexTag[2] = (*C.char)(helpers.TrackedCString("C"))
	case C.DATADOG_AGENT_RTLOADER_TAGGER_ORCHESTRATOR:
		indexTag[0] = (*C.char)(helpers.TrackedCString("1"))
		indexTag[1] = (*C.char)(helpers.TrackedCString("2"))
		indexTag[2] = (*C.char)(helpers.TrackedCString("3"))
	default:
		C._free(cTags)
		return nil
	}
	return (**C.char)(cTags)
}

//revive:enable
