// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package app

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	cmdconfig "github.com/DataDog/datadog-agent/cmd/cluster-agent/commands/config"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	ClusterAgentCmd.AddCommand(cmdconfig.Config(getSettingsClient))
}

func setupConfig() error {
	if flagNoColor {
		color.NoColor = true
	}

	// we'll search for a config file named `datadog-cluster.yaml`
	config.Datadog.SetConfigName("datadog-cluster")
	err := common.SetupConfig(confPath)
	if err != nil {
		return fmt.Errorf("unable to set up global cluster agent configuration: %v", err)
	}

	err = config.SetupLogger(loggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
	if err != nil {
		fmt.Printf("Cannot setup logger, exiting: %v\n", err)
		return err
	}

	return util.SetAuthToken()
}

func getSettingsClient(_ *cobra.Command, _ []string) (commonsettings.Client, error) {
	err := setupConfig()
	if err != nil {
		return nil, err
	}

	c := util.GetClient(false)
	apiConfigURL := fmt.Sprintf("https://localhost:%v/config", config.Datadog.GetInt("cluster_agent.cmd_port"))

	return settingshttp.NewClient(c, apiConfigURL, "datadog-cluster-agent"), nil
}

// initRuntimeSettings builds the map of runtime Cluster Agent settings configurable at runtime.
func initRuntimeSettings() error {
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.LogLevelRuntimeSetting{}); err != nil {
		return err
	}

	if err := commonsettings.RegisterRuntimeSetting(commonsettings.RuntimeMutexProfileFraction("runtime_mutex_profile_fraction")); err != nil {
		return err
	}

	if err := commonsettings.RegisterRuntimeSetting(commonsettings.RuntimeBlockProfileRate("runtime_block_profile_rate")); err != nil {
		return err
	}

	if err := commonsettings.RegisterRuntimeSetting(commonsettings.ProfilingGoroutines("internal_profiling_goroutines")); err != nil {
		return err
	}

	return commonsettings.RegisterRuntimeSetting(commonsettings.ProfilingRuntimeSetting("internal_profiling"))
}
