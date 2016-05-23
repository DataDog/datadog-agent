package ddagentmain

import (
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/checks"
	"github.com/DataDog/datadog-agent/checks/system"
	"github.com/DataDog/datadog-agent/py"
	"github.com/DataDog/datadog-go/statsd"
	"github.com/op/go-logging"
	"github.com/sbinet/go-python"
)

const AGENT_VERSION = "6.0.0"
const confdPath = "py/conf.d"

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

// process results. Temporary solution: send results to DogStatsD
func collectResults(pending <-chan checks.CheckResult, c *statsd.Client) {
	for res := range pending {
		m := metrics{}
		if err := json.Unmarshal([]byte(res.Result), &m); err != nil {
			log.Errorf("Error parsing results: %s\n%v", res.Result, err)
		}
		// gauges
		for _, g := range m["gauge"] {
			log.Infof("gauge posted %s", g.Name)
			if err := c.Gauge(g.Name, g.Value, g.Tags, 1); err != nil {
				log.Errorf("Error posting gauge %s: %v", g.Name, err)
			}
		}
		// histogram
		for _, g := range m["histogram"] {
			log.Infof("histogram posted %s", g.Name)
			if err := c.Histogram(g.Name, g.Value, g.Tags, 1); err != nil {
				log.Errorf("Error histogram %s: %v", g.Name, err)
			}
		}
	}
}

// Start the main check loop
func Start() {

	log.Infof("Starting Datadog Agent v%v", AGENT_VERSION)

	pending := make(chan checks.Check, 100)
	results := make(chan checks.CheckResult, 100)

	err := python.Initialize()
	if err != nil {
		panic(err.Error())
	}
	// Set the PYTHONPATH
	path := python.PySys_GetObject("path")
	python.PyList_Append(path, python.PyString_FromString("py"))

	// `python.Initialize` acquires the GIL but we don't need it, let's release it
	state := python.PyEval_SaveThread()

	// DogStatsD client, temporary solution to post metrics
	c, err := statsd.New("127.0.0.1:8125")
	if err != nil {
		panic(err)
	}
	c.Namespace = "agent6."

	// Get a single Runner instance, i.e. we process checks sequentially
	go checks.Runner(pending, results)
	// Get a set of collectors able to process all the results w/o causing check starvation
	for i := 0; i < 2; i++ {
		go collectResults(results, c)
	}

	// Get a list of Python checks we want to run
	checksNames := []string{"checks.directory", "checks.go_expvar", "checks.process"}
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
