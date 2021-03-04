package testtopology

import (
	"fmt"
	"os"
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

func TestSubmitComponent(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`topology.submit_component(None, "checkid", {"type": "instance.type", "url": "instance.url"}, "myid", "mytype", { "key": "value", "intlist": [1], "emptykey": None, "nestedobject": { "nestedkey": "nestedValue" }})`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	if checkID != "checkid" {
		t.Fatalf("Unexpected check id value: %s", checkID)
	}
	//test instance
	//test comp id
	//test comp type
	//test comp data ?? should be a map[string]interface{}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

//test passing something that cannot be yaml unmarshalled ?

func TestSubmitRelation(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`topology.submit_relation(None, "checkid", {"type": "instance.type", "url": "instance.url"}, "source", "target", "mytype", { "key": "value", "intlist": [1], "emptykey": None, "nestedobject": { "nestedkey": "nestedValue" }})`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	if checkID != "checkid" {
		t.Fatalf("Unexpected check id value: %s", checkID)
	}
	//test instance
	//test comp id
	//test comp type
	//test comp data ?? should be a map[string]interface{}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestStartSnapshot(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`topology.submit_start_snapshot(None, "checkid", {"type": "instance.type", "url": "instance.url"})`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	if checkID != "checkid" {
		t.Fatalf("Unexpected check id value: %s", checkID)
	}
	//test instance


	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestStopSnapshot(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`topology.submit_stop_snapshot(None, "checkid", {"type": "instance.type", "url": "instance.url"})`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	if checkID != "checkid" {
		t.Fatalf("Unexpected check id value: %s", checkID)
	}
	//test instance

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}
