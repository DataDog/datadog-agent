package testkubeutil

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	common "github.com/DataDog/datadog-agent/six/test/common"
)

// #cgo CFLAGS: -I../../include
// #cgo !windows LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #cgo windows LDFLAGS: -L../../six/ -ldatadog-agent-six -lstdc++ -static
// #include <datadog_agent_six.h>
//
// extern void getConnectionInfo(char **);
//
// static void initKubeUtilTests(six_t *six) {
//    set_get_connection_info_cb(six, getConnectionInfo);
// }
import "C"

var (
	six        *C.six_t
	tmpfile    *os.File
	returnNull bool
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

	C.initKubeUtilTests(six)

	// Updates sys.path so testing Check can be found
	C.add_python_path(six, C.CString("../python"))

	if ok := C.init(six); ok != 1 {
		return fmt.Errorf("`init` failed: %s", C.GoString(C.get_error(six)))
	}

	C.ensure_gil(six)
	return nil
}

func tearDown() {
	os.Remove(tmpfile.Name())
}

func run(call string) (string, error) {
	tmpfile.Truncate(0)
	code := C.CString(fmt.Sprintf(`
try:
	import kubeutil
	%s
except Exception as e:
	with open(r'%s', 'w') as f:
		f.write("{}\n".format(e))
`, call, tmpfile.Name()))

	ret := C.run_simple_string(six, code) == 1
	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	output, err := ioutil.ReadFile(tmpfile.Name())

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
	retval, _ := json.Marshal(h)

	*in = C.CString(string(retval))
}
