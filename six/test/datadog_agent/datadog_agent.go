package testdatadogagent

import (
	"encoding/json"
	"fmt"
	"os"

	common "../common"
)

// #cgo CFLAGS: -I../../include
// #cgo LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #include <datadog_agent_six.h>
//
// extern void getVersion(char **);
// extern void getConfig(char *, char **);
//
// static void initDatadogAgentTests(six_t *six) {
//    set_get_version_cb(six, getVersion);
//    set_get_config_cb(six, getConfig);
// }
import "C"

var six *C.six_t

type message struct {
	Name string `json:"name"`
	Body string `json:"body"`
	Time int64  `json:"time"`
}

func setUp() error {
	if _, ok := os.LookupEnv("TESTING_TWO"); ok {
		six = C.make2()
		if six == nil {
			return fmt.Errorf("`make2` failed")
		}
	} else {
		six = C.make3()
		if six == nil {
			return fmt.Errorf("`make3` failed")
		}
	}

	C.initDatadogAgentTests(six)

	// Updates sys.path so testing Check can be found
	C.add_python_path(six, C.CString("../python"))

	if ok := C.init(six, nil); ok != 1 {
		return fmt.Errorf("`init` failed: %s", C.GoString(C.get_error(six)))
	}

	return nil
}

func tearDown() {
	C.destroy(six)
	six = nil
}

func run(call string) (string, error) {

	code := C.CString(fmt.Sprintf(`
try:
	import sys
	import datadog_agent
	%s
except Exception as e:
	sys.stderr.write("{}\n".format(e))
	sys.stderr.flush()
`, call))

	var (
		err    error
		ret    bool
		output []byte
	)

	output, err = common.Capture(func() {
		ret = C.run_simple_string(six, code) == 1
	})

	if err != nil {
		return "", err
	}

	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	return string(output), err
}

//export getVersion
func getVersion(in **C.char) {
	*in = C.CString("1.2.3")
}

//export getConfig
func getConfig(key *C.char, in **C.char) {
	m := message{C.GoString(key), "Hello", 123456}
	b, _ := json.Marshal(m)

	*in = C.CString(string(b))
}
