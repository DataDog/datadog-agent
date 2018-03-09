package app

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "expvar" // Blank import used because this isn't directly used in this file

	log "github.com/cihub/seelog"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/api"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// FIXME: move SetupAutoConfig and StartAutoConfig in their own package so we don't import cmd/agent
var (
	ClusterAgentCmd = &cobra.Command{
		Use:   "cluster-agent [command]",
		Short: "Datadog Cluster Agent at your service.",
		Long: `
Datadog Cluster Agent takes care of running checks that need run only once per cluster.
It also exposes an API for other Datadog agents that provides them with cluster-level
metadata for their metrics.`,
	}

	startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the Cluster Agent",
		Long:  `Runs Datadog Cluster agent in the foreground`,
		RunE:  start,
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			av, _ := version.New(version.AgentVersion, version.Commit)
			fmt.Println(fmt.Sprintf("Cluster Agent from Agent %s - Codename: %s - Commit: %s - Serialization version: %s", av.GetNumber(), av.Meta, av.Commit, serializer.AgentPayloadVersion))
		},
	}

	confPath    string
	flagNoColor bool
)

func init() {
	// attach the command to the root
	ClusterAgentCmd.AddCommand(startCmd)
	ClusterAgentCmd.AddCommand(versionCmd)

	ClusterAgentCmd.PersistentFlags().StringVarP(&confPath, "cfgpath", "c", "", "path to directory containing datadog-cluster.yaml")
	ClusterAgentCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")
}

func start(cmd *cobra.Command, args []string) error {
	configFound := false

	// a path to the folder containing the config file was passed
	if len(confPath) != 0 {
		// we'll search for a config file named `datadog-cluster.yaml`
		config.Datadog.AddConfigPath(confPath)
		confErr := config.Datadog.ReadInConfig()
		if confErr != nil {
			log.Error(confErr)
		} else {
			configFound = true
		}
	}

	if !configFound {
		log.Infof("Config read from env variables")
	}

	// Setup logger
	syslogURI := config.GetSyslogURI()
	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultLogFile
	}
	if config.Datadog.GetBool("disable_file_logging") {
		// this will prevent any logging on file
		logFile = ""
	}

	err := config.SetupLogger(
		config.Datadog.GetString("log_level"),
		logFile,
		syslogURI,
		config.Datadog.GetBool("syslog_rfc"),
		config.Datadog.GetBool("syslog_tls"),
		config.Datadog.GetString("syslog_pem"),
		config.Datadog.GetBool("log_to_console"),
		config.Datadog.GetBool("log_format_json"),
	)
	if err != nil {
		log.Criticalf("Unable to setup logger: %s", err)
		return nil
	}

	if !config.Datadog.IsSet("api_key") {
		log.Critical("no API key configured, exiting")
		return nil
	}

	// get hostname
	hostname, err := util.GetHostname()
	if err != nil {
		return log.Errorf("Error while getting hostname, exiting: %v", err)
	}
	log.Infof("Hostname is: %s", hostname)

	// start the cmd HTTP server
	if err = api.StartServer(); err != nil {
		return log.Errorf("Error while starting api server, exiting: %v", err)
	}

	// setup the forwarder
	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	f := forwarder.NewDefaultForwarder(keysPerDomain)
	f.Start()
	s := &serializer.Serializer{Forwarder: f}

	hname, err := util.GetHostname()
	if err != nil {
		log.Warnf("Error getting hostname: %s", err)
		hname = ""
	}
	log.Debugf("Using hostname: %s", hname)

	aggregatorInstance := aggregator.InitAggregator(s, hname)
	aggregatorInstance.AddAgentStartupEvent("DCA")

	clusterAgent, err := clusteragent.Run(aggregatorInstance.GetChannels())
	if err != nil {
		log.Errorf("Could not start the Cluster Agent Process.")

	}
	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	// create and setup the Autoconfig instance
	common.SetupAutoConfig(config.Datadog.GetString("confd_dca_path"))
	// start the autoconfig, this will immediately run any configured check
	common.StartAutoConfig()
	// Block here until we receive the interrupt signal
	<-signalCh

	clusterAgent.Stop()
	log.Info("See ya!")
	log.Flush()
	return nil
}
