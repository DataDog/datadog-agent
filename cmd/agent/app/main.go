package ddagentmain

import (
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/core"
	"github.com/DataDog/datadog-agent/pkg/collector/check/py"
	"github.com/DataDog/datadog-agent/pkg/collector/loader"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/kardianos/osext"
	"github.com/op/go-logging"
	"github.com/sbinet/go-python"

	// register core checks
	_ "github.com/DataDog/datadog-agent/pkg/collector/check/core/system"
)

const agentVersion = "6.0.0"

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

// build a list of providers for checks' configurations, the sequence defines
// the precedence.
func getConfigProviders() (providers []loader.ConfigProvider) {
	confdPath := filepath.Join(distPath, "conf.d")
	configPaths := []string{confdPath}

	// File Provider
	providers = append(providers, loader.NewFileConfigProvider(configPaths))

	return providers
}

// build a list of check loaders, the sequence defines the precedence.
func getCheckLoaders() []loader.CheckLoader {
	return []loader.CheckLoader{
		py.NewPythonCheckLoader(),
		core.NewGoCheckLoader(),
	}
}

// define configuration data for the Agent using different providers
func getConfiguration() *config.Config {
	cfg := config.NewConfig()

	// for now, we only load configuration from file
	fileProvider := config.NewFileProvider(configPath)
	fileProvider.Configure(cfg)

	return cfg
}

// Start the main check loop
func Start() {

	log.Infof("Starting Datadog Agent v%v", agentVersion)

	// Global Agent configuration
	// config := getConfiguration()

	// Create a channel to enqueue the checks
	pending := make(chan check.Check, 10)

	// Initialize the CPython interpreter
	state := py.Initialize(distPath, filepath.Join(distPath, "checks"))

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
