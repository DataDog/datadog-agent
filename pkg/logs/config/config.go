// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"github.com/spf13/viper"
)

// Config represents the configuration object logs-agent uses to initialize its data pipeline
type Config struct {
	apiKey string
	logset string

	ddURL          string
	ddPort         int
	runPath        string
	openFilesLimit int

	devModeNoSSL bool

	logsSources *LogSources

	numberOfPipelines int
	chanSize          int
}

// GetAPIKey returns the API key of the user
func (c *Config) GetAPIKey() string {
	return c.apiKey
}

// GetLogset returns the logset of the user
func (c *Config) GetLogset() string {
	return c.logset
}

// GetDDURL returns the URL to connect to the backend
func (c *Config) GetDDURL() string {
	return c.ddURL
}

// GetDDPort returns the port to connect to the backend
func (c *Config) GetDDPort() int {
	return c.ddPort
}

// GetRunPath returns the run path where are stored logs-agent context files
func (c *Config) GetRunPath() string {
	return c.runPath
}

// GetOpenFilesLimit returns the maximum number of files the agent can open at the same time
func (c *Config) GetOpenFilesLimit() int {
	return c.openFilesLimit
}

// GetDevModeNoSSL returns whether the agent should skip SSL validation while sending logs to the backend (only used for debug purpose)
func (c *Config) GetDevModeNoSSL() bool {
	return c.devModeNoSSL
}

// GetLogsSources returns the list of logs sources
func (c *Config) GetLogsSources() *LogSources {
	return c.logsSources
}

// GetNumberOfPipelines returns the number of pipelines that are running in parallel
func (c *Config) GetNumberOfPipelines() int {
	return c.numberOfPipelines
}

// GetChanSize returns the sise of the channels of the pipelines
func (c *Config) GetChanSize() int {
	return c.chanSize
}

// Build returns the logs-agent Config
func Build(config *viper.Viper) (*Config, error) {
	sources, err := buildLogSources(config.GetString("confd_path"))
	if err != nil {
		return nil, err
	}
	return build(config, sources), nil
}

// build return a Config aggregating data from config, logSources and default constants
func build(config *viper.Viper, logSources *LogSources) *Config {
	return &Config{
		apiKey:            config.GetString("api_key"),
		logset:            config.GetString("logset"),
		ddURL:             config.GetString("logs_config.dd_url"),
		ddPort:            config.GetInt("logs_config.dd_port"),
		runPath:           config.GetString("logs_config.run_path"),
		openFilesLimit:    config.GetInt("logs_config.open_files_limit"),
		devModeNoSSL:      config.GetBool("logs_config.dev_mode_no_ssl"),
		logsSources:       logSources,
		numberOfPipelines: defaultNumberOfPipelines,
		chanSize:          defaultChanSize,
	}
}
