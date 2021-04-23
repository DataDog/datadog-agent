package testtopology

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"strings"
	"unsafe"

	common "github.com/StackVista/stackstate-agent/rtloader/test/common"
	"github.com/StackVista/stackstate-agent/rtloader/test/helpers"
	"gopkg.in/yaml.v2"
)

/*
#include "rtloader_mem.h"
#include "datadog_agent_rtloader.h"

extern void submitComponent(char *, instance_key_t *, char *, char *, char *);
extern void submitRelation(char *, instance_key_t *, char *, char *, char *, char *);
extern void submitStartSnapshot(char *, instance_key_t *);
extern void submitStopSnapshot(char *, instance_key_t *);

static void initTopologyTests(rtloader_t *rtloader) {
	set_submit_component_cb(rtloader, submitComponent);
	set_submit_relation_cb(rtloader, submitRelation);
	set_submit_start_snapshot_cb(rtloader, submitStartSnapshot);
	set_submit_stop_snapshot_cb(rtloader, submitStopSnapshot);
}
*/
import "C"

var (
	rtloader		*C.rtloader_t
	checkID			string
	_instance		*Instance
	_data			map[string]interface{}
	_externalID		string
	_componentType	string
	_sourceID		string
	_targetID		string
	_relationType	string
)

type Instance struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

func resetOuputValues() {
	checkID = ""
	_instance = nil
	_data = nil
	_externalID = ""
	_componentType = ""
	_sourceID = ""
	_targetID = ""
	_relationType = ""
}

func setUp() error {
	// Initialize memory tracking
	helpers.InitMemoryTracker()

	rtloader = (*C.rtloader_t)(common.GetRtLoader())
	if rtloader == nil {
		return fmt.Errorf("make failed")
	}

	C.initTopologyTests(rtloader)

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
	import topology
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

//export submitComponent
func submitComponent(id *C.char, instanceKey *C.instance_key_t, externalID *C.char, componentType *C.char, data *C.char) {
	checkID = C.GoString(id)

	_instance = &Instance{
		Type: C.GoString(instanceKey.type_),
		URL:  C.GoString(instanceKey.url),
	}

	_externalID = C.GoString(externalID)
	_componentType = C.GoString(componentType)
	_data = make(map[string]interface{})
	yaml.Unmarshal([]byte(C.GoString(data)), _data)
}

//export submitRelation
func submitRelation(id *C.char, instanceKey *C.instance_key_t, sourceID *C.char, targetID *C.char, relationType *C.char, data *C.char) {
	checkID = C.GoString(id)

	_instance = &Instance{
		Type: C.GoString(instanceKey.type_),
		URL:  C.GoString(instanceKey.url),
	}

	_sourceID = C.GoString(sourceID)
	_targetID = C.GoString(targetID)
	_relationType = C.GoString(relationType)

	_externalID = fmt.Sprintf("%s-%s-%s", _sourceID, _relationType, _targetID)

	_data = make(map[string]interface{})
	yaml.Unmarshal([]byte(C.GoString(data)), _data)
}

//export submitStartSnapshot
func submitStartSnapshot(id *C.char, instanceKey *C.instance_key_t) {
	checkID = C.GoString(id)

	_instance = &Instance{
		Type: C.GoString(instanceKey.type_),
		URL:  C.GoString(instanceKey.url),
	}
}

//export submitStopSnapshot
func submitStopSnapshot(id *C.char, instanceKey *C.instance_key_t) {
	checkID = C.GoString(id)

	_instance = &Instance{
		Type: C.GoString(instanceKey.type_),
		URL:  C.GoString(instanceKey.url),
	}
}