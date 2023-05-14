// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testkubeutil

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	common "github.com/DataDog/datadog-agent/rtloader/test/common"
	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
	yaml "gopkg.in/yaml.v2"
)

/*
#include "rtloader_mem.h"
#include "datadog_agent_rtloader.h"

extern void getConnectionInfo(char **);

static void initKubeUtilTests(rtloader_t *rtloader) {
   set_cgo_free_cb(rtloader, _free);
   set_get_connection_info_cb(rtloader, getConnectionInfo);
}
*/
import "C"

var (
	rtloader   *C.rtloader_t
	tmpfile    *os.File
	returnNull bool
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

	// Updates sys.path so testing Check can be found
	C.add_python_path(rtloader, C.CString("../python"))

	if ok := C.init(rtloader); ok != 1 {
		return fmt.Errorf("`init` failed: %s", C.GoString(C.get_error(rtloader)))
	}

	C.initKubeUtilTests(rtloader)

	return nil
}

func tearDown() {
	os.Remove(tmpfile.Name())
}

func run(call string) (string, error) {
	tmpfile.Truncate(0)
	code := (*C.char)(helpers.TrackedCString(fmt.Sprintf(`
try:
	import kubeutil
	%s
except Exception as e:
	with open(r'%s', 'w') as f:
		f.write("{}\n".format(e))
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

	return string(output), err
}

//export getConnectionInfo
func getConnectionInfo(in **C.char) {
	if returnNull {
		return
	}

	h := map[string]string{
		"FooKey": "FooValue",
		"BarKey": "BarValue",
	}
	retval, _ := yaml.Marshal(h)

	*in = (*C.char)(helpers.TrackedCString(string(retval)))
}
