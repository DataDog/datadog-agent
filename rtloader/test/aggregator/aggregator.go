package testaggregator

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"strings"
	"unsafe"

	common "github.com/DataDog/datadog-agent/rtloader/test/common"
	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
)

/*
#include "rtloader_mem.h"
#include "datadog_agent_rtloader.h"

extern void submitMetric(char *, metric_type_t, char *, double, char **, char *);
extern void submitServiceCheck(char *, char *, int, char **, char *, char *);
extern void submitEvent(char*, event_t*);
extern void submitHistogramBucket(char *, char *, long long, float, float, int, char *, char **);

static void initAggregatorTests(rtloader_t *rtloader) {
   set_submit_metric_cb(rtloader, submitMetric);
   set_submit_service_check_cb(rtloader, submitServiceCheck);
   set_submit_event_cb(rtloader, submitEvent);
   set_submit_histogram_bucket_cb(rtloader, submitHistogramBucket);
}
*/
import "C"

var (
	rtloader   *C.rtloader_t
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
	intValue   int
	lowerBound float64
	upperBound float64
	monotonic  bool
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
	intValue = -1
	lowerBound = 1.0
	upperBound = 1.0
	monotonic = false
}

func setUp() error {
	// Initialize memory tracking
	helpers.InitMemoryTracker()

	rtloader = (*C.rtloader_t)(common.GetRtLoader())
	if rtloader == nil {
		return fmt.Errorf("make failed")
	}

	C.initAggregatorTests(rtloader)

	// Updates sys.path so testing Check can be found
	C.add_python_path(rtloader, C.CString("../python"))

	if ok := C.init(rtloader); ok != 1 {
		return fmt.Errorf("`init` failed: %s", C.GoString(C.get_error(rtloader)))
	}

	return nil
}

func run(call string) (string, error) {
	resetOuputValues()
	tmpfile, err := ioutil.TempFile("", "testout")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	code := (*C.char)(helpers.TrackedCString(fmt.Sprintf(`
try:
	import aggregator
	%s
except Exception as e:
	with open(r'%s', 'w') as f:
		f.write("{}: {}\n".format(type(e).__name__, e))
`, call, tmpfile.Name())))
	defer C._free(unsafe.Pointer(code))

	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)

	ret := C.run_simple_string(rtloader, code) == 1

	C.release_gil(rtloader, state)
	runtime.UnlockOSThread()

	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	var output []byte
	output, err = ioutil.ReadFile(tmpfile.Name())

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

//export submitMetric
func submitMetric(id *C.char, mt C.metric_type_t, mname *C.char, val C.double, t **C.char, hname *C.char) {
	checkID = C.GoString(id)
	metricType = int(mt)
	name = C.GoString(mname)
	value = float64(val)
	hostname = C.GoString(hname)
	if t != nil {
		tags = append(tags, charArrayToSlice(t)...)
	}
}

//export submitServiceCheck
func submitServiceCheck(id *C.char, name *C.char, level C.int, t **C.char, hname *C.char, message *C.char) {
	checkID = C.GoString(id)
	scLevel = int(level)
	scName = C.GoString(name)
	hostname = C.GoString(hname)
	scMessage = C.GoString(message)
	if t != nil {
		tags = append(tags, charArrayToSlice(t)...)
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
		_event.tags = append(_event.tags, charArrayToSlice(ev.tags)...)
	}
}

//export submitHistogramBucket
func submitHistogramBucket(id *C.char, cMetricName *C.char, cVal C.longlong, cLowerBound C.float, cUpperBound C.float, cMonotonic C.int, cHostname *C.char, t **C.char) {
	checkID = C.GoString(id)
	name = C.GoString(cMetricName)
	intValue = int(cVal)
	lowerBound = float64(cLowerBound)
	upperBound = float64(cUpperBound)
	monotonic = (cMonotonic != 0)
	hostname = C.GoString(cHostname)
	if t != nil {
		tags = append(tags, charArrayToSlice(t)...)
	}
}
