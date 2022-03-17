// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"path"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
)

const (
	instancePath   string = "instances"
	checkNamePath  string = "check_names"
	initConfigPath string = "init_configs"
)

func init() {
	// Where to look for check templates if no custom path is defined
	config.Datadog.SetDefault("autoconf_template_dir", "/datadog/check_configs")
	// Defaut Timeout in second when talking to storage for configuration (etcd, zookeeper, ...)
	config.Datadog.SetDefault("autoconf_template_url_timeout", 5)
}

func buildStoreKey(key ...string) string {
	parts := []string{config.Datadog.GetString("autoconf_template_dir")}
	parts = append(parts, key...)
	return path.Join(parts...)
}

// GetPollInterval computes the poll interval from the config
func GetPollInterval(cp config.ConfigurationProviders) time.Duration {
	if cp.PollInterval != "" {
		customInterval, err := time.ParseDuration(cp.PollInterval)
		if err == nil {
			return customInterval
		}
	}
	return config.Datadog.GetDuration("ad_config_poll_interval") * time.Second
}

// providerCache supports monitoring a service for changes either to the number
// of things being monitored, or to one of those things being modified.  This
// can be used to determine IsUpToDate() and avoid full Collect() calls when
// nothing has changed.
type providerCache struct {
	// mostRecentMod is the most recent modification timestamp of a
	// monitored thing
	mostRecentMod float64

	// count is the number of monitored things
	count int
}

// ErrorMsgSet contains a list of unique configuration errors for a provider
type ErrorMsgSet map[string]struct{}

// newProviderCache instantiate a ProviderCache.
func newProviderCache() *providerCache {
	return &providerCache{
		mostRecentMod: 0,
		count:         0,
	}
}
