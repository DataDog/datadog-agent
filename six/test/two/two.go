package two

// #cgo CFLAGS: -I../../include
// #cgo LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #include <datadog_agent_six.h>
//
// extern six_pyobject_t *printFoo();
// static void goSetupMyModule(six_t *six) {
//     add_module_func(six, DATADOG_AGENT_SIX_DATADOG_AGENT, DATADOG_AGENT_SIX_NOARGS, "print_foo", printFoo);
// }
import "C"

import (
	"fmt"

	common "../common"
)

var six *C.six_t

func get2() *C.six_t {
	six := C.make2()
	C.init(six, nil)
	return six
}

func init2() error {
	six := C.make2()
	if six == nil {
		return fmt.Errorf("`make2` failed")
	}

	C.init(six, nil)
	if C.is_initialized(six) != 1 {
		return fmt.Errorf("Six not initialized")
	}

	return nil
}

func getVersion() string {
	six := get2()
	ret := C.GoString(C.get_py_version(six))
	C.destroy2(six)
	return ret
}

func runString(code string) (string, error) {
	var ret bool
	var err error
	var output []byte
	output, err = common.Capture(func() {
		six := get2()
		ret = C.run_simple_string(six, C.CString(code)) == 0
		C.destroy2(six)
	})

	if err != nil {
		return "", err
	}

	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	return string(output), err
}

//export printFoo
func printFoo() *C.six_pyobject_t {
	fmt.Println("I'm extending Python!")
	return C.get_none(six)
}

func extend() (string, error) {
	var err error
	six = C.make2()

	// cb := printFoo
	C.goSetupMyModule(six)
	// C.add_module_func_noargs(six, C.CString("my_module"), C.CString("print_foo"), C.goPrintFoo)
	C.init(six, nil)

	code := C.CString(`
try:
	import datadog_agent
	datadog_agent.print_foo()
except Exception as e:
	print(e)
`)
	var ret bool
	var output []byte
	output, err = common.Capture(func() {
		ret = C.run_simple_string(six, code) == 0
	})

	if err != nil {
		C.destroy2(six)
		return "", err
	}

	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	C.destroy2(six)
	return string(output), err
}
