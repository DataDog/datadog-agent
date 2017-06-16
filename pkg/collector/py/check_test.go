// NOTICE: See TestMain function in `utils_test.go` for Python initialization
package py

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/sbinet/go-python"
	"github.com/stretchr/testify/assert"
)

var (
	// package scope variables used to prevent compiler
	// optimisations in some benchmarks
	result error
)

func getClass(moduleName, className string) (checkClass *python.PyObject) {
	// Lock the GIL while operating with go-python
	gstate := newStickyLock()
	defer gstate.unlock()

	module := python.PyImport_ImportModule(moduleName)
	if module == nil {
		python.PyErr_Print()
		panic("Unable to import " + moduleName)
	}

	checkClass = module.GetAttrString(className)
	if checkClass == nil {
		python.PyErr_Print()
		panic("Unable to load " + className + " class")
	}

	return checkClass
}

func getCheckInstance(moduleName, className string) (*PythonCheck, error) {
	checkClass := getClass(moduleName, className)
	check := NewPythonCheck(moduleName, checkClass)
	err := check.Configure([]byte("foo: bar"), []byte("foo: bar"))
	return check, err
}

func TestNewPythonCheck(t *testing.T) {
	// Lock the GIL and release it at the end of the run
	gstate := newStickyLock()
	defer gstate.unlock()

	tuple := python.PyTuple_New(0)
	res := NewPythonCheck("FooBar", tuple)

	assert.Equal(t, tuple, res.Class)
	assert.Equal(t, "FooBar", res.ModuleName)
}

func TestRun(t *testing.T) {
	check, _ := getCheckInstance("testcheck", "TestCheck")
	err := check.Run()
	assert.Nil(t, err)
}

func TestStr(t *testing.T) {
	check, _ := getCheckInstance("testcheck", "TestCheck")
	name := "testcheck"
	if check.String() != name {
		t.Fatalf("Expected %s, found: %v", name, check)
	}
}

func TestInterval(t *testing.T) {
	c, _ := getCheckInstance("testcheck", "TestCheck")
	assert.Equal(t, check.DefaultCheckInterval, c.Interval())
	c.Configure([]byte("min_collection_interval: 1"), []byte("foo: bar"))
	assert.Equal(t, time.Duration(1)*time.Second, c.Interval())
}

func TestInitKwargsCheck(t *testing.T) {
	_, err := getCheckInstance("kwargs_init_signature", "TestCheck")
	assert.Nil(t, err)
}

func TestInitOldSignatureCheck(t *testing.T) {
	_, err := getCheckInstance("old_init_signature", "TestCheck")
	assert.Nil(t, err)
}

func TestInitOldAltSignatureCheck(t *testing.T) {
	_, err := getCheckInstance("old_alt_init_signature", "TestCheck")
	assert.Nil(t, err)
}

func TestInitNewSignatureCheck(t *testing.T) {
	_, err := getCheckInstance("new_init_signature", "TestCheck")
	assert.Nil(t, err)
}

func TestInitException(t *testing.T) {
	_, err := getCheckInstance("init_exception", "TestCheck")
	assert.EqualError(t, err, "could not invoke python check constructor: ['Traceback (most recent call last):\\n', '  File \"tests/init_exception.py\", line 6, in __init__\\n    raise RuntimeError(\"unexpected error\")\\n', 'RuntimeError: unexpected error\\n']")
}

func TestInitNoTracebackException(t *testing.T) {
	_, err := getCheckInstance("init_no_traceback_exception", "TestCheck")
	assert.EqualError(t, err, "could not invoke python check constructor: __init__() takes exactly 8 arguments (5 given)")
}

// BenchmarkRun executes a single check: benchmark results
// give an idea of the overhead of a CPython function call from go,
// that's why we don't care about Run's result.
func BenchmarkRun(b *testing.B) {
	var e error
	check, err := getCheckInstance("testcheck", "TestCheck")
	if err != nil {
		panic(err)
	}

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
// from different goroutines on different check instances
func BenchmarkConcurrentRun(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		class := getClass("testcheck", "TestCheck")
		for pb.Next() {
			check := NewPythonCheck("testcheck", class)
			err := check.Configure([]byte("foo: bar"), []byte("foo: bar"))
			if err != nil {
				panic(err)
			}
			check.Run()
		}
	})
}
