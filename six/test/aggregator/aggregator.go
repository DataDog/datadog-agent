package testaggregator

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"unsafe"

	common "github.com/DataDog/datadog-agent/six/test/common"
)

// #cgo CFLAGS: -I../../include
// #cgo !windows LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #cgo windows LDFLAGS: -L../../six/ -ldatadog-agent-six -lstdc++ -static
// #include <datadog_agent_six.h>
//
// extern void submitMetric(char *, metric_type_t, char *, float, char **, int, char *);
// extern void submitServiceCheck(char *, char *, int, char **, int, char *, char *);
// extern void submitEvent(char*, event_t*);
//
// static void initAggregatorTests(six_t *six) {
//    set_submit_metric_cb(six, submitMetric);
//    set_submit_service_check_cb(six, submitServiceCheck);
//    set_submit_event_cb(six, submitEvent);
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
	_event     *event
)

type event struct {
	title          string
	text           string
	ts             int64
	priority       string
	host           string
	tags           []string
	alertType      string
	aggregationKey string
	sourceTypeName string
	eventType      string
}

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
	_event = nil
}

func setUp() error {
	six = (*C.six_t)(common.GetSix())
	if six == nil {
		return fmt.Errorf("make failed")
	}

	C.initAggregatorTests(six)

	// Updates sys.path so testing Check can be found
	C.add_python_path(six, C.CString("../python"))

	if ok := C.init(six); ok != 1 {
		return fmt.Errorf("`init` failed: %s", C.GoString(C.get_error(six)))
	}

	C.ensure_gil(six)
	return nil
}

func run(call string) (string, error) {
	resetOuputValues()
	tmpfile, err := ioutil.TempFile("", "testout")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	code := C.CString(fmt.Sprintf(`
try:
	import aggregator
	%s
except Exception as e:
	with open(r'%s', 'w') as f:
		f.write("{}\n".format(e))
`, call, tmpfile.Name()))

	ret := C.run_simple_string(six, code) == 1
	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	var output []byte
	output, err = ioutil.ReadFile(tmpfile.Name())

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

//export submitEvent
func submitEvent(id *C.char, ev *C.event_t) {
	checkID = C.GoString(id)
	_event = &event{}
	if ev.title != nil {
		_event.title = C.GoString(ev.title)
	}
	if ev.text != nil {
		_event.text = C.GoString(ev.text)
	}
	_event.ts = int64(ev.ts)
	if ev.priority != nil {
		_event.priority = C.GoString(ev.priority)
	}
	if ev.host != nil {
		_event.host = C.GoString(ev.host)
	}
	if ev.alert_type != nil {
		_event.alertType = C.GoString(ev.alert_type)
	}
	if ev.aggregation_key != nil {
		_event.aggregationKey = C.GoString(ev.aggregation_key)
	}
	if ev.source_type_name != nil {
		_event.sourceTypeName = C.GoString(ev.source_type_name)
	}
	if ev.event_type != nil {
		_event.eventType = C.GoString(ev.event_type)
	}

	if ev.tags != nil {
		for _, s := range (*[1 << 30]*C.char)(unsafe.Pointer(ev.tags))[:ev.tags_num:ev.tags_num] {
			_event.tags = append(_event.tags, C.GoString(s))
		}
	}
}
