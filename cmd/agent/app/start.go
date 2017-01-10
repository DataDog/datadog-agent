package app

import (
	"syscall"

	"os"
	"os/exec"
	"os/signal"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/agent/app/api"
	"github.com/DataDog/datadog-agent/cmd/agent/app/ipc"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/core"
	"github.com/DataDog/datadog-agent/pkg/collector/check/py"
	"github.com/DataDog/datadog-agent/pkg/collector/loader"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	log "github.com/cihub/seelog"
	python "github.com/sbinet/go-python"
	"github.com/spf13/cobra"
)

var (
	// flags variables
	runForeground bool
	pidfilePath   string

	startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the Agent",
		Long:  ``,
		Run:   start,
	}
)

func init() {
	// attach the command to the root
	AgentCmd.AddCommand(startCmd)

	// local flags
	startCmd.Flags().BoolVarP(&runForeground, "foreground", "f", false, "run in foreground")
	startCmd.Flags().StringVarP(&pidfilePath, "pidfile", "p", "", "path to the pidfile")
}

// build a list of providers for checks' configurations, the sequence defines
// the precedence.
func getConfigProviders() (providers []loader.ConfigProvider) {
	confSearchPaths := []string{}
	for _, path := range configPaths {
		confSearchPaths = append(confSearchPaths, filepath.Join(path, "conf.d"))
	}

	// File Provider
	providers = append(providers, loader.NewFileConfigProvider(confSearchPaths))

	return providers
}

// build a list of check loaders, the sequence defines the precedence.
func getCheckLoaders() []loader.CheckLoader {
	return []loader.CheckLoader{
		py.NewPythonCheckLoader(),
		core.NewGoCheckLoader(),
	}
}

// runBackground spawns a child so that the main process can exit.
// The forked process is started with the `-f`` option so that we don't
// get in a fork loop. If not already present, we add the `-p` flag
// to write the pidfile.
func runBackground() {
	args := os.Args
	args = append(args, "-f")
	if pidfilePath == "" {
		args = append(args, "-p", pidfile.Path())
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()
}

// Start the main loop
func start(cmd *cobra.Command, args []string) {
	if !runForeground {
		runBackground()
		return
	}

	if pidfilePath != "" {
		err := pidfile.WritePID(pidfilePath)
		if err != nil {
			panic(err)
		}
	}

	defer log.Flush()

	log.Infof("Starting Datadog Agent v%v", agentVersion)

	// Global Agent configuration
	setupConfig()

	// start the ipc server
	ipc.Listen()

	// start the cmd HTTP server
	api.StartServer()

	// start the forwarder
	_forwarder = forwarder.NewForwarder()
	_forwarder.Start()

	// Initialize the CPython interpreter
	state := py.Initialize(_distPath, filepath.Join(_distPath, "checks"))

	// Get a list of config checks from the configured providers
	var configs []check.Config
	for _, provider := range getConfigProviders() {
		c, _ := provider.Collect()
		configs = append(configs, c...)
	}

	// Get a Runner instance
	_runner = check.NewRunner()

	// Instance the scheduler
	_scheduler = scheduler.NewScheduler()

	// Instance the Aggregator
	_ = aggregator.GetAggregator(_forwarder)

	// given a list of configurations, try to load corresponding checks using different loaders
	// TODO add check type to the conf file so that we avoid the inner for
	loaders := getCheckLoaders()
	for _, conf := range configs {
		for _, loader := range loaders {
			res, err := loader.Load(conf)
			if err == nil {
				for _, check := range res {
					_scheduler.Enter(check)
				}
			}
		}
	}

	// Start the Runner using only one worker, i.e. we process checks sequentially
	_runner.Run(1)

	// Run the scheduler
	_scheduler.Run(_runner.GetChan())

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Block here until we receive the interrupt signal
	select {
	case <-ipc.ShouldStop:
		log.Info("Received stop command, shutting down...")
		goto teardown
	case sig := <-signalCh:
		log.Infof("Received signal '%s', shutting down...", sig)
		if sig == os.Interrupt || sig == syscall.SIGTERM {
			goto teardown
		}
	}

teardown:
	// gracefully shut down any component
	_runner.Stop()
	_scheduler.Stop()
	_forwarder.Stop()
	python.PyEval_RestoreThread(state)
	ipc.StopListen()
	os.Remove(pidfilePath)
	log.Info("See ya!")
	os.Exit(0)
}
