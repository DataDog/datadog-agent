package app

import (
	"path"
	"time"

	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/api"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
	log "github.com/cihub/seelog"

	// register core checks
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/network"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system"

	// register metadata providers
	_ "github.com/DataDog/datadog-agent/pkg/metadata/host"
	_ "github.com/DataDog/datadog-agent/pkg/metadata/resources"
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

// run the host metadata collector every 14400 seconds (4 hours)
const hostMetadataCollectorInterval = 14400

// StartAgent Initializes the agent process
func StartAgent() {

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

	hostname, err := util.GetHostname()
	if err != nil {
		panic(err)
	}

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
	common.Forwarder = forwarder.NewDefaultForwarder(keysPerDomain)
	log.Debugf("Starting forwarder")
	common.Forwarder.Start()
	log.Debugf("Forwarder started")

	// setup the aggregator
	agg := aggregator.InitAggregator(common.Forwarder, hostname)
	agg.AddAgentStartupEvent(version.AgentVersion)

	// start
	if config.Datadog.GetBool("use_dogstatsd") {
		err := common.CreateDSD(agg)
		if err != nil {
			log.Errorf("Could not start dogstatsd: %s", err)
		} else {
			log.Debugf("statsd started")
		}
	}

	// create and setup the Autoconfig instance
	common.SetupAutoConfig(config.Datadog.GetString("confd_path"))

	// setup the metadata collector, this needs a working Python env to function
	common.MetadataScheduler = metadata.NewScheduler(common.Forwarder, config.Datadog.GetString("api_key"), hostname)
	var C []config.MetadataProviders
	err = config.Datadog.UnmarshalKey("metadata_providers", &C)
	if err == nil {
		log.Debugf("Adding configured providers to the metadata collector")
		for _, c := range C {
			if c.Name == "host" {
				continue
			}
			intl := c.Interval * time.Second
			err = common.MetadataScheduler.AddCollector(c.Name, intl)
			if err != nil {
				log.Errorf("Unable to add '%s' metadata provider: %v", c.Name, err)
			} else {
				log.Infof("Scheduled metadata provider '%v' to run every %v", c.Name, intl)
			}
		}
	} else {
		log.Errorf("Unable to parse metadata_providers config: %v", err)
	}

	// always add the host metadata collector, this is not user-configurable by design
	err = common.MetadataScheduler.AddCollector("host", hostMetadataCollectorInterval*time.Second)
	if err != nil {
		panic("Host metadata is supposed to be always available in the catalog!")
	}
}

// StopAgent Tears down the agent process
func StopAgent() {
	// gracefully shut down any component
	common.StopDSD()
	common.AC.Stop()
	common.MetadataScheduler.Stop()
	api.StopServer()
	common.Forwarder.Stop()
	os.Remove(pidfilePath)
	log.Info("See ya!")
	log.Flush()
}
