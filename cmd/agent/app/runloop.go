package app

import (
	"path"

	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/api"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/metadata"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
	log "github.com/cihub/seelog"

	// register core checks
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/network"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system"
)

var (
	// flags variables
	runForeground bool
	pidfilePath   string
	confdPath     string
	// ConfFilePath holds the path to the folder containing the configuration
	// file, for override from the command line
	confFilePath string
)

// StartAgent Initializes the agent process
func StartAgent() (*dogstatsd.Server, *metadata.Collector, *forwarder.Forwarder) {

	if pidfilePath != "" {
		err := pidfile.WritePID(pidfilePath)
		if err != nil {
			panic(err)
		}
	}

	// Global Agent configuration
	common.SetupConfig(confFilePath)

	// Setup logger
	err := config.SetupLogger(config.Datadog.GetString("log_level"), config.Datadog.GetString("log_file"))
	if err != nil {
		panic(err)
	}

	hostname := util.GetHostname()

	// store the computed hostname in the global cache
	key := path.Join(util.AgentCachePrefix, "hostname")
	util.Cache.Set(key, hostname, util.NoExpiration)

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
	log.Debugf("Starting forwarder")
	fwd.Start()
	log.Debugf("Forwarder started")

	// setup the aggregator
	agg := aggregator.InitAggregator(fwd, hostname)
	agg.AddAgentStartupEvent(version.AgentVersion)

	// start dogstatsd
	var statsd *dogstatsd.Server
	if config.Datadog.GetBool("use_dogstatsd") {
		var err error
		statsd, err = dogstatsd.NewServer(agg.GetChannels())
		if err != nil {
			log.Errorf("Could not start dogstatsd: %s", err)
		}
	}
	log.Debugf("statsd started")

	// create and setup the Autoconfig instance
	common.SetupAutoConfig(config.Datadog.GetString("confd_path"))

	// setup the metadata collector, this needs a working Python env to function
	metaCollector := metadata.NewCollector(fwd, config.Datadog.GetString("api_key"), hostname)
	log.Debugf("metaCollector created")
	return statsd, metaCollector, fwd
}

// StopAgent Tears down the agent process
func StopAgent(statsd *dogstatsd.Server, metaCollector *metadata.Collector, fwd *forwarder.Forwarder) {
	// gracefully shut down any component
	if statsd != nil {
		statsd.Stop()
	}
	common.AC.Stop()
	metaCollector.Stop()
	api.StopServer()
	fwd.Stop()
	os.Remove(pidfilePath)
	log.Info("See ya!")
	log.Flush()
}
