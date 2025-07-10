// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"path"
	"strconv"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const (
	//nolint needed as these constants are defined in a file without a build tag,
	// but only used in multiple files with different build tags, none of which
	// are used in the IoT Agent.
	//nolint:unused,deadcode
	instancePath string = "instances"
	//nolint:unused,deadcode
	checkNamePath string = "check_names"
	//nolint:unused,deadcode
	initConfigPath string = "init_configs"
)

func buildStoreKey(key ...string) string {
	parts := []string{pkgconfigsetup.Datadog().GetString("autoconf_template_dir")}
	parts = append(parts, key...)
	return path.Join(parts...)
}

// GetPollInterval computes the poll interval from the config
func GetPollInterval(cp pkgconfigsetup.ConfigurationProviders) time.Duration {
	if cp.PollInterval != "" {
		customInterval, err := time.ParseDuration(cp.PollInterval)
		if err == nil {
			return customInterval
		}
	}
	return pkgconfigsetup.Datadog().GetDuration("ad_config_poll_interval") * time.Second
}

// providerCache supports monitoring a service for changes either to the number
// of things being monitored, or to one of those things being modified.  This
// can be used to determine IsUpToDate() and avoid full Collect() calls when
// nothing has changed.
// nolint needed as this type is defined in a file without a build tag,
// but only used in multiple files with different build tags, none of which
// are used in the IoT Agent.
//
//nolint:unused
type providerCache struct {
	// mostRecentMod is the most recent modification timestamp of a
	// monitored thing
	mostRecentMod float64

	// count is the number of monitored things
	count int
}

// newProviderCache instantiate a ProviderCache.
// nolint needed as this function is defined in a file without a build tag,
// but only used in multiple files with different build tags, none of which
// are used in the IoT Agent.
//
//nolint:deadcode,unused
func newProviderCache() *providerCache {
	return &providerCache{
		mostRecentMod: 0,
		count:         0,
	}
}

// ignoreADTagsFromAnnotations returns of the `ad.datadoghq.com/{endpoints,service}.ignore_autodiscovery_tags` annotation
// TODO(CINT)(Agent 7.53+) Remove support for hybrid scenarios
//
//nolint:deadcode,unused
func ignoreADTagsFromAnnotations(annotations map[string]string, prefix string) bool {
	ignoreAdForHybridScenariosTags, _ := strconv.ParseBool(annotations[prefix+"ignore_autodiscovery_tags"])
	return ignoreAdForHybridScenariosTags
}
