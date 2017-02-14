package app

import (
	"syscall"

	"os"
	"os/exec"
	"os/signal"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/agent/api"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/py"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/version"
	log "github.com/cihub/seelog"
	python "github.com/sbinet/go-python"
	"github.com/spf13/cobra"
)

var (
	// flags variables
	runForeground bool
	pidfilePath   string
	confdPath     string

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

	// Global Agent configuration
	common.SetupConfig()

	// local flags
	startCmd.Flags().BoolVarP(&runForeground, "foreground", "f", false, "run in foreground")
	startCmd.Flags().StringVarP(&pidfilePath, "pidfile", "p", "", "path to the pidfile")
	startCmd.Flags().StringVarP(&confdPath, "confd", "c", "", "path to the confd folder")
	config.Datadog.BindPFlag("confd_path", startCmd.Flags().Lookup("confd"))
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

	log.Infof("Starting Datadog Agent v%v", version.AgentVersion)
	log.Infof("Hostname is: %s", common.GetHostname())

	// start the cmd HTTP server
	api.StartServer()

	// Initialize the CPython interpreter
	state := py.Initialize(common.DistPath, filepath.Join(common.DistPath, "checks"))

	// Get a list of config checks from the configured providers
	var configs []check.Config
	for _, provider := range common.GetConfigProviders(config.Datadog.GetString("confd_path")) {
		c, _ := provider.Collect()
		configs = append(configs, c...)
	}

	// Get a Runner instance
	common.AgentRunner = check.NewRunner(config.Datadog.GetInt("run_workers"))

	// Instance the scheduler
	common.AgentScheduler = scheduler.NewScheduler(common.AgentRunner.GetChan())

	// Instance the Aggregator
	_ = aggregator.GetAggregator()

	// given a list of configurations, try to load corresponding checks using different loaders
	// TODO add check type to the conf file so that we avoid the inner for
	loaders := common.GetCheckLoaders()
	for _, conf := range configs {
		for _, loader := range loaders {
			res, err := loader.Load(conf)
			if err == nil {
				for _, check := range res {
					common.AgentScheduler.Enter(check)
				}
			}
		}
	}

	// Run the scheduler
	common.AgentScheduler.Run()

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Block here until we receive the interrupt signal
	select {
	case <-common.Stopper:
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
	common.AgentRunner.Stop()
	common.AgentScheduler.Stop()
	python.PyEval_RestoreThread(state)
	api.StopServer()
	os.Remove(pidfilePath)
	log.Info("See ya!")
	log.Flush()
	os.Exit(0)
}
