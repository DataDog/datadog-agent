package testaggregator

import (
	"fmt"
	"os"
	"unsafe"

	common "../common"
)

// #cgo CFLAGS: -I../../include
// #cgo LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #include <datadog_agent_six.h>
//
// extern void submitMetric(char *, metric_type_t, char *, float, char **, int, char *);
// extern void submitServiceCheck(char *, char *, int, char **, int, char *, char *);
//
// static void initAggregator(six_t *six) {
//    set_submit_metric_cb(six, submitMetric);
//    set_submit_service_check_cb(six, submitServiceCheck);
// }
import "C"

var (
	six        *C.six_t
	checkID    string
	metricType int
	name       string
	value      float64
	tags       []string
	hostname   string
	scLevel    int
	scName     string
	scMessage  string
)

func resetOuputValues() {
	checkID = ""
	metricType = -1
	name = ""
	value = -1
	tags = []string{}
	hostname = ""
	scLevel = -1
	scName = ""
	scMessage = ""
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

	C.initAggregator(six)

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
	resetOuputValues()

	code := C.CString(fmt.Sprintf(`
try:
	import aggregator
	%s
except Exception as e:
	print(e, flush=True)
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

//export submitMetric
func submitMetric(id *C.char, mt C.metric_type_t, mname *C.char, val C.float, t **C.char, tagsLen C.int, hname *C.char) {
	checkID = C.GoString(id)
	metricType = int(mt)
	name = C.GoString(mname)
	value = float64(val)
	hostname = C.GoString(hname)
	if t != nil {
		for _, s := range (*[1 << 30]*C.char)(unsafe.Pointer(t))[:tagsLen:tagsLen] {
			tags = append(tags, C.GoString(s))
		}
	}
}

//export submitServiceCheck
func submitServiceCheck(id *C.char, name *C.char, level C.int, t **C.char, tagsLen C.int, hname *C.char, message *C.char) {
	checkID = C.GoString(id)
	scLevel = int(level)
	scName = C.GoString(name)
	hostname = C.GoString(hname)
	scMessage = C.GoString(message)
	if t != nil {
		for _, s := range (*[1 << 30]*C.char)(unsafe.Pointer(t))[:tagsLen:tagsLen] {
			tags = append(tags, C.GoString(s))
		}
	}
}
