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

func getCheckInstance(initAggregator bool, moduleName string, className string) (*PythonCheck, error) {
	// Lock the GIL and release it at the end of the run
	_gstate := python.PyGILState_Ensure()
	defer func() {
		python.PyGILState_Release(_gstate)
	}()

	if initAggregator {
		aggregator.InitAggregator(nil)
	}

	module := python.PyImport_ImportModule(moduleName)
	if module == nil {
		python.PyErr_Print()
		panic("Unable to import testcheck module")
	}

	checkClass := module.GetAttrString(className)
	if checkClass == nil {
		python.PyErr_Print()
		panic("Unable to load " + className + " class")
	}

	check := NewPythonCheck(moduleName, checkClass)
	err := check.Configure([]byte("foo: bar"), []byte("foo: bar"))
	return check, err
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
	check, _ := getCheckInstance(true, "testcheck", "TestCheck")
	if err := check.Run(); err != nil {
		t.Fatalf("Expected error nil, found: %s", err)
	}
}

func TestStr(t *testing.T) {
	check, _ := getCheckInstance(true, "testcheck", "TestCheck")
	name := "testcheck"
	if check.String() != name {
		t.Fatalf("Expected %s, found: %v", name, check)
	}
}

func TestInterval(t *testing.T) {
	c, _ := getCheckInstance(true, "testcheck", "TestCheck")
	assert.Equal(t, check.DefaultCheckInterval, c.Interval())
	c.Configure([]byte("min_collection_interval: 1"), []byte("foo: bar"))
	assert.Equal(t, time.Duration(1)*time.Second, c.Interval())
}

func TestInitKwargsCheck(t *testing.T) {
	_, err := getCheckInstance(true, "kwargs_init_signature", "TestCheck")
	assert.Nil(t, err)
}

func TestInitOldSignatureCheck(t *testing.T) {
	_, err := getCheckInstance(true, "old_init_signature", "TestCheck")
	assert.Nil(t, err)
}

func TestInitNewSignatureCheck(t *testing.T) {
	_, err := getCheckInstance(true, "new_init_signature", "TestCheck")
	assert.Nil(t, err)
}

// BenchmarkRun executes a single check, benchmark results
// give an idea of the overhead of a CPython function call from go
func BenchmarkRun(b *testing.B) {
	var e error
	check, _ := getCheckInstance(false, "testcheck", "TestCheck")
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
			check, _ := getCheckInstance(false, "testcheck", "TestCheck")
			check.Run()
		}
	})
}
