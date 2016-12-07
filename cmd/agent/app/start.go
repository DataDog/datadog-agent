package app

import (
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/core"
	"github.com/DataDog/datadog-agent/pkg/collector/check/py"
	"github.com/DataDog/datadog-agent/pkg/collector/loader"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
	"github.com/kardianos/osext"
	python "github.com/sbinet/go-python"
	"github.com/spf13/cobra"
)

var (
	here, _  = osext.ExecutableFolder()
	distPath = filepath.Join(here, "dist")
	startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the Agent",
		Long:  ``,
		Run:   start,
	}
)

func init() {
	AgentCmd.AddCommand(startCmd)
}

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

// Start the main check loop
func start(cmd *cobra.Command, args []string) {
	defer log.Flush()

	log.Infof("Starting Datadog Agent v%v", agentVersion)

	// Global Agent configuration
	for _, path := range configPaths {
		config.Datadog.AddConfigPath(path)
	}
	err := config.Datadog.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("unable to load Datadog config file: %s", err))
	}

	// Initialize the CPython interpreter
	state := py.Initialize(distPath, filepath.Join(distPath, "checks"))

	// Get a list of config checks from the configured providers
	var configs []check.Config
	for _, provider := range getConfigProviders() {
		c, _ := provider.Collect()
		configs = append(configs, c...)
	}

	// Get a Runner instance
	runner := check.NewRunner()

	// Instance the scheduler
	scheduler := scheduler.NewScheduler()

	// Instance the Aggregator
	_ = aggregator.GetAggregator()

	// given a list of configurations, try to load corresponding checks using different loaders
	// TODO add check type to the conf file so that we avoid the inner for
	loaders := getCheckLoaders()
	for _, conf := range configs {
		for _, loader := range loaders {
			res, err := loader.Load(conf)
			if err == nil {
				for _, check := range res {
					scheduler.Enter(check)
				}
			}
		}
	}

	// Start the Runner using only one worker, i.e. we process checks sequentially
	runner.Run(1)

	// Run the scheduler
	scheduler.Run(runner.GetChan())

	// indefinitely block here for now, later we'll migrate to a more sophisticated
	// system to handle interrupts (reloads, restarts, service discovery events, etc...)
	var c chan bool
	<-c

	// this is not called for now, sorry CPython for leaving a mess on exit!
	python.PyEval_RestoreThread(state)
}
