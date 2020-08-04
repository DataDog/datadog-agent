package testdatadogagent

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"unsafe"

	common "github.com/DataDog/datadog-agent/rtloader/test/common"
	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
	yaml "gopkg.in/yaml.v2"
)

/*
#include "rtloader_mem.h"
#include "datadog_agent_rtloader.h"

extern void doLog(char*, int);
extern void getClustername(char **);
extern void getConfig(char *, char **);
extern void getHostname(char **);
extern bool getTracemallocEnabled();
extern void getVersion(char **);
extern void headers(char **);
extern void setCheckMetadata(char*, char*, char*);
extern void setExternalHostTags(char*, char*, char**);
extern void writePersistentCache(char*, char*);
extern char* readPersistentCache(char*);
extern char* obfuscateSQL(char*, char**);


static void initDatadogAgentTests(rtloader_t *rtloader) {
   set_cgo_free_cb(rtloader, _free);
   set_get_clustername_cb(rtloader, getClustername);
   set_get_config_cb(rtloader, getConfig);
   set_get_hostname_cb(rtloader, getHostname);
   set_tracemalloc_enabled_cb(rtloader, getTracemallocEnabled);
   set_get_version_cb(rtloader, getVersion);
   set_headers_cb(rtloader, headers);
   set_log_cb(rtloader, doLog);
   set_set_check_metadata_cb(rtloader, setCheckMetadata);
   set_set_external_tags_cb(rtloader, setExternalHostTags);
   set_write_persistent_cache_cb(rtloader, writePersistentCache);
   set_read_persistent_cache_cb(rtloader, readPersistentCache);
   set_obfuscate_sql_cb(rtloader, obfuscateSQL);
}
*/
import "C"

var (
	rtloader *C.rtloader_t
	tmpfile  *os.File
)

type message struct {
	Name string `yaml:"name"`
	Body string `yaml:"body"`
	Time int64  `yaml:"time"`
}

func setUp() error {
	// Initialize memory tracking
	helpers.InitMemoryTracker()

	rtloader = (*C.rtloader_t)(common.GetRtLoader())
	if rtloader == nil {
		return fmt.Errorf("make failed")
	}

	var err error
	tmpfile, err = ioutil.TempFile("", "testout")
	if err != nil {
		return err
	}

	// Updates sys.path so testing Check can be found
	C.add_python_path(rtloader, C.CString("../python"))

	if ok := C.init(rtloader); ok != 1 {
		return fmt.Errorf("`init` failed: %s", C.GoString(C.get_error(rtloader)))
	}

	C.initDatadogAgentTests(rtloader)

	return nil
}

func tearDown() {
	os.Remove(tmpfile.Name())
}

func run(call string) (string, error) {
	tmpfile.Truncate(0)
	code := (*C.char)(helpers.TrackedCString(fmt.Sprintf(`
import sys
try:
	import datadog_agent
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

	output, err := ioutil.ReadFile(tmpfile.Name())

	return strings.TrimSpace(string(output)), err
}

//export getVersion
func getVersion(in **C.char) {
	*in = (*C.char)(helpers.TrackedCString("1.2.3"))
}

//export getConfig
func getConfig(key *C.char, in **C.char) {

	goKey := C.GoString(key)
	switch goKey {
	case "log_level":
		*in = (*C.char)(helpers.TrackedCString("\"warning\""))
	case "foo":
		m := message{C.GoString(key), "Hello", 123456}
		b, _ := yaml.Marshal(m)
		*in = (*C.char)(helpers.TrackedCString(string(b)))
	default:
		*in = (*C.char)(helpers.TrackedCString("null"))
	}
}

//export headers
func headers(in **C.char) {
	h := map[string]string{
		"User-Agent":   "Datadog Agent/0.99",
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "text/html, */*",
	}
	retval, _ := yaml.Marshal(h)

	*in = (*C.char)(helpers.TrackedCString(string(retval)))
}

//export getHostname
func getHostname(in **C.char) {
	*in = (*C.char)(helpers.TrackedCString("localfoobar"))
}

//export getClustername
func getClustername(in **C.char) {
	*in = (*C.char)(helpers.TrackedCString("the-cluster"))
}

//export getTracemallocEnabled
func getTracemallocEnabled() C.bool {
	return C.bool(true)
}

//export doLog
func doLog(msg *C.char, level C.int) {
	data := []byte(fmt.Sprintf("[%d]%s", int(level), C.GoString(msg)))
	ioutil.WriteFile(tmpfile.Name(), data, 0644)
}

//export setCheckMetadata
func setCheckMetadata(checkID, name, value *C.char) {
	cid := C.GoString(checkID)
	key := C.GoString(name)
	val := C.GoString(value)

	f, _ := os.OpenFile(tmpfile.Name(), os.O_APPEND|os.O_RDWR|os.O_CREATE, 0666)
	defer f.Close()

	f.WriteString(strings.Join([]string{cid, key, val}, ","))
}

//export setExternalHostTags
func setExternalHostTags(hostname *C.char, sourceType *C.char, tags **C.char) {
	hname := C.GoString(hostname)
	stype := C.GoString(sourceType)
	tagsStrings := []string{}

	pTags := uintptr(unsafe.Pointer(tags))
	ptrSize := unsafe.Sizeof(*tags)

	f, _ := os.OpenFile(tmpfile.Name(), os.O_APPEND|os.O_RDWR|os.O_CREATE, 0666)
	defer f.Close()

	f.WriteString(strings.Join([]string{hname, stype}, ","))

	// loop over the **char array
	for i := uintptr(0); ; i++ {
		tagPtr := *(**C.char)(unsafe.Pointer(pTags + ptrSize*i))
		if tagPtr == nil {
			break
		}
		tag := C.GoString(tagPtr)
		tagsStrings = append(tagsStrings, tag)
	}
	f.WriteString(",")
	f.WriteString(strings.Join(tagsStrings, ","))
	f.WriteString("\n")
}

//export writePersistentCache
func writePersistentCache(key, value *C.char) {
	keyName := C.GoString(key)
	val := C.GoString(value)

	f, _ := os.OpenFile(tmpfile.Name(), os.O_APPEND|os.O_RDWR|os.O_CREATE, 0600)
	defer f.Close()

	f.WriteString(keyName)
	f.WriteString(val)
}

//export readPersistentCache
func readPersistentCache(key *C.char) *C.char {
	return (*C.char)(helpers.TrackedCString("somevalue"))
}

//export obfuscateSQL
func obfuscateSQL(rawQuery *C.char, errResult **C.char) *C.char {
	s := C.GoString(rawQuery)
	switch s {
	case "select * from table where id = 1":
		return (*C.char)(helpers.TrackedCString("select * from table where id = ?"))
	// expected error results from obfuscator
	case "":
		*errResult = (*C.char)(helpers.TrackedCString("result is empty"))
		return nil
	default:
		*errResult = (*C.char)(helpers.TrackedCString("unknown test case"))
		return nil
	}
}
