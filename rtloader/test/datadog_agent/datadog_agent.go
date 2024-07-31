// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testdatadogagent

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
	"unsafe"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	common "github.com/DataDog/datadog-agent/rtloader/test/common"
	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
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
extern char* obfuscateSQL(char*, char*, char**);
extern char* obfuscateSQLExecPlan(char*, bool, char**);
extern double getProcessStartTime();
extern char* obfuscateMongoDBString(char*, char**);


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
   set_obfuscate_sql_exec_plan_cb(rtloader, obfuscateSQLExecPlan);
   set_get_process_start_time_cb(rtloader, getProcessStartTime);
   set_obfuscate_mongodb_string_cb(rtloader, obfuscateMongoDBString);
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
	tmpfile, err = os.CreateTemp("", "testout")
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
	import json
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

	output, err := os.ReadFile(tmpfile.Name())

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
	os.WriteFile(tmpfile.Name(), data, 0644)
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

// sqlConfig holds the config for the python SQL obfuscator.
type sqlConfig struct {
	// TableNames specifies whether the obfuscator should extract and return table names as SQL metadata when obfuscating.
	TableNames bool `json:"table_names"`
	// CollectCommands specifies whether the obfuscator should extract and return commands as SQL metadata when obfuscating.
	CollectCommands bool `json:"collect_commands"`
	// CollectComments specifies whether the obfuscator should extract and return comments as SQL metadata when obfuscating.
	CollectComments bool `json:"collect_comments"`
	// ReplaceDigits specifies whether digits in table names and identifiers should be obfuscated.
	ReplaceDigits bool `json:"replace_digits"`
	// KeepSQLAlias specifies whether or not to strip sql aliases while obfuscating.
	KeepSQLAlias bool `json:"keep_sql_alias"`
	// DollarQuotedFunc specifies whether or not to remove $func$ strings in postgres.
	DollarQuotedFunc bool `json:"dollar_quoted_func"`
	// ReturnJSONMetadata specifies whether the stub will return metadata as JSON.
	ReturnJSONMetadata bool `json:"return_json_metadata"`
}

//export obfuscateSQL
func obfuscateSQL(rawQuery, opts *C.char, errResult **C.char) *C.char {
	var sqlOpts sqlConfig
	optStr := C.GoString(opts)
	if optStr == "" {
		optStr = "{}"
	}
	if err := json.Unmarshal([]byte(optStr), &sqlOpts); err != nil {
		*errResult = (*C.char)(helpers.TrackedCString(err.Error()))
		return nil
	}
	s := C.GoString(rawQuery)
	switch s {
	case "select * from table where id = 1":
		obfuscatedQuery := obfuscate.ObfuscatedQuery{
			Query: "select * from table where id = ?",
			Metadata: obfuscate.SQLMetadata{
				TablesCSV: "table",
				Commands:  []string{"SELECT"},
				Comments:  []string{"-- SQL test comment"},
			},
		}
		out, err := json.Marshal(obfuscatedQuery)
		if err != nil {
			*errResult = (*C.char)(helpers.TrackedCString(err.Error()))
			return nil
		}
		return (*C.char)(helpers.TrackedCString(string(out)))
	// expected error results from obfuscator
	case "":
		*errResult = (*C.char)(helpers.TrackedCString("result is empty"))
		return nil
	default:
		*errResult = (*C.char)(helpers.TrackedCString("unknown test case"))
		return nil
	}
}

//export obfuscateSQLExecPlan
func obfuscateSQLExecPlan(rawQuery *C.char, normalize C.bool, errResult **C.char) *C.char {
	switch C.GoString(rawQuery) {
	case "raw-json-plan":
		if bool(normalize) {
			return (*C.char)(helpers.TrackedCString("obfuscated-and-normalized"))
		}

		// obfuscate only
		return (*C.char)(helpers.TrackedCString("obfuscated"))
	// expected error results from obfuscator
	case "":
		*errResult = (*C.char)(helpers.TrackedCString("empty"))
		return nil
	default:
		*errResult = (*C.char)(helpers.TrackedCString("unknown test case"))
		return nil
	}
}

var processStartTime = float64(time.Now().Unix())

//export getProcessStartTime
func getProcessStartTime() float64 {
	return processStartTime
}

//export obfuscateMongoDBString
func obfuscateMongoDBString(cmd *C.char, errResult **C.char) *C.char {
	switch C.GoString(cmd) {
	case "{\"find\": \"customer\"}":
		return (*C.char)(helpers.TrackedCString("{\"find\": \"customer\"}"))
	case "":
		*errResult = (*C.char)(helpers.TrackedCString("Empty MongoDB command"))
		return nil
	default:
		*errResult = (*C.char)(helpers.TrackedCString("unknown test case"))
		return nil
	}
}
