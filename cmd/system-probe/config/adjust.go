// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var adjustMtx sync.Mutex

// Adjust makes changes to the raw config based on deprecations and inferences.
func Adjust(cfg config.Config) {
	adjustMtx.Lock()
	defer adjustMtx.Unlock()
	if cfg.GetBool(spNS("adjusted")) {
		return
	}

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
		log.Warn(deprecationMessage(spNS("enabled"), netNS("enabled")))
		// ensure others can key off of this single config value for NPM status
		cfg.Set(netNS("enabled"), true)
	}

	validateString(cfg, spNS("sysprobe_socket"), defaultSystemProbeAddress, ValidateSocketAddress)

	adjustNetwork(cfg)
	adjustUSM(cfg)
	adjustSecurity(cfg)

	cfg.Set(spNS("adjusted"), true)
}

// validateString validates the string configuration value at `key` using a custom provided function `valFn`.
// If `key` is not set or `valFn` returns an error, the `defaultVal` is used instead.
func validateString(cfg config.Config, key string, defaultVal string, valFn func(string) error) {
	if cfg.IsSet(key) {
		if err := valFn(cfg.GetString(key)); err != nil {
			log.Errorf("error validating `%s`: %s, using default value of `%s`", key, err, defaultVal)
			cfg.Set(key, defaultVal)
		}
	} else {
		cfg.Set(key, defaultVal)
	}
}

// validateInt validates the int configuration value at `key` using a custom provided function `valFn`.
// If `key` is not set or `valFn` returns an error, the `defaultVal` is used instead.
func validateInt(cfg config.Config, key string, defaultVal int, valFn func(int) error) {
	if cfg.IsSet(key) {
		if err := valFn(cfg.GetInt(key)); err != nil {
			log.Errorf("error validating `%s`: %s, using default value of `%d`", key, err, defaultVal)
			cfg.Set(key, defaultVal)
		}
	} else {
		cfg.Set(key, defaultVal)
	}
}

// validateInt64 validates the int64 configuration value at `key` using a custom provided function `valFn`.
// If `key` is not set or `valFn` returns an error, the `defaultVal` is used instead.
func validateInt64(cfg config.Config, key string, defaultVal int64, valFn func(int64) error) {
	if cfg.IsSet(key) {
		if err := valFn(cfg.GetInt64(key)); err != nil {
			log.Errorf("error validating `%s`: %s. using default value of `%d`", key, err, defaultVal)
			cfg.Set(key, defaultVal)
		}
	} else {
		cfg.Set(key, defaultVal)
	}
}

// applyDefault sets configuration `key` to `defaultVal` only if not previously set.
func applyDefault(cfg config.Config, key string, defaultVal interface{}) {
	if !cfg.IsSet(key) {
		cfg.Set(key, defaultVal)
	}
}

// deprecateBool logs a deprecation message if `oldkey` is used.
// It sets `newkey` to the value obtained from `getFn`, but only if `oldkey` is set and `newkey` is not set.
func deprecateBool(cfg config.Config, oldkey string, newkey string) {
	deprecateCustom(cfg, oldkey, newkey, func(cfg config.Config) interface{} {
		return cfg.GetBool(oldkey)
	})
}

// deprecateInt64 logs a deprecation message if `oldkey` is used.
// It sets `newkey` to the value obtained from `getFn`, but only if `oldkey` is set and `newkey` is not set.
func deprecateInt64(cfg config.Config, oldkey string, newkey string) {
	deprecateCustom(cfg, oldkey, newkey, func(cfg config.Config) interface{} {
		return cfg.GetInt64(oldkey)
	})
}

// deprecateGeneric logs a deprecation message if `oldkey` is used.
// It sets `newkey` to the value obtained from `getFn`, but only if `oldkey` is set and `newkey` is not set.
func deprecateGeneric(cfg config.Config, oldkey string, newkey string) {
	deprecateCustom(cfg, oldkey, newkey, func(cfg config.Config) interface{} {
		return cfg.Get(oldkey)
	})
}

// deprecateInt logs a deprecation message if `oldkey` is used.
// It sets `newkey` to the value obtained from `getFn`, but only if `oldkey` is set and `newkey` is not set.
func deprecateInt(cfg config.Config, oldkey string, newkey string) {
	deprecateCustom(cfg, oldkey, newkey, func(cfg config.Config) interface{} {
		return cfg.GetInt(oldkey)
	})
}

// deprecateString logs a deprecation message if `oldkey` is used.
// It sets `newkey` to the value obtained from `getFn`, but only if `oldkey` is set and `newkey` is not set.
func deprecateString(cfg config.Config, oldkey string, newkey string) {
	deprecateCustom(cfg, oldkey, newkey, func(cfg config.Config) interface{} {
		return cfg.GetString(oldkey)
	})
}

// deprecateCustom logs a deprecation message if `oldkey` is used.
// It sets `newkey` to the value obtained from `getFn`, but only if `oldkey` is set and `newkey` is not set.
func deprecateCustom(cfg config.Config, oldkey string, newkey string, getFn func(config.Config) interface{}) {
	if cfg.IsSet(oldkey) {
		log.Warn(deprecationMessage(oldkey, newkey))
		if !cfg.IsSet(newkey) {
			cfg.Set(newkey, getFn(cfg))
		}
	}
}

// deprecationMessage returns the standard deprecation message
func deprecationMessage(oldkey, newkey string) string {
	return fmt.Sprintf("configuration key `%s` is deprecated, use `%s` instead", oldkey, newkey)
}

// limitMaxInt logs a warning and sets `key` to `max` if the value exceeds `max`.
func limitMaxInt(cfg config.Config, key string, max int) {
	val := cfg.GetInt(key)
	if val > max {
		log.Warnf("configuration key `%s` was set to `%d`, using maximum value `%d` instead", key, val, max)
		cfg.Set(key, max)
	}
}

// limitMaxInt64 logs a warning and sets `key` to `max` if the value exceeds `max`.
func limitMaxInt64(cfg config.Config, key string, max int64) {
	val := cfg.GetInt64(key)
	if val > max {
		log.Warnf("configuration key `%s` was set to `%d`, using maximum value `%d` instead", key, val, max)
		cfg.Set(key, max)
	}
}
