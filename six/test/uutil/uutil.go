package testuutil

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"unsafe"

	common "github.com/DataDog/datadog-agent/six/test/common"
)

// #cgo CFLAGS: -I../../include
// #cgo !windows LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #cgo windows LDFLAGS: -L../../six/ -ldatadog-agent-six -lstdc++ -static
// #include <datadog_agent_six.h>
//
// extern void getSubprocessOutput(char **, char **, char **, int*, char **);
//
// static void init_utilTests(six_t *six) {
//    set_get_subprocess_output_cb(six, getSubprocessOutput);
// }
import "C"

var (
	six     *C.six_t
	tmpfile *os.File
)

func setUp() error {
	six = (*C.six_t)(common.GetSix())
	if six == nil {
		return fmt.Errorf("make failed")
	}

	var err error
	tmpfile, err = ioutil.TempFile("", "testout")
	if err != nil {
		return err
	}

	C.init_utilTests(six)

	// Updates sys.path so testing Check can be found
	C.add_python_path(six, C.CString("../python"))

	if ok := C.init(six); ok != 1 {
		return fmt.Errorf("`init` failed: %s", C.GoString(C.get_error(six)))
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
	import _util
	import sys
	%s
except Exception as e:
	with open(r'%s', 'w') as f:
		f.write("{}: {}\n".format(type(e).__name__, e))
`, call, tmpfile.Name()))

	runtime.LockOSThread()
	state := C.ensure_gil(six)

	ret := C.run_simple_string(six, code) == 1

	C.release_gil(six, state)
	runtime.UnlockOSThread()

	resetTest()
	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	output, err := ioutil.ReadFile(tmpfile.Name())

	return strings.TrimSpace(string(output)), err
}

func charArrayToSlice(array **C.char) (res []string) {
	pTags := uintptr(unsafe.Pointer(array))
	ptrSize := unsafe.Sizeof(*array)

	for i := uintptr(0); ; i++ {
		tagPtr := *(**C.char)(unsafe.Pointer(pTags + ptrSize*i))
		if tagPtr == nil {
			return
		}
		tag := C.GoString(tagPtr)
		res = append(res, tag)
	}
}

//export getSubprocessOutput
func getSubprocessOutput(cargs **C.char, cstdout **C.char, cstderr **C.char, cretCode *C.int, cexception **C.char) {
	args = charArrayToSlice(cargs)
	*cstdout = C.CString(stdout)
	*cstderr = C.CString(stderr)
	*cretCode = C.int(retCode)
	if setException {
		*cexception = C.CString(exception)
	}
}
