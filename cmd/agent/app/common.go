package app

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func setupConfig() {
	// set the paths where a config file is expected
	for _, path := range configPaths {
		config.Datadog.AddConfigPath(path)
	}

	// load the configuration
	err := config.Datadog.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("unable to load Datadog config file: %s", err))
	}

	// define defaults for the Agent
	config.Datadog.SetDefault("cmd_sock", "/tmp/agent.sock")
	config.Datadog.BindEnv("cmd_sock")
}
