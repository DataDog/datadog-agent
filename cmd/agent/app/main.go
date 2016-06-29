package ddagentmain

import (
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/checks"
	"github.com/DataDog/datadog-agent/pkg/checks/system"
	"github.com/DataDog/datadog-agent/pkg/loader"
	"github.com/DataDog/datadog-agent/pkg/py"
	"github.com/kardianos/osext"
	"github.com/op/go-logging"
	"github.com/sbinet/go-python"
)

const AGENT_VERSION = "6.0.0"

var here, _ = osext.ExecutableFolder()
var distPath = filepath.Join(here, "dist")
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

func getConfigProviders() (providers []loader.ConfigProvider) {
	confdPath := filepath.Join(distPath, "conf.d")
	configPaths := []string{confdPath}

	// File Provider
	providers = append(providers, loader.NewFileConfigProvider(configPaths))

	return providers
}

// Start the main check loop
func Start() {

	log.Infof("Starting Datadog Agent v%v", AGENT_VERSION)

	pending := make(chan checks.Check, 10)

	err := python.Initialize()
	if err != nil {
		panic(err.Error())
	}

	// Set the PYTHONPATH
	checksPath := filepath.Join(distPath, "checks")
	path := python.PySys_GetObject("path")
	python.PyList_Append(path, python.PyString_FromString(distPath))
	python.PyList_Append(path, python.PyString_FromString(checksPath))

	// `python.Initialize` acquires the GIL but we don't need it, let's release it
	state := python.PyEval_SaveThread()

	// Expose `CheckAggregator` methods to Python
	py.InitApi()

	// Get a single Runner instance, i.e. we process checks sequentially
	go checks.Runner(pending)

	// Get a list of config checks from the configured providers
	var configs []loader.CheckConfig
	for _, provider := range getConfigProviders() {
		c, _ := provider.Collect()
		configs = append(configs, c...)
	}

	// try to import corresponding checks using the PythonCheckLoader
	checks := []checks.Check{}
	checksLoader := py.NewPythonCheckLoader()
	for _, conf := range configs {
		res, err := checksLoader.Load(conf)
		if err == nil {
			checks = append(checks, res...)
		}
	}

	// Run memory check, this is a native check, not Python
	// TODO: see above, this should be done elsewhere, not manually here
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
