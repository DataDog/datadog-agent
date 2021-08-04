package testhealth

import (
	"fmt"
	"github.com/stretchr/testify/assert"
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
  "stringlist": ["a", "b", "c", "04", "09"],
  "boollist": [True, False],
  "intlist": [1],
  "doublelist": [0.7, 1.42],
  "emptykey": None,
  "nestedobject": {"nestedkey": "nestedValue"}
}`

func testHealthCheckData(t *testing.T) {
	if result["key"] != "value ®" {
		t.Fatalf("Unexpected component data 'key' value: %s", result["key"])
	}
	var stringlist = result["stringlist"].([]interface{})
	if len(stringlist) != 5 {
		t.Fatalf("Unexpected component data 'stringlist' size: %v", len(stringlist))
	}
	if assert.ObjectsAreEqualValues(stringlist, []string{"a", "b", "c", "04", "09"}) {
		t.Fatalf("Unexpected component data 'stringlist' value: %s", result["stringlist"])
	}
	var boollist = result["boollist"].([]interface{})
	if len(boollist) != 2 {
		t.Fatalf("Unexpected component data 'boollist' size: %v", len(boollist))
	}
	if assert.ObjectsAreEqualValues(boollist, []bool{true, false}) {
		t.Fatalf("Unexpected component data 'boollist' value: %s", result["boollist"])
	}
	var intlist = result["intlist"].([]interface{})
	if len(intlist) != 1 {
		t.Fatalf("Unexpected component data 'intlist' size: %v", len(intlist))
	}
	if assert.ObjectsAreEqualValues(intlist, []int64{1}) {
		t.Fatalf("Unexpected component data 'intlist' value: %s", result["intlist"])
	}
	var doublelist = result["doublelist"].([]interface{})
	if len(doublelist) != 2 {
		t.Fatalf("Unexpected component data 'doublelist' size: %v", len(doublelist))
	}
	if assert.ObjectsAreEqualValues(doublelist, []float64{0.7, 1.42}) {
		t.Fatalf("Unexpected component data 'doublelist' value: %s", result["doublelist"])
	}
	if result["emptykey"] != nil {
		t.Fatalf("Unexpected component data 'emptykey' value: %s", result["emptykey"])
	}
	if result["nestedobject"] == nil {
		t.Fatalf("Unexpected component data 'nestedobject' value: %s", result["nestedobject"])
	}
	var nestedObj = result["nestedobject"].(map[string]interface{})
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
	// keys must be a string
	if !strings.Contains(out, "keys must be") {
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
