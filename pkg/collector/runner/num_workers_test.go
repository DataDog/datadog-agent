package runner

import (
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/py"
	python "github.com/sbinet/go-python"
)

/****************** Testing configuration ****************************/

var testingEfficiency = false // Run this test (should be false normally)
var static = true             // Run with default num workers (vs dynamic)
var prettyOutput = true       // Labelled test results
var checkType = lazyWait      // Which type of check to run (busyWait, lazyWait, or pythonCheck)
var numIntervals = 2          // How many times to repeat each check (for the time tests)
var memoryTest = false        // After the time tests, run the memory test

/*********************************************************************/

type CheckType int

const (
	busyWait    CheckType = iota
	lazyWait    CheckType = iota
	pythonCheck CheckType = iota
)

type stickyLock struct {
	gstate python.PyGILState
	locked uint32 // Flag set to 1 if the lock is locked, 0 otherwise
}

// NumWorkersCheck implements the 'Check' interface via the TestCheck struct defined in runner_test.go
type NumWorkersCheck struct {
	TestCheck
	name string
}

func (nc *NumWorkersCheck) String() string { return nc.name }
func (nc *NumWorkersCheck) ID() check.ID   { return check.ID(nc.String()) }

func (nc *NumWorkersCheck) Run() error {
	switch checkType {
	case busyWait:
		start := time.Now()
		now := time.Now()
		for {
			if now.Sub(start) > time.Millisecond*100 {
				break
			}
			now = time.Now()
		}

	case lazyWait:
		time.Sleep(time.Millisecond * 100)

	case pythonCheck:
		// BUG: testing the python checks crashes every so often
		runPythonCheck()
	}

	nc.hasRun = true
	return nil
}

// Efficiency test for the number of check workers running
// Note: use the -v flag when testing to see the output
func TestUpdateNumWorkers(t *testing.T) {
	if !testingEfficiency {
		return
	}

	if checkType == pythonCheck {
		// Initialize the python interpreter & the aggregator
		state := py.Initialize(".", "../dist")
		aggregator.InitAggregator(nil, "")

		defer python.PyEval_RestoreThread(state)
	}

	// Run the time tests
	interval := false
	t.Log("********* Starting time efficiency test *********")
	for i := 0; i < 2; i++ {
		if interval {
			t.Logf("Running each check %v times:", numIntervals)
		} else {
			t.Log("Running each check once:")
		}

		checksToRun := [10]int{5, 15, 25, 35, 45, 55, 65, 75, 85, 100}

		for _, n := range checksToRun {
			ti := timeToComplete(n, interval)

			if prettyOutput {
				t.Logf("Time to run %v checks: %v", n, ti.Seconds())
			} else {
				t.Logf("%v", ti.Seconds())
			}
		}

		if numIntervals == 0 {
			break
		}

		interval = true
	}

	if !memoryTest {
		return
	}
	// Run the memory test
	r := NewRunner()
	curr, _ := strconv.Atoi(runnerStats.Get("Workers").String())
	runnerStats.Add("Workers", int64(curr*-1))
	m := &runtime.MemStats{}

	t.Log("********* Starting memory test *********")
	runtime.ReadMemStats(m)

	if prettyOutput {
		t.Logf("At start:")
		t.Logf("\tAlloc = %v\tSys = %v\tHeapAlloc = %v\tHeapSys = %v\tHeapObj = %v\t",
			m.Alloc/1024, m.Sys/1024, m.HeapAlloc, m.HeapSys, m.HeapObjects)
	} else {
		t.Logf("%v\t%v\t%v\t%v\t%v\t", m.Alloc/1024, m.Sys/1024, m.HeapAlloc, m.HeapSys, m.HeapObjects)
	}

	for i := 1; i < 500; i++ {
		c := NumWorkersCheck{name: "Check" + strconv.Itoa(i)}
		if !static {
			r.UpdateNumWorkers(int64(i))
		}
		r.pending <- &c

		if i%100 == 0 {
			runtime.ReadMemStats(m)

			if prettyOutput {
				t.Logf("After %d checks:", i)
				t.Logf("\tAlloc = %v\tSys = %v\tHeapAlloc = %v\tHeapSys = %v\tHeapObj = %v\t",
					m.Alloc/1024, m.Sys/1024, m.HeapAlloc, m.HeapSys, m.HeapObjects)
			} else {
				t.Logf("%v\t%v\t%v\t%v\t%v\t", m.Alloc/1024, m.Sys/1024, m.HeapAlloc, m.HeapSys, m.HeapObjects)
			}
		}
	}
}

func timeToComplete(numChecks int, runMultiple bool) time.Duration {
	r := NewRunner()

	// Reset the stats
	curr, _ := strconv.Atoi(runnerStats.Get("Workers").String())
	runnerStats.Add("Workers", int64(curr*-1)+defaultNumWorkers)

	start := time.Now()

	// Initialize the correct number of checks in the channel
	for i := 1; i < numChecks; i++ {
		c := NumWorkersCheck{name: "Check" + strconv.Itoa(i)}
		if !static {
			r.UpdateNumWorkers(int64(i))
		}
		r.pending <- &c
	}

	if runMultiple {
		// To imitate a check running at an interval (UpdateNumWorkers doesn't run again)
		for j := 0; j < numIntervals; j++ {
			for i := 1; i < numChecks; i++ {
				c := NumWorkersCheck{name: "Check" + strconv.Itoa(i)}
				r.pending <- &c
			}
		}
	}
	close(r.pending)

	// Wait for all the checks to finish
	Test_wg.Wait()

	return time.Now().Sub(start)
}

func runPythonCheck() {
	// Lock the Global Interpreter Lock while operating with go-python
	runtime.LockOSThread()
	gstate := &stickyLock{
		gstate: python.PyGILState_Ensure(),
		locked: 1,
	}

	// Import the runner_test module
	module := python.PyImport_ImportModule("runner_test")
	if module == nil {
		python.PyErr_Print()
		panic("Unable to import runner_test")
	}

	// Import the TestCheck class
	checkClass := module.GetAttrString("TestCheck")
	if checkClass == nil {
		python.PyErr_Print()
		panic("Unable to load TestCheck class")
	}

	// Unlock the sticky lock
	atomic.StoreUint32(&gstate.locked, 0)
	python.PyGILState_Release(gstate.gstate)
	runtime.UnlockOSThread()

	// Acquire a PythonCheck instance
	check := py.NewPythonCheck("runner_test", checkClass)                                    // acquires its own sticky lock
	e := check.Configure([]byte("foo_instance: bar_instance"), []byte("foo_init: bar_init")) // acquires its own sticky lock
	if check == nil || e != nil {
		panic("Unable to acquire check instance")
	}

	// Run the check
	e = check.Run() // acquires its own stickyLock
	if e != nil {
		panic("Unable to run check: " + e.Error())
	}
}
