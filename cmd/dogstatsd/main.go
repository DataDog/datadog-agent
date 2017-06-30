package main

import (
	_ "expvar"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/cihub/seelog"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	// dogstatsdCmd is the root command
	dogstatsdCmd = &cobra.Command{
		Use:   "dogstatsd [command]",
		Short: "Datadog dogstatsd at your service.",
		Long: `
DogStatsD accepts custom application metrics points over UDP, and then
periodically aggregates and forwards them to Datadog, where they can be graphed
on dashboards. DogStatsD implements the StatsD protocol, along with a few
extensions for special Datadog features.`,
	}

	startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start DogStatsD",
		Long:  `Runs DogStatsD in the foreground`,
		RunE:  start,
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			av, _ := version.New(version.AgentVersion)
			fmt.Println(fmt.Sprintf("DogStatsD from Agent %s - Codename: %s - Commit: %s", av.GetNumber(), av.Meta, av.Commit))
		},
	}

	confPath    string
	socketPath  string
	legacyAgent bool
)

// run the host metadata collector every 14400 seconds (4 hours)
const hostMetadataCollectorInterval = 14400

func init() {
	// attach the command to the root
	dogstatsdCmd.AddCommand(startCmd)
	dogstatsdCmd.AddCommand(versionCmd)

	// local flags
	startCmd.Flags().StringVarP(&confPath, "cfgpath", "f", "", "path to datadog.yaml")
	config.Datadog.BindPFlag("conf_path", startCmd.Flags().Lookup("cfgpath"))
	startCmd.Flags().BoolVarP(&legacyAgent, "legacy_agent", "l", false, "Run dogstatsd on a legacy agent(5)")
}

func start(cmd *cobra.Command, args []string) error {
	config.Datadog.SetConfigFile(config.Datadog.GetString("conf_path"))
	confErr := config.Datadog.ReadInConfig()

	// Setup logger
	logFile := config.Datadog.GetString("log_file")
	if legacyAgent {
		logFile = config.Datadog.GetString("legacy_dogstatsd_log")
	}
	err := config.SetupLogger(config.Datadog.GetString("log_level"), logFile)
	if err != nil {
		log.Criticalf("Unable to setup logger: %s", err)
		return nil
	}
	defer log.Flush()

	if confErr != nil && legacyAgent {
		log.Infof("unable to parse Datadog config file, error: %v", confErr)
		log.Info("attempting to load legacy agent5 config...")
		confErr = config.ReadLegacyConfig()
	}

	if confErr != nil {
		log.Infof("unable to parse any Datadog config file, running with env variables: %s", confErr)
	}
	if legacyAgent && (!config.Datadog.GetBool("dogstatsd6_enable") || !config.Datadog.GetBool("use_dogstatsd")) {
		log.Infof("running in legacy mode but dogstatsd6 not enabled - shutting down")
		time.Sleep(4 * time.Second)
		return nil // clean exit.
	}

	if !config.Datadog.IsSet("api_key") {
		log.Critical("no API key configured, exiting")
		return nil
	}

	// setup the forwarder
	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	f := forwarder.NewDefaultForwarder(keysPerDomain)
	f.Start()

	hname, err := util.GetHostname()
	if err != nil {
		log.Warnf("Error getting hostname: %s\n", err)
		hname = ""
	}

	// start metadata collection
	metaCollector := metadata.NewScheduler(f, config.Datadog.GetString("api_key"), hname)

	// add the host metadata collector
	err = metaCollector.AddCollector("host", hostMetadataCollectorInterval*time.Second)
	if err != nil {
		panic("Host metadata is supposed to be always available in the catalog!")
	}

	aggregatorInstance := aggregator.InitAggregator(f, hname)
	statsd, err := dogstatsd.NewServer(aggregatorInstance.GetChannels())
	if err != nil {
		log.Criticalf("Unable to start dogstatsd: %s", err)
		return nil
	}

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Block here until we receive the interrupt signal
	<-signalCh

	metaCollector.Stop()
	statsd.Stop()
	log.Info("See ya!")
	return nil
}

func main() {
	// go_expvar server
	go http.ListenAndServe("127.0.0.1:5000", http.DefaultServeMux)

	if err := dogstatsdCmd.Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}
