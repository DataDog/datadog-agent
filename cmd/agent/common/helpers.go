package common

import (
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/autodiscovery"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed"
	"github.com/DataDog/datadog-agent/pkg/collector/providers"
	"github.com/DataDog/datadog-agent/pkg/collector/py"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
)

// SetupAutoConfig instantiate the global AutoConfig object and sets up
// the Agent configuration providers and check loaders
func SetupAutoConfig(confdPath string) {
	// create the Collector instance and start all the components
	// NOTICE: this will also setup the Python environment
	coll := collector.NewCollector(GetDistPath(), filepath.Join(GetDistPath(), "checks"),
		config.Datadog.GetString("additional_checksd"), PyChecksPath)

	// create the Autoconfig instance
	AC = autodiscovery.NewAutoConfig(coll)

	// add the check loaders
	if loader := py.NewPythonCheckLoader(); loader != nil {
		AC.AddLoader(loader)
	} else {
		log.Errorf("Unable to create Python loader.")
	}

	// can't fail
	AC.AddLoader(core.NewGoCheckLoader())

	if loader := embed.NewJMXCheckLoader(); loader != nil {
		AC.AddLoader(loader)
	} else {
		log.Errorf("Unable to create JMX loader.")
	}

	// add the configuration providers
	// File Provider
	confSearchPaths := []string{
		confdPath,
		filepath.Join(GetDistPath(), "conf.d"),
	}
	AC.AddProvider(providers.NewFileConfigProvider(confSearchPaths), false)

	// Etcd Provider
	etcd, err := providers.NewEtcdConfigProvider()
	if err != nil {
		log.Errorf("Cannot use the etcd config provider: %s", err)
	} else {
		AC.AddProvider(etcd, true)
	}
}

// SetupConfig fires up the configuration system
func SetupConfig(confFilePath string) {
	// set the paths where a config file is expected
	if len(confFilePath) != 0 {
		// if the configuration file path was supplied on the command line,
		// add that first so it's first in line
		config.Datadog.AddConfigPath(confFilePath)
	}
	config.Datadog.AddConfigPath(defaultConfPath)
	config.Datadog.AddConfigPath(GetDistPath())

	// load the configuration
	err := config.Datadog.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("unable to load Datadog config file: %s", err))
	}
}
