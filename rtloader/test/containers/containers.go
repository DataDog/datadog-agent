// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testcontainers

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

extern int is_excluded(char *, char *, char *, char *);

static void initContainersTests(rtloader_t *rtloader) {
   set_is_excluded_cb(rtloader, is_excluded);
}
*/
import "C"

var (
	rtloader *C.rtloader_t
	tmpfile  *os.File
)

type message struct {
	Name string `yaml:"name"`
	Body string `yaml:"body"`
	Time int64  `yaml:"time"`
}

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

	C.initContainersTests(rtloader)

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
	import containers
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

//export is_excluded
func is_excluded(annotation *C.char, name *C.char, image *C.char, namespace *C.char) C.int {
	if C.GoString(name) == "foo" {
		return 1
	}
	return 0
}
