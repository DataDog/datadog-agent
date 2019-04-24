package testtagger

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
// extern char **Tags(char*, int);
//
// static void initTaggerTests(six_t *six) {
//    set_tags_cb(six, Tags);
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

	C.initTaggerTests(six)

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
	import tagger
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

//export Tags
func Tags(id *C.char, cardinality C.int) **C.char {
	goId := C.GoString(id)

	length := 4
	cTags := C.malloc(C.size_t(length) * C.size_t(unsafe.Sizeof(uintptr(0))))
	// convert the C array to a Go Array so we can index it
	indexTag := (*[1<<29 - 1]*C.char)(cTags)[:length:length]
	indexTag[length-1] = nil

	if goId != "base" {
		return nil
	}

	// dummy value for each cardinality value
	switch cardinality {
	case C.DATADOG_AGENT_SIX_TAGGER_LOW:
		indexTag[0] = C.CString("a")
		indexTag[1] = C.CString("b")
		indexTag[2] = C.CString("c")
	case C.DATADOG_AGENT_SIX_TAGGER_HIGH:
		indexTag[0] = C.CString("A")
		indexTag[1] = C.CString("B")
		indexTag[2] = C.CString("C")
	case C.DATADOG_AGENT_SIX_TAGGER_ORCHESTRATOR:
		indexTag[0] = C.CString("1")
		indexTag[1] = C.CString("2")
		indexTag[2] = C.CString("3")
	default:
		C.free(cTags)
		return nil
	}
	return (**C.char)(cTags)
}
