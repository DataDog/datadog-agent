package app

import (
	"path"
	"path/filepath"

	"os"

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

	hostname := common.GetHostname()
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
	fwd.Start()

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
	return statsd, metaCollector, fwd
}

// StopAgent Tears down the agent process
func StopAgent(statsd *dogstatsd.Server, metaCollector *metadata.Collector, fwd *forwarder.Forwarder) {
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

}
