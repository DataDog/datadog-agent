// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython

// NOTICE: See TestMain function in `utils_test.go` for Python initialization
package py

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"

	python "github.com/sbinet/go-python"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
	mockConfig := config.NewMock()
	mockConfig.Set("foo_agent", "bar_agent")
	defer mockConfig.Set("foo_agent", nil)

	checkClass := getClass(moduleName, className)
	check := NewPythonCheck(moduleName, checkClass)
	err := check.Configure([]byte("foo_instance: bar_instance"), []byte("foo_init: bar_init"))
	return check, err
}

func TestNewPythonCheck(t *testing.T) {
	// Lock the GIL and release it at the end of the run
	gstate := newStickyLock()
	defer gstate.unlock()

	tuple := python.PyTuple_New(0)
	res := NewPythonCheck("FooBar", tuple)

	assert.Equal(t, tuple, res.class)
	assert.Equal(t, "FooBar", res.ModuleName)
}

func TestRun(t *testing.T) {
	check, _ := getCheckInstance("testcheck", "TestCheck")
	err := check.Run()
	assert.Nil(t, err)
}

func TestSubprocessRun(t *testing.T) {
	check, _ := getCheckInstance("testsubprocess", "TestSubprocessCheck")
	err := check.Run()
	assert.Nil(t, err)
}

func TestSubprocessRunConcurrent(t *testing.T) {
	instances := make([]*PythonCheck, 30)
	for i := range instances {
		check, _ := getCheckInstance("testsubprocess", "TestSubprocessCheck")
		instances[i] = check
	}

	for _, check := range instances {
		go func(c *PythonCheck) {
			err := c.Run()
			assert.Nil(t, err)
		}(check)
	}
}

func TestWarning(t *testing.T) {
	check, _ := getCheckInstance("testwarnings", "TestCheck")
	err := check.Run()
	assert.Nil(t, err)

	warnings := check.GetWarnings()
	require.Len(t, warnings, 1)
	assert.Equal(t, "The cake is a lie", warnings[0].Error())
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

	assert.Regexp(t, "could not invoke python check constructor: Traceback \\(most recent call last\\):\n  File \"[\\S]+(\\/|\\\\)init_exception\\.py\", line 11, in __init__\n    raise RuntimeError\\(\"unexpected error\"\\)\nRuntimeError: unexpected error", err.Error())
}

func TestInitNoTracebackException(t *testing.T) {
	_, err := getCheckInstance("init_no_traceback_exception", "TestCheck")
	assert.EqualError(t, err, "could not invoke python check constructor: __init__() takes exactly 8 arguments (5 given)")
}

// TestAggregatorLink checks to see if a simple check that sends metrics to the aggregator has no errors
func TestAggregatorLink(t *testing.T) {
	check, _ := getCheckInstance("testaggregator", "TestAggregatorCheck")

	mockSender := mocksender.NewMockSender(check.ID())

	mockSender.On("ServiceCheck",
		"testservicecheck", mock.AnythingOfType("metrics.ServiceCheckStatus"), "",
		[]string(nil), mock.AnythingOfType("string")).Return().Times(1)
	mockSender.On("ServiceCheck",
		"testservicecheckwithhostname", mock.AnythingOfType("metrics.ServiceCheckStatus"), "testhostname",
		[]string{"foo", "bar"}, "a message").Return().Times(1)
	mockSender.On("ServiceCheck",
		"testservicecheckwithnonemessage", mock.AnythingOfType("metrics.ServiceCheckStatus"), "",
		[]string(nil), "").Return().Times(1)
	mockSender.On("Gauge", "testmetric", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	mockSender.On("Gauge", "testmetricstringvalue", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(1)
	mockSender.On("Counter", "test.increment", 1., "", []string{"foo", "bar"}).Return().Times(1)
	mockSender.On("Counter", "test.decrement", -1., "", []string{"foo", "bar", "baz"}).Return().Times(1)
	mockSender.On("Event", mock.AnythingOfType("metrics.Event")).Return().Times(1)
	mockSender.On("Commit").Return().Times(1)

	err := check.Run()
	assert.Nil(t, err)
}

// TestAggregatorLinkTwoRuns checks to ensure that it is consistently grabbing the correct aggregator
// Essentially it ensures that checkID is being set correctly
func TestAggregatorLinkTwoRuns(t *testing.T) {
	check, _ := getCheckInstance("testaggregator", "TestAggregatorCheck")

	mockSender := mocksender.NewMockSender(check.ID())

	mockSender.On("ServiceCheck",
		"testservicecheck", mock.AnythingOfType("metrics.ServiceCheckStatus"), "",
		[]string(nil), mock.AnythingOfType("string")).Return().Times(2)
	mockSender.On("ServiceCheck",
		"testservicecheckwithhostname", mock.AnythingOfType("metrics.ServiceCheckStatus"), "testhostname",
		[]string{"foo", "bar"}, "a message").Return().Times(2)
	mockSender.On("ServiceCheck",
		"testservicecheckwithnonemessage", mock.AnythingOfType("metrics.ServiceCheckStatus"), "",
		[]string(nil), "").Return().Times(2)
	mockSender.On("Gauge", "testmetric", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(2)
	mockSender.On("Gauge", "testmetricstringvalue", mock.AnythingOfType("float64"), "", []string(nil)).Return().Times(2)
	mockSender.On("Counter", "test.increment", 1., "", []string{"foo", "bar"}).Return().Times(2)
	mockSender.On("Counter", "test.decrement", -1., "", []string{"foo", "bar", "baz"}).Return().Times(2)
	mockSender.On("Event", mock.AnythingOfType("metrics.Event")).Return().Times(2)
	mockSender.On("Commit").Return().Times(2)

	err := check.Run()
	assert.Nil(t, err)
	err = check.Run()
	assert.Nil(t, err)
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
