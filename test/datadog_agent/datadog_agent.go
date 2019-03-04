package testdatadogagent

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	common "../common"
)

// #cgo CFLAGS: -I../../include
// #cgo LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #include <datadog_agent_six.h>
//
// extern void getVersion(char **);
// extern void getConfig(char *, char **);
// extern void headers(char **);
// extern void getHostname(char **);
// extern void getClustername(char **);
// extern void doLog(char*, int);
// extern void setExternalHostTags(char*);
//
// static void initDatadogAgentTests(six_t *six) {
//    set_get_version_cb(six, getVersion);
//    set_get_config_cb(six, getConfig);
//    set_headers_cb(six, headers);
//    set_get_hostname_cb(six, getHostname);
//    set_get_clustername_cb(six, getClustername);
//    set_log_cb(six, doLog);
//    set_set_external_tags_cb(six, setExternalHostTags);
// }
import "C"

var six *C.six_t

type message struct {
	Name string `json:"name"`
	Body string `json:"body"`
	Time int64  `json:"time"`
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

	C.initDatadogAgentTests(six)

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

	code := C.CString(fmt.Sprintf(`
try:
	import sys
	import datadog_agent
	%s
except Exception as e:
	sys.stderr.write("{}\n".format(e))
	sys.stderr.flush()
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

//export getVersion
func getVersion(in **C.char) {
	*in = C.CString("1.2.3")
}

//export getConfig
func getConfig(key *C.char, in **C.char) {
	m := message{C.GoString(key), "Hello", 123456}
	b, _ := json.Marshal(m)

	*in = C.CString(string(b))
}

//export headers
func headers(in **C.char) {
	h := map[string]string{
		"User-Agent":   "Datadog Agent/0.99",
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "text/html, */*",
	}
	retval, _ := json.Marshal(h)

	*in = C.CString(string(retval))
}

//export getHostname
func getHostname(in **C.char) {
	*in = C.CString("localfoobar")
}

//export getClustername
func getClustername(in **C.char) {
	*in = C.CString("the-cluster")
}

//export doLog
func doLog(msg *C.char, level C.int) {
	fmt.Printf("[%d]%s", int(level), C.GoString(msg))
	// in a real implementation, msg should be freed
}

//export setExternalHostTags
func setExternalHostTags(dump *C.char) {
	jsonData := []byte(C.GoString(dump))
	type HostTag []interface{}
	var hostTags []HostTag
	err := json.Unmarshal(jsonData, &hostTags)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	for _, t := range hostTags {
		out := []string{}

		// hostname
		out = append(out, t[0].(string))
		stypes := t[1].(map[string]interface{})
		for k, v := range stypes {
			// source type
			out = append(out, k)
			// tags
			tags := v.([]interface{})
			for _, tag := range tags {
				out = append(out, tag.(string))
			}
		}
		fmt.Println(strings.Join(out, ","))
	}

	// in a real implementation, dump should be freed
}
