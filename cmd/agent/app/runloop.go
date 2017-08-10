package app

import (
	"path"
	"time"

	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/api"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/serializer"
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

// run the agent checks metadata collector every 600 seconds (10 minutes)
const agentChecksMetadataCollectorInterval = 600

// StartAgent Initializes the agent process
func StartAgent() {
	// Global Agent configuration
	common.SetupConfig(confFilePath)

	// Setup logger
	err := config.SetupLogger(config.Datadog.GetString("log_level"), config.Datadog.GetString("log_file"))
	if err != nil {
		panic(err)
	}

	log.Infof("Starting Datadog Agent v%v", version.AgentVersion)

	if pidfilePath != "" {
		err := pidfile.WritePID(pidfilePath)
		if err != nil {
			panic(err)
		}
		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), pidfilePath)
	}

	hostname, err := util.GetHostname()
	if err != nil {
		panic(err)
	}

	// store the computed hostname in the global cache
	key := path.Join(util.AgentCachePrefix, "hostname")
	util.Cache.Set(key, hostname, util.NoExpiration)

	log.Infof("Hostname is: %s", hostname)

	// start the cmd HTTP server
	api.StartServer()

	// setup the forwarder
	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	common.Forwarder = forwarder.NewDefaultForwarder(keysPerDomain)
	log.Debugf("Starting forwarder")
	common.Forwarder.Start()
	log.Debugf("Forwarder started")

	// setup the aggregator
	s := &serializer.Serializer{Forwarder: common.Forwarder}
	agg := aggregator.InitAggregator(s, hostname)
	agg.AddAgentStartupEvent(version.AgentVersion)

	// start dogstatsd
	if config.Datadog.GetBool("use_dogstatsd") {
		var err error
		common.DSD, err = dogstatsd.NewServer(agg.GetChannels())
		if err != nil {
			log.Errorf("Could not start dogstatsd: %s", err)
		}
	}
	log.Debugf("statsd started")

	// create and setup the Autoconfig instance
	common.SetupAutoConfig(config.Datadog.GetString("confd_path"))
	// start the autoconfig, this will immediately run any configured check
	common.StartAutoConfig()

	// setup the metadata collector, this needs a working Python env to function
	if config.Datadog.GetBool("enable_metadata_collection") {
		common.MetadataScheduler = metadata.NewScheduler(s, hostname)
		var C []config.MetadataProviders
		err = config.Datadog.UnmarshalKey("metadata_providers", &C)
		if err == nil {
			log.Debugf("Adding configured providers to the metadata collector")
			for _, c := range C {
				if c.Name == "host" || c.Name == "agent_checks" {
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
		// Should be always true, except in some edge cases (multiple agents per host)
		err = common.MetadataScheduler.AddCollector("host", hostMetadataCollectorInterval*time.Second)
		if err != nil {
			panic("Host metadata is supposed to be always available in the catalog!")
		}
		err = common.MetadataScheduler.AddCollector("agent_checks", agentChecksMetadataCollectorInterval*time.Second)
		if err != nil {
			panic("Agent Checks metadata is supposed to be always available in the catalog!")
		}
	} else {
		log.Warnf("Metadata collection disabled, only do that if another agent/dogstatsd is running on this host")
	}
}

// StopAgent Tears down the agent process
func StopAgent() {
	// gracefully shut down any component
	if common.DSD != nil {
		common.DSD.Stop()
	}
	common.AC.Stop()
	if common.MetadataScheduler != nil {
		common.MetadataScheduler.Stop()
	}
	api.StopServer()
	common.Forwarder.Stop()
	os.Remove(pidfilePath)
	log.Info("See ya!")
	log.Flush()
}
