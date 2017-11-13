package runner

import (
	"flag"
	"fmt"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/py"
	python "github.com/sbinet/go-python"
)

// Testing configuration is set at run-time by various flags - see the README for details
// Note: testingEfficiency defaults to false because this isn't a unit test that should automatically be included
var testingEfficiency = flag.Bool("efficiency", false, "run the num workers efficiency test")
var testingMemory = flag.Bool("memory", false, "run the num workers memory test")
var granularity = flag.Int("granularity", 100, "how many checks to run between memory measurements")
var static = flag.Bool("static", false, "use default num workers")
var lazyWait = flag.Bool("lazy", false, "use lazy wait")
var pythonCheck = flag.Bool("python", false, "run a python check")
var waitLength = flag.Int("wait", 100, "how many ms to each test check will take to complete")

// NumWorkersCheck implements the 'Check' interface via the TestCheck struct defined in runner_test.go
type NumWorkersCheck struct {
	TestCheck
	name string
}

type stickyLock struct {
	gstate python.PyGILState
	locked uint32 // Flag set to 1 if the lock is locked, 0 otherwise
}

func (nc *NumWorkersCheck) String() string { return nc.name }
func (nc *NumWorkersCheck) ID() check.ID   { return check.ID(nc.String()) }
func (nc *NumWorkersCheck) Run() error {
	if *pythonCheck {
		e := runPythonCheck() // BUG: this panics occasionally
		nc.hasRun = true
		return e
	}

	if *lazyWait {
		time.Sleep(time.Millisecond * time.Duration(*waitLength))
	} else {
		start := time.Now()
		now := time.Now()
		for {
			if now.Sub(start) > time.Millisecond*time.Duration(*waitLength) {
				break
			}
			now = time.Now()
		}
	}

	nc.hasRun = true
	return nil
}

// Efficiency test for the number of check workers running
func TestNumWorkersEfficiency(t *testing.T) {
	flag.Parse()
	if !(*testingEfficiency) {
		return
	}

	// Display the configuration details
	t.Logf("Testing check workers efficiency. Configuration:")
	t.Logf("\tCPU cores: %v", runtime.NumCPU())
	if *pythonCheck {
		t.Logf("\tCheck type: python check")
	} else {
		t.Logf("\tCheck type: golang check")
	}
	t.Logf("\tTime for check to complete: %vms", *waitLength)
	if *lazyWait {
		t.Logf("\tWait method: lazy wait")
	} else {
		t.Logf("\tWait method: busy wait")
	}

	if *pythonCheck {
		// Initialize the python interpreter
		state := py.Initialize(".", "../dist")

		defer python.PyEval_RestoreThread(state)
	}

	// Run the time tests
	t.Log("Starting time test")
	checksToRun := [9]int{5, 15, 25, 35, 45, 55, 65, 75, 100}

	for _, n := range checksToRun {
		ti := timeToComplete(n)

		t.Logf("Time for %v checks to complete: %vs", n, ti.Seconds())
	}

	// Run the memory tests if the flag was passed
	if *testingMemory {
		testMemory(t)
	}
}

// Measures the time for n checks to complete
func timeToComplete(n int) time.Duration {
	r := NewRunner()

	// Reset the stats
	curr, _ := strconv.Atoi(runnerStats.Get("Workers").String())
	runnerStats.Add("Workers", int64(curr*-1+defaultNumWorkers))

	start := time.Now()

	// Initialize the correct number of checks in the channel
	for i := 1; i <= n; i++ {
		c := NumWorkersCheck{name: "Check" + strconv.Itoa(i)}
		if !(*static) {
			r.UpdateNumWorkers(int64(i))
		}
		r.pending <- &c
	}

	// For the purpose of this test, we make each check run 3 times - the first time simulates
	// a new check being scheduled (UpdateNumWorkers gets called), the next 2 times simulate
	// an already scheduled check being run
	// This is done in order to offset any delay that calling UpdateNumWorkers might cause
	for i := 1; i <= n*2; i++ {
		c := NumWorkersCheck{name: "Check" + strconv.Itoa(i)}
		r.pending <- &c
	}
	close(r.pending)

	// Wait for all the checks to finish
	TestWg.Wait()

	return time.Now().Sub(start)
}

// Memory test for the number of check workers running
func testMemory(t *testing.T) {
	r := NewRunner()
	curr, _ := strconv.Atoi(runnerStats.Get("Workers").String())
	runnerStats.Add("Workers", int64(curr*-1))
	m := &runtime.MemStats{}

	t.Log("Starting memory test")
	runtime.ReadMemStats(m)

	t.Logf("Initially:")
	t.Logf("\tAlloc = %v\tSys = %v\tHeapAlloc = %v\tHeapSys = %v\tHeapObj = %v\t",
		m.Alloc/1024, m.Sys/1024, m.HeapAlloc, m.HeapSys, m.HeapObjects)

	for i := 1; i <= (*granularity)*5; i++ {
		c := NumWorkersCheck{name: "Check" + strconv.Itoa(i)}
		if !(*static) {
			r.UpdateNumWorkers(int64(i))
		}
		r.pending <- &c

		if i%(*granularity) == 0 {
			runtime.ReadMemStats(m)

			t.Logf("After %d checks:", i)
			t.Logf("\tAlloc = %v\tSys = %v\tHeapAlloc = %v\tHeapSys = %v\tHeapObj = %v\t",
				m.Alloc/1024, m.Sys/1024, m.HeapAlloc, m.HeapSys, m.HeapObjects)
		}
	}
}

func runPythonCheck() error {
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
		return fmt.Errorf("Unable to import runner_test")
	}

	// Import the TestCheck class
	checkClass := module.GetAttrString("TestCheck")
	if checkClass == nil {
		python.PyErr_Print()
		return fmt.Errorf("Unable to load TestCheck class")
	}

	// Set the check configuration
	config := "\ntest_instance:\n  lazy_wait: " + strconv.FormatBool(*lazyWait) + "\n  wait_length: " + strconv.Itoa(*waitLength)

	// Unlock the sticky lock
	atomic.StoreUint32(&gstate.locked, 0)
	python.PyGILState_Release(gstate.gstate)
	runtime.UnlockOSThread()

	// Acquire a PythonCheck instance
	check := py.NewPythonCheck("runner_test", checkClass)              // acquires its own stickyLock
	e := check.Configure([]byte(config), []byte("foo_init: bar_init")) // acquires its own stickyLock
	if check == nil || e != nil {
		return fmt.Errorf("Unable to acquire check instance")
	}

	// Run the check
	e = check.RunSimple() // acquires its own stickyLock
	return e
}
