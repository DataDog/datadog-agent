package two_extend

import "fmt"
import common "../common"

// #cgo CFLAGS: -I../../include
// #cgo LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #include <datadog_agent_six.h>
//
// extern six_pyobject_t *printFoo();
// static void goSetupMyModule(six_t *six) {
//    add_module_func(six, DATADOG_AGENT_SIX_DATADOG_AGENT, DATADOG_AGENT_SIX_NOARGS, "print_foo", printFoo);
// }
import "C"

var six *C.six_t

//export printFoo
func printFoo() *C.six_pyobject_t {
	fmt.Println("I'm extending Python!")
	return C.get_none(six)
}

func extend() (string, error) {
	var err error
	six = C.make2()

	C.goSetupMyModule(six)
	C.add_module_int_const(six, C.DATADOG_AGENT_SIX_DATADOG_AGENT, C.CString("my_const"), 42)
	C.init(six, nil)

	code := C.CString(`
try:
	from datadog_agent import print_foo, my_const
	print_foo()
	print(my_const)
except Exception as e:
	print(e)
	`)
	var ret bool
	var output []byte
	output, err = common.Capture(func() {
		ret = C.run_simple_string(six, code) == 0
	})

	if err != nil {
		return "", err
	}

	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	return string(output), err
}
