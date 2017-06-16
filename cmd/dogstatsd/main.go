package main

import (
	_ "expvar"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	log "github.com/cihub/seelog"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
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

	confPath   string
	socketPath string
)

func init() {
	// attach the command to the root
	dogstatsdCmd.AddCommand(startCmd)
	dogstatsdCmd.AddCommand(versionCmd)

	// local flags
	startCmd.Flags().StringVarP(&confPath, "cfgpath", "f", "", "path to datadog.yaml")
	config.Datadog.BindPFlag("conf_path", startCmd.Flags().Lookup("cfgpath"))
	startCmd.Flags().StringVarP(&socketPath, "socket", "s", "", "listen to this socket instead of UDP")
	config.Datadog.BindPFlag("dogstatsd_socket", startCmd.Flags().Lookup("socket"))
}

func start(cmd *cobra.Command, args []string) error {
	config.Datadog.SetConfigFile(config.Datadog.GetString("conf_path"))
	confErr := config.Datadog.ReadInConfig()

	// Setup logger
	err := config.SetupLogger(config.Datadog.GetString("log_level"), config.Datadog.GetString("log_file"))
	if err != nil {
		log.Criticalf("Unable to setup logger: %s", err)
		return nil
	}

	if confErr != nil {
		log.Infof("unable to parse Datadog config file, running with env variables: %s", confErr)
	}

	if !config.Datadog.IsSet("api_key") {
		log.Critical("no API key configured, exiting")
		return nil
	}

	// for now we handle only one key and one domain
	keysPerDomain := map[string][]string{
		config.Datadog.GetString("dd_url"): {
			config.Datadog.GetString("api_key"),
		},
	}
	f := forwarder.NewDefaultForwarder(keysPerDomain)
	f.Start()

	hname, err := util.GetHostname()
	if err != nil {
		log.Warnf("Error getting hostname: %s\n", err)
		hname = ""
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

	statsd.Stop()
	log.Info("See ya!")
	log.Flush()
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
