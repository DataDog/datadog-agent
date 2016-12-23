// NOTICE: See TestMain function in `utils_test.go` for Python initialization
package py

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/sbinet/go-python"
	"github.com/stretchr/testify/assert"
)

var (
	// package scope variables used to prevent compiler
	// optimisations in some benchmarks
	result error
)

func getCheckInstance(initAggregator bool) *PythonCheck {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	if initAggregator {
		aggregator.GetAggregator()
	}

	module := python.PyImport_ImportModule("testcheck")
	if module == nil {
		python.PyErr_Print()
		panic("Unable to import testcheck module")
	}

	checkClass := module.GetAttrString("TestCheck")
	if checkClass == nil {
		python.PyErr_Print()
		panic("Unable to load TestCheck class")
	}

	check := NewPythonCheck("testcheck", checkClass)
	check.Configure([]byte("foo: bar"))
	return check
}

func TestNewPythonCheck(t *testing.T) {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	tuple := python.PyTuple_New(0)
	res := NewPythonCheck("FooBar", tuple)

	if res.Class != tuple {
		t.Fatalf("Expected %v, found: %v", tuple, res.Class)
	}

	if res.ModuleName != "FooBar" {
		t.Fatalf("Expected FooBar, found: %v", res.ModuleName)
	}
}

func TestRun(t *testing.T) {
	check := getCheckInstance(true)
	if err := check.Run(); err != nil {
		t.Fatalf("Expected error nil, found: %s", err)
	}
}

func TestStr(t *testing.T) {
	check := getCheckInstance(true)
	name := "testcheck"
	if check.String() != name {
		t.Fatalf("Expected %s, found: %v", name, check)
	}
}

func TestInterval(t *testing.T) {
	c := getCheckInstance(true)
	assert.Equal(t, check.DefaultCheckInterval, c.Interval())
	c.Configure([]byte("min_collection_interval: 1"))
	assert.Equal(t, time.Duration(1), c.Interval())
}

// BenchmarkRun executes a single check, benchmark results
// give an idea of the overhead of a CPython function call from go
func BenchmarkRun(b *testing.B) {
	var e error
	check := getCheckInstance(false)
	for n := 0; n < b.N; n++ {
		// assign the return value to prevent compiler
		// optimisations
		e = check.Run()
	}
	// assign the error to a global var to prevent compiler
	// optimisations
	result = e
}

// BenchmarkRun simulates a Runner invoking `check.Run`
// from different goroutines
func BenchmarkConcurrentRun(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			check := getCheckInstance(false)
			check.Run()
		}
	})
}
