package testdatadogagent

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

// #cgo CFLAGS: -I../../include
// #cgo linux LDFLAGS: -L../../six/ -ldatadog-agent-six -ldl
// #cgo windows LDFLAGS: -L../../six/ -ldatadog-agent-six -lstdc++ -static
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

var (
	six     *C.six_t
	tmpfile *os.File
)

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

	var err error
	tmpfile, err = ioutil.TempFile("", "testout")
	if err != nil {
		return err
	}

	C.initDatadogAgentTests(six)

	// Updates sys.path so testing Check can be found
	C.add_python_path(six, C.CString("../python"))

	if ok := C.init(six, nil); ok != 1 {
		return fmt.Errorf("`init` failed: %s", C.GoString(C.get_error(six)))
	}

	C.ensure_gil(six)
	return nil
}

func tearDown() {
	os.Remove(tmpfile.Name())
}

func run(call string) (string, error) {
	tmpfile.Truncate(0)
	code := C.CString(fmt.Sprintf(`
try:
	import datadog_agent
	%s
except Exception as e:
	with open(r'%s', 'w') as f:
		f.write("{}\n".format(e))
`, call, tmpfile.Name()))

	ret := C.run_simple_string(six, code) == 1
	if !ret {
		return "", fmt.Errorf("`run_simple_string` errored")
	}

	output, err := ioutil.ReadFile(tmpfile.Name())

	return string(output), err
}

//export getVersion
func getVersion(in **C.char) {
	*in = C.CString("1.2.3")
}

//export getConfig
func getConfig(key *C.char, in **C.char) {

	goKey := C.GoString(key)
	switch goKey {
	case "log_level":
		*in = C.CString("\"warning\"")
	case "foo":
		m := message{C.GoString(key), "Hello", 123456}
		b, _ := json.Marshal(m)
		*in = C.CString(string(b))
	default:
		*in = C.CString("null")
	}
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
	data := []byte(fmt.Sprintf("[%d]%s", int(level), C.GoString(msg)))
	ioutil.WriteFile(tmpfile.Name(), data, 0644)
}

//export setExternalHostTags
func setExternalHostTags(dump *C.char) {
	jsonData := []byte(C.GoString(dump))
	type HostTag []interface{}
	var hostTags []HostTag
	err := json.Unmarshal(jsonData, &hostTags)
	if err != nil {
		ioutil.WriteFile(tmpfile.Name(), []byte(fmt.Sprintf("Error: %s", err)), 0644)
		return
	}

	var f *os.File
	f, err = os.Create(tmpfile.Name())
	defer f.Close()

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
		f.WriteString(strings.Join(out, ","))
		f.WriteString("\n")
	}
}
