package testtagger

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"unsafe"

	common "github.com/DataDog/datadog-agent/rtloader/test/common"
)

// #cgo CFLAGS: -I../../include -I../../common
// #cgo !windows LDFLAGS: -L../../rtloader/ -ldatadog-agent-rtloader -ldl
// #cgo windows LDFLAGS: -L../../rtloader/ -ldatadog-agent-rtloader -lstdc++ -static
//
// #include "datadog_agent_rtloader.h"
// #include "memory.h"
//
// #include <stdlib.h>
//
// extern char **Tags(char*, int);
//
// static void initTaggerTests(rtloader_t *rtloader) {
//    set_tags_cb(rtloader, Tags);
// }
import "C"

var (
	rtloader *C.rtloader_t
	tmpfile  *os.File
)

func setUp() error {
	rtloader = (*C.rtloader_t)(common.GetRtLoader())
	if rtloader == nil {
		return fmt.Errorf("make failed")
	}

	var err error
	tmpfile, err = ioutil.TempFile("", "testout")
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
	code := C.CString(fmt.Sprintf(`
try:
	import tagger
	%s
except Exception as e:
	with open(r'%s', 'w') as f:
		f.write("{}: {}\n".format(type(e).__name__, e))
`, call, tmpfile.Name()))

	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	ret := C.run_simple_string(rtloader, code) == 1
	C._free(unsafe.Pointer(code))

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	output, err := ioutil.ReadFile(tmpfile.Name())
	return strings.TrimSpace(string(output)), err
}

//export Tags
func Tags(id *C.char, cardinality C.int) **C.char {
	goId := C.GoString(id)

	length := 4
	cTags := C._malloc(C.size_t(length) * C.size_t(unsafe.Sizeof(uintptr(0))))
	// convert the C array to a Go Array so we can index it
	indexTag := (*[1<<29 - 1]*C.char)(cTags)[:length:length]
	indexTag[length-1] = nil

	if goId != "base" {
		return nil
	}

	// dummy value for each cardinality value
	switch cardinality {
	case C.DATADOG_AGENT_RTLOADER_TAGGER_LOW:
		indexTag[0] = C.CString("a")
		indexTag[1] = C.CString("b")
		indexTag[2] = C.CString("c")
	case C.DATADOG_AGENT_RTLOADER_TAGGER_HIGH:
		indexTag[0] = C.CString("A")
		indexTag[1] = C.CString("B")
		indexTag[2] = C.CString("C")
	case C.DATADOG_AGENT_RTLOADER_TAGGER_ORCHESTRATOR:
		indexTag[0] = C.CString("1")
		indexTag[1] = C.CString("2")
		indexTag[2] = C.CString("3")
	default:
		C._free(cTags)
		return nil
	}
	return (**C.char)(cTags)
}
