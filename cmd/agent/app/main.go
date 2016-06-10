package ddagentmain

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/aggregator"
	"github.com/DataDog/datadog-agent/pkg/checks"
	"github.com/DataDog/datadog-agent/pkg/checks/system"
	"github.com/DataDog/datadog-agent/pkg/py"
	"github.com/kardianos/osext"
	"github.com/op/go-logging"
	"github.com/sbinet/go-python"
)

const AGENT_VERSION = "6.0.0"

var log = logging.MustGetLogger("datadog-agent")

// schedule all the available checks for running
func enqueueChecks(pending chan checks.Check, checks []checks.Check) {
	for i := 0; i < len(checks); i++ {
		pending <- checks[i]
	}
}

// for testing purposes only: collect and log check results
type metric struct {
	Name  string
	Value float64
	Tags  []string
}

type metrics map[string][]metric

// Start the main check loop
func Start() {

	log.Infof("Starting Datadog Agent v%v", AGENT_VERSION)

	pending := make(chan checks.Check, 10)

	err := python.Initialize()
	if err != nil {
		panic(err.Error())
	}
	// Set the PYTHONPATH
	here, _ := osext.ExecutableFolder()
	distPath := fmt.Sprintf("%s/dist", here)
	confdPath := fmt.Sprintf("%s/conf.d", distPath)
	path := python.PySys_GetObject("path")
	python.PyList_Append(path, python.PyString_FromString(distPath))

	// `python.Initialize` acquires the GIL but we don't need it, let's release it
	state := python.PyEval_SaveThread()

	// for now, only Python needs it, build and pass it on the fly
	py.InitApi(aggregator.NewUnbufferedAggregator())

	// Get a single Runner instance, i.e. we process checks sequentially
	go checks.Runner(pending)

	// Get a list of Python checks we want to run
	checksNames := []string{"checks.go_expvar"}
	// Search for and import all the desired Python checks
	checks := py.CollectChecks(checksNames, confdPath)

	// Run memory check, this is a native check, not Python
	mc := system.MemoryCheck{}
	checks = append(checks, &mc)

	// Start the scheduler
	ticker := time.NewTicker(time.Millisecond * 5000)
	for t := range ticker.C {
		log.Infof("Tick at %v", t)
		// Schedule the checks
		go enqueueChecks(pending, checks)
	}

	python.PyEval_RestoreThread(state)
}
