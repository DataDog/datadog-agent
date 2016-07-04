package ddagentmain

import (
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/check"
	"github.com/DataDog/datadog-agent/pkg/checks"
	"github.com/DataDog/datadog-agent/pkg/loader"
	"github.com/DataDog/datadog-agent/pkg/py"
	"github.com/kardianos/osext"
	"github.com/op/go-logging"
	"github.com/sbinet/go-python"

	// register core checks
	_ "github.com/DataDog/datadog-agent/pkg/checks/system"
)

const AGENT_VERSION = "6.0.0"

var here, _ = osext.ExecutableFolder()
var distPath = filepath.Join(here, "dist")
var log = logging.MustGetLogger("datadog-agent")

// schedule all the available checks for running
func enqueueChecks(pending chan check.Check, checks []check.Check) {
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

func getCheckLoaders() []loader.CheckLoader {
	return []loader.CheckLoader{
		py.NewPythonCheckLoader(),
		checks.NewGoCheckLoader(),
	}
}

// Start the main check loop
func Start() {

	log.Infof("Starting Datadog Agent v%v", AGENT_VERSION)

	pending := make(chan check.Check, 10)

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
	go check.Runner(pending)

	// Get a list of config checks from the configured providers
	var configs []check.Config
	for _, provider := range getConfigProviders() {
		c, _ := provider.Collect()
		configs = append(configs, c...)
	}

	// given a list of configurations, try to load corresponding checks using different loaders
	loaders := getCheckLoaders()
	checks := []check.Check{}
	for _, conf := range configs {
		for _, loader := range loaders {
			res, err := loader.Load(conf)
			if err == nil {
				checks = append(checks, res...)
			}
		}
	}

	// Start the scheduler
	ticker := time.NewTicker(time.Millisecond * 5000)
	for t := range ticker.C {
		log.Infof("Tick at %v", t)
		// Schedule the checks
		go enqueueChecks(pending, checks)
	}

	python.PyEval_RestoreThread(state)
}
