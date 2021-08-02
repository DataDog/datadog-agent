package testhealth

import (
	"encoding/json"
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/health"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"strings"
	"unsafe"

	common "github.com/StackVista/stackstate-agent/rtloader/test/common"
	"github.com/StackVista/stackstate-agent/rtloader/test/helpers"
)

/*
#include "rtloader_mem.h"
#include "datadog_agent_rtloader.h"

extern void submitHealthCheckData(char *, health_stream_t *, char *);
extern void submitHealthStartSnapshot(char *, health_stream_t *, int, int);
extern void submitHealthStopSnapshot(char *, health_stream_t *);

static void initHealthTests(rtloader_t *rtloader) {
	set_submit_health_check_data_cb(rtloader, submitHealthCheckData);
	set_submit_health_start_snapshot_cb(rtloader, submitHealthStartSnapshot);
	set_submit_health_stop_snapshot_cb(rtloader, submitHealthStopSnapshot);
}
*/
import "C"

var (
	rtloader               *C.rtloader_t
	checkID                string
	_healthStream          *health.Stream
	_data                  map[string]interface{}
	result                 map[string]interface{}
	_expirySeconds         int
	_repeatIntervalSeconds int
)

func resetOuputValues() {
	checkID = ""
	_healthStream = nil
	_data = nil
	result = nil
	_expirySeconds = 0
	_repeatIntervalSeconds = 0
}

func setUp() error {
	// Initialize memory tracking
	helpers.InitMemoryTracker()

	rtloader = (*C.rtloader_t)(common.GetRtLoader())
	if rtloader == nil {
		return fmt.Errorf("make failed")
	}

	C.initHealthTests(rtloader)

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
	import health
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

//export submitHealthCheckData
func submitHealthCheckData(id *C.char, healthStream *C.health_stream_t, data *C.char) {
	checkID = C.GoString(id)
	_raw_data := C.GoString(data)
	healthPayload := &health.Payload{}
	json.Unmarshal([]byte(_raw_data), healthPayload)
	result = healthPayload.Data
	_healthStream = &healthPayload.Stream
}

//export submitHealthStartSnapshot
func submitHealthStartSnapshot(id *C.char, healthStream *C.health_stream_t, expirySeconds C.int, repeatIntervalSeconds C.int) {
	checkID = C.GoString(id)

	_healthStream = &health.Stream{
		Urn:       C.GoString(healthStream.urn),
		SubStream: C.GoString(healthStream.sub_stream),
	}

	_expirySeconds = int(expirySeconds)
	_repeatIntervalSeconds = int(repeatIntervalSeconds)
}

//export submitHealthStopSnapshot
func submitHealthStopSnapshot(id *C.char, healthStream *C.health_stream_t) {
	checkID = C.GoString(id)
	_healthStream = &health.Stream{
		Urn:       C.GoString(healthStream.urn),
		SubStream: C.GoString(healthStream.sub_stream),
	}
}
