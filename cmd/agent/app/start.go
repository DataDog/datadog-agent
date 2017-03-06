package app

import (
	"path/filepath"
	"syscall"

	"os"
	"os/exec"
	"os/signal"

	"github.com/DataDog/datadog-agent/cmd/agent/api"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/metadata"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/version"
	log "github.com/cihub/seelog"
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

	hostname := common.GetHostname()

	log.Infof("Starting Datadog Agent v%v", version.AgentVersion)
	log.Infof("Hostname is: %s", hostname)

	// start the cmd HTTP server
	api.StartServer()

	// setup the forwarder
	// for now we handle only one key and one domain
	keysPerDomain := map[string][]string{
		config.Datadog.GetString("dd_url"): {
			config.Datadog.GetString("api_key"),
		},
	}
	fwd := forwarder.NewForwarder(keysPerDomain)
	fwd.Start()

	// setup the aggregator
	agg := aggregator.InitAggregator(fwd)

	// start dogstatsd
	var statsd *dogstatsd.Server
	if config.Datadog.GetBool("use_dogstatsd") {
		var err error
		statsd, err = dogstatsd.NewServer(agg.GetChannel())
		if err != nil {
			log.Errorf("Could not start dogstatsd: %s", err)
		}
	}

	// create the Collector instance and start all the components
	// NOTICE: this will also setup the Python environment
	common.Collector = collector.NewCollector(common.DistPath, filepath.Join(common.DistPath, "checks"))

	// setup the metadata collector, this needs a working Python env to function
	metaCollector := metadata.NewCollector(fwd, config.Datadog.GetString("api_key"), hostname)

	// Get a list of config checks from the configured providers
	var configs []check.Config
	for _, provider := range common.GetConfigProviders(config.Datadog.GetString("confd_path")) {
		c, _ := provider.Collect()
		configs = append(configs, c...)
	}

	// given a list of configurations, try to load corresponding checks using different loaders
	// TODO add check type to the conf file so that we avoid the inner for
	loaders := common.GetCheckLoaders()
	for _, conf := range configs {
		for _, loader := range loaders {
			res, err := loader.Load(conf)
			if err != nil {
				log.Warnf("Unable to load the check '%s' from the configuration: %s", conf.Name, err)
				continue
			}

			for _, check := range res {
				err := common.Collector.RunCheck(check)
				if err != nil {
					log.Warnf("Unable to run check %v: %s", check, err)
				}
			}
		}
	}

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Block here until we receive the interrupt signal
	select {
	case <-common.Stopper:
		log.Info("Received stop command, shutting down...")
	case sig := <-signalCh:
		log.Infof("Received signal '%s', shutting down...", sig)
	}

	// gracefully shut down any component
	if statsd != nil {
		statsd.Stop()
	}
	common.Collector.Stop()
	metaCollector.Stop()
	api.StopServer()
	fwd.Stop()
	os.Remove(pidfilePath)
	log.Info("See ya!")
	log.Flush()
	os.Exit(0)
}
