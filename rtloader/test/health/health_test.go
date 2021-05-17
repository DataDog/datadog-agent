package testhealth

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/StackVista/stackstate-agent/rtloader/test/helpers"
)

func TestMain(m *testing.M) {
	err := setUp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up tests: %v", err)
		os.Exit(-1)
	}

	os.Exit(m.Run())
}

const healthCheckData = `
{
  "key": "value ®",
  "stringlist": ["a", "b", "c"],
  "boollist": [True, False],
  "intlist": [1],
  "doublelist": [0.7, 1.42],
  "emptykey": None,
  "nestedobject": {"nestedkey": "nestedValue"}
}`

func testHealthCheckData(t *testing.T) {
	if _data["key"] != "value ®" {
		t.Fatalf("Unexpected component data 'key' value: %s", _data["key"])
	}
	var stringlist = _data["stringlist"].([]interface{})
	if len(stringlist) != 3 {
		t.Fatalf("Unexpected component data 'stringlist' size: %v", len(stringlist))
	}
	if stringlist[0] != "a" && stringlist[1] != "b"  && stringlist[2] != "c" {
		t.Fatalf("Unexpected component data 'stringlist' value: %s", _data["stringlist"])
	}
	var boollist = _data["boollist"].([]interface{})
	if len(boollist) != 2 {
		t.Fatalf("Unexpected component data 'boollist' size: %v", len(boollist))
	}
	if boollist[0] != true && boollist[1] != false {
		t.Fatalf("Unexpected component data 'boollist' value: %s", _data["boollist"])
	}
	var intlist = _data["intlist"].([]interface{})
	if len(intlist) != 1 {
		t.Fatalf("Unexpected component data 'intlist' size: %v", len(intlist))
	}
	if intlist[0] != 1 {
		t.Fatalf("Unexpected component data 'intlist' value: %s", _data["intlist"])
	}
	var doublelist = _data["doublelist"].([]interface{})
	if len(doublelist) != 2 {
		t.Fatalf("Unexpected component data 'doublelist' size: %v", len(doublelist))
	}
	if doublelist[0] != 0.7 && doublelist[1] != 1.42 {
		t.Fatalf("Unexpected component data 'doublelist' value: %s", _data["doublelist"])
	}
	if _data["emptykey"] != nil {
		t.Fatalf("Unexpected component data 'emptykey' value: %s", _data["emptykey"])
	}
	if _data["nestedobject"] == nil {
		t.Fatalf("Unexpected component data 'nestedobject' value: %s", _data["nestedobject"])
	}
	var nestedObj = _data["nestedobject"].(map[interface{}]interface{})
	if nestedObj["nestedkey"] != "nestedValue" {
		t.Fatalf("Unexpected component data 'nestedkey' value: %s", nestedObj["nestedkey"])
	}
}

func TestSubmitHealthCheckData(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`health.submit_health_check_data(None, "checkid", {"urn": "urn:", "sub_stream": "subStream"}, ` + healthCheckData + ` )`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	if checkID != "checkid" {
		t.Fatalf("Unexpected check id value: %s", checkID)
	}
	if _healthStream.Urn != "urn:" {
		t.Fatalf("Unexpected health stream urn value: %s", _healthStream.Urn)
	}
	if _healthStream.SubStream != "subStream" {
		t.Fatalf("Unexpected health stream sub stream value: %s", _healthStream.SubStream)
	}

	testHealthCheckData(t)

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSubmitHealthCheckDataNoDict(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`health.submit_health_check_data(None, "checkid", {"urn": "urn:", "sub_stream": "subStream"}, "I should be a dict")`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: health check data must be a dict" {
		t.Errorf("wrong printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSubmitHealthCheckDataCannotBeSerialized(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`health.submit_health_check_data(None, "checkid", {"urn": "urn:", "sub_stream": "subStream"}, {object(): object()})`)

	if err != nil {
		t.Fatal(err)
	}
	// example error 'RepresenterError: ('cannot represent an object', <object object at 0x7fc1df8f3e90>)'
	if !strings.HasPrefix(out, "RepresenterError: ('cannot represent an object'") {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestHealthStartSnapshot(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`health.submit_health_start_snapshot(None, "checkid", {"urn": "urn:", "sub_stream": "subStream"}, 0, 1)`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	if checkID != "checkid" {
		t.Fatalf("Unexpected check id value: %s", checkID)
	}

	if _healthStream.Urn != "urn:" {
		t.Fatalf("Unexpected health stream urn value: %s", _healthStream.Urn)
	}
	if _healthStream.SubStream != "subStream" {
		t.Fatalf("Unexpected health stream sub stream value: %s", _healthStream.SubStream)
	}
	if _expirySeconds != 0 {
		t.Fatalf("Unexpected expirySeconds: %d", _expirySeconds)
	}
	if _repeatIntervalSeconds != 1 {
		t.Fatalf("Unexpected repeateIntervalSeconds: %d", _repeatIntervalSeconds)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestStopSnapshot(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`health.submit_health_stop_snapshot(None, "checkid", {"urn": "urn:", "sub_stream": "subStream"})`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	if checkID != "checkid" {
		t.Fatalf("Unexpected check id value: %s", checkID)
	}
	if _healthStream.Urn != "urn:" {
		t.Fatalf("Unexpected health stream urn value: %s", _healthStream.Urn)
	}
	if _healthStream.SubStream != "subStream" {
		t.Fatalf("Unexpected health stream sub stream value: %s", _healthStream.SubStream)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}
