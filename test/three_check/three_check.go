package three_check

// #cgo CFLAGS: -I../../include
// #cgo LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #include <datadog_agent_six.h>
//
import "C"

var six *C.six_t

func getFakeAgentCheck() *C.six_pyobject_t {
	if six == nil {
		six = C.make3()
	}

	if C.is_initialized(six) == 0 {
		C.init(six, nil)
	}

	// Updates sys.path so Agent for testing can be found
	code := C.CString("import sys; sys.path.insert(0, '../python/')")
	success := C.run_simple_string(six, code)
	if success != 0 {
		return nil
	}

	return C.get_check(six, C.CString("fake_agent"), C.CString(""), C.CString("[{fake_agent: \"/\"}]"))
}

func runFakeAgentCheck() string {
	if six == nil {
		six = C.make3()
	}

	if C.is_initialized(six) == 0 {
		C.init(six, nil)
	}

	// Updates sys.path so Agent for testing can be found
	code := C.CString("import sys; sys.path.insert(0, '../python/')")
	success := C.run_simple_string(six, code)
	if success != 0 {
		return ""
	}

	check := C.get_check(six, C.CString("fake_agent"), C.CString(""), C.CString("[{fake_agent: \"/\"}]"))

	return C.GoString(C.run_check(six, check))
}

func getCheckClass(moduleName string) *C.six_pyobject_t {
	if six == nil {
		six = C.make3()
	}

	if C.is_initialized(six) == 0 {
		C.init(six, nil)
	}

	// Updates sys.path so testing Check can be found
	code := C.CString("import sys; sys.path.insert(0, '../python/')")
	success := C.run_simple_string(six, code)
	if success != 0 {
		return nil
	}

	return C.get_check_class(six, C.CString(moduleName))
}
