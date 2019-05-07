package testcontainers

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
//
// #include <stdlib.h>
// #include <datadog_agent_six.h>
//
// extern int is_excluded(char *, char*);
//
// static void initContainersTests(six_t *six) {
//    set_is_excluded_cb(six, is_excluded);
// }
import "C"

var (
	six     *C.six_t
	tmpfile *os.File
)

type message struct {
	Name string `yaml:"name"`
	Body string `yaml:"body"`
	Time int64  `yaml:"time"`
}

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

	C.initContainersTests(six)

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
	import containers
	%s
except Exception as e:
	with open(r'%s', 'w') as f:
		f.write("{}: {}\n".format(type(e).__name__, e))
`, call, tmpfile.Name()))

	runtime.LockOSThread()
	state := C.ensure_gil(six)

	ret := C.run_simple_string(six, code) == 1
	C.free(unsafe.Pointer(code))

	C.release_gil(six, state)
	runtime.UnlockOSThread()

	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	output, err := ioutil.ReadFile(tmpfile.Name())

	return strings.TrimSpace(string(output)), err
}

//export is_excluded
func is_excluded(name *C.char, image *C.char) C.int {
	if C.GoString(name) == "foo" {
		return 1
	}
	return 0
}
