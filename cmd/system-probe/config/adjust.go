// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// adjustConfig makes changes to the raw config based on deprecations and inferences.
func adjustConfig(cfg config.Config) {
	deprecateString(cfg, spNS("log_level"), "log_level")
	deprecateString(cfg, spNS("log_file"), "log_file")

	usmEnabled := cfg.GetBool(smNS("enabled"))
	dsmEnabled := cfg.GetBool(dsmNS("enabled"))
	// this check must come first, so we can accurately tell if system_probe was explicitly enabled
	if cfg.GetBool(spNS("enabled")) &&
		!cfg.IsSet(netNS("enabled")) &&
		!usmEnabled &&
		!dsmEnabled {
		// This case exists to preserve backwards compatibility. If system_probe_config.enabled is explicitly set to true, and there is no network_config block,
		// enable the connections/network check.
		log.Info(deprecationMessage(spNS("enabled"), netNS("enabled")))
		// ensure others can key off of this single config value for NPM status
		cfg.Set(netNS("enabled"), true)
	}

	validateString(cfg, spNS("sysprobe_socket"), defaultSystemProbeAddress, ValidateSocketAddress)

	adjustNetwork(cfg)
	adjustUSM(cfg)
	adjustSecurity(cfg)
}

func validateString(cfg config.Config, key string, defaultVal string, fn func(string) error) {
	if cfg.IsSet(key) {
		if err := fn(cfg.GetString(key)); err != nil {
			log.Errorf("error validating `%s`: %s. using default value of `%s`", key, err, defaultVal)
			cfg.Set(key, defaultVal)
		}
	} else {
		cfg.Set(key, defaultVal)
	}
}

func validateInt(cfg config.Config, key string, defaultVal int, fn func(int) error) {
	if cfg.IsSet(key) {
		if err := fn(cfg.GetInt(key)); err != nil {
			log.Errorf("error validating `%s`: %s. using default value of `%d`", key, err, defaultVal)
			cfg.Set(key, defaultVal)
		}
	} else {
		cfg.Set(key, defaultVal)
	}
}

func validateInt64(cfg config.Config, key string, defaultVal int64, fn func(int64) error) {
	if cfg.IsSet(key) {
		if err := fn(cfg.GetInt64(key)); err != nil {
			log.Errorf("error validating `%s`: %s. using default value of `%d`", key, err, defaultVal)
			cfg.Set(key, defaultVal)
		}
	} else {
		cfg.Set(key, defaultVal)
	}
}

func applyDefault(cfg config.Config, key string, defaultVal interface{}) {
	if !cfg.IsSet(key) {
		cfg.Set(key, defaultVal)
	}
}

// deprecateBool sets `newkey` to the value at `oldkey` and logs a deprecation message,
// but only if `oldkey` is set and `newkey` is not set.
func deprecateBool(cfg config.Config, oldkey string, newkey string) {
	deprecateCustom(cfg, oldkey, newkey, func(cfg config.Config) interface{} {
		return cfg.GetBool(oldkey)
	})
}

func deprecateString(cfg config.Config, oldkey string, newkey string) {
	deprecateCustom(cfg, oldkey, newkey, func(cfg config.Config) interface{} {
		return cfg.GetString(oldkey)
	})
}

func deprecateCustom(cfg config.Config, oldkey string, newkey string, getFn func(config.Config) interface{}) {
	if cfg.IsSet(oldkey) && !cfg.IsSet(newkey) {
		log.Info(deprecationMessage(oldkey, newkey))
		cfg.Set(newkey, getFn(cfg))
	}
}

func deprecationMessage(oldkey, newkey string) string {
	return fmt.Sprintf("configuration key `%s` is deprecated, use `%s` instead", oldkey, newkey)
}

func limitMaxInt(cfg config.Config, key string, max int) {
	val := cfg.GetInt(key)
	if val > max {
		log.Warnf("configuration key `%s` was set to `%d`, using maximum value `%d` instead", key, val, max)
		cfg.Set(key, max)
	}
}

func limitMaxInt64(cfg config.Config, key string, max int64) {
	val := cfg.GetInt64(key)
	if val > max {
		log.Warnf("configuration key `%s` was set to `%d`, using maximum value `%d` instead", key, val, max)
		cfg.Set(key, max)
	}
}
