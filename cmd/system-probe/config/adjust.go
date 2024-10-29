// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config contains the general configuration for system-probe
package config

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var adjustMtx sync.Mutex

// Adjust makes changes to the raw config based on deprecations and inferences.
func Adjust(cfg model.Config) {
	adjustMtx.Lock()
	defer adjustMtx.Unlock()
	if cfg.GetBool(spNS("adjusted")) {
		return
	}

	deprecateString(cfg, spNS("log_level"), "log_level")
	deprecateString(cfg, spNS("log_file"), "log_file")

	usmEnabled := cfg.GetBool(smNS("enabled"))
	npmEnabled := cfg.GetBool(netNS("enabled"))
	// this check must come first, so we can accurately tell if system_probe was explicitly enabled
	if cfg.GetBool(spNS("enabled")) &&
		!cfg.IsSet(netNS("enabled")) &&
		!usmEnabled {
		// This case exists to preserve backwards compatibility. If system_probe_config.enabled is explicitly set to true, and there is no network_config block,
		// enable the connections/network check.
		log.Warn(deprecationMessage(spNS("enabled"), netNS("enabled")))
		// ensure others can key off of this single config value for NPM status
		cfg.Set(netNS("enabled"), true, model.SourceAgentRuntime)
	}

	validateString(cfg, spNS("sysprobe_socket"), defaultSystemProbeAddress, ValidateSocketAddress)

	deprecateBool(cfg, spNS("allow_precompiled_fallback"), spNS("allow_prebuilt_fallback"))
	allowPrebuiltEbpfFallback(cfg)

	adjustNetwork(cfg)
	adjustUSM(cfg)
	adjustSecurity(cfg)

	if cfg.GetBool(spNS("process_service_inference", "enabled")) &&
		!usmEnabled &&
		!npmEnabled {
		log.Warn("universal service monitoring and network monitoring are disabled, disabling process service inference")
		cfg.Set(spNS("process_service_inference", "enabled"), false, model.SourceAgentRuntime)
	}

	cfg.Set(spNS("adjusted"), true, model.SourceAgentRuntime)
}

// validateString validates the string configuration value at `key` using a custom provided function `valFn`.
// If `key` is not set or `valFn` returns an error, the `defaultVal` is used instead.
func validateString(cfg model.Config, key string, defaultVal string, valFn func(string) error) {
	if cfg.IsSet(key) {
		if err := valFn(cfg.GetString(key)); err != nil {
			log.Errorf("error validating `%s`: %s, using default value of `%s`", key, err, defaultVal)
			cfg.Set(key, defaultVal, model.SourceAgentRuntime)
		}
	} else {
		cfg.Set(key, defaultVal, model.SourceAgentRuntime)
	}
}

// validateInt validates the int configuration value at `key` using a custom provided function `valFn`.
// If `key` is not set or `valFn` returns an error, the `defaultVal` is used instead.
func validateInt(cfg model.Config, key string, defaultVal int, valFn func(int) error) {
	if cfg.IsSet(key) {
		if err := valFn(cfg.GetInt(key)); err != nil {
			log.Errorf("error validating `%s`: %s, using default value of `%d`", key, err, defaultVal)
			cfg.Set(key, defaultVal, model.SourceAgentRuntime)
		}
	} else {
		cfg.Set(key, defaultVal, model.SourceAgentRuntime)
	}
}

// validateInt64 validates the int64 configuration value at `key` using a custom provided function `valFn`.
// If `key` is not set or `valFn` returns an error, the `defaultVal` is used instead.
func validateInt64(cfg model.Config, key string, defaultVal int64, valFn func(int64) error) {
	if cfg.IsSet(key) {
		if err := valFn(cfg.GetInt64(key)); err != nil {
			log.Errorf("error validating `%s`: %s. using default value of `%d`", key, err, defaultVal)
			cfg.Set(key, defaultVal, model.SourceAgentRuntime)
		}
	} else {
		cfg.Set(key, defaultVal, model.SourceAgentRuntime)
	}
}

// applyDefault sets configuration `key` to `defaultVal` only if not previously set.
func applyDefault(cfg model.Config, key string, defaultVal interface{}) {
	if !cfg.IsSet(key) {
		cfg.Set(key, defaultVal, model.SourceAgentRuntime)
	}
}

// deprecateBool logs a deprecation message if `oldkey` is used.
// It sets `newkey` to the value obtained from `getFn`, but only if `oldkey` is set and `newkey` is not set.
func deprecateBool(cfg model.Config, oldkey string, newkey string) {
	deprecateCustom(cfg, oldkey, newkey, func(cfg model.Config) interface{} {
		return cfg.GetBool(oldkey)
	})
}

// deprecateInt64 logs a deprecation message if `oldkey` is used.
// It sets `newkey` to the value obtained from `getFn`, but only if `oldkey` is set and `newkey` is not set.
func deprecateInt64(cfg model.Config, oldkey string, newkey string) {
	deprecateCustom(cfg, oldkey, newkey, func(cfg model.Config) interface{} {
		return cfg.GetInt64(oldkey)
	})
}

// deprecateGeneric logs a deprecation message if `oldkey` is used.
// It sets `newkey` to the value obtained from `getFn`, but only if `oldkey` is set and `newkey` is not set.
func deprecateGeneric(cfg model.Config, oldkey string, newkey string) {
	deprecateCustom(cfg, oldkey, newkey, func(cfg model.Config) interface{} {
		return cfg.Get(oldkey)
	})
}

// deprecateInt logs a deprecation message if `oldkey` is used.
// It sets `newkey` to the value obtained from `getFn`, but only if `oldkey` is set and `newkey` is not set.
func deprecateInt(cfg model.Config, oldkey string, newkey string) {
	deprecateCustom(cfg, oldkey, newkey, func(cfg model.Config) interface{} {
		return cfg.GetInt(oldkey)
	})
}

// deprecateString logs a deprecation message if `oldkey` is used.
// It sets `newkey` to the value obtained from `getFn`, but only if `oldkey` is set and `newkey` is not set.
func deprecateString(cfg model.Config, oldkey string, newkey string) {
	deprecateCustom(cfg, oldkey, newkey, func(cfg model.Config) interface{} {
		return cfg.GetString(oldkey)
	})
}

// deprecateCustom logs a deprecation message if `oldkey` is used.
// It sets `newkey` to the value obtained from `getFn`, but only if `oldkey` is set and `newkey` is not set.
func deprecateCustom(cfg model.Config, oldkey string, newkey string, getFn func(model.Config) interface{}) {
	if cfg.IsSet(oldkey) {
		log.Warn(deprecationMessage(oldkey, newkey))
		if !cfg.IsSet(newkey) {
			cfg.Set(newkey, getFn(cfg), model.SourceAgentRuntime)
		}
	}
}

// deprecationMessage returns the standard deprecation message
func deprecationMessage(oldkey, newkey string) string {
	return fmt.Sprintf("configuration key `%s` is deprecated, use `%s` instead", oldkey, newkey)
}

// limitMaxInt logs a warning and sets `key` to `max` if the value exceeds `max`.
func limitMaxInt(cfg model.Config, key string, max int) {
	val := cfg.GetInt(key)
	if val > max {
		log.Warnf("configuration key `%s` was set to `%d`, using maximum value `%d` instead", key, val, max)
		cfg.Set(key, max, model.SourceAgentRuntime)
	}
}

// limitMaxInt64 logs a warning and sets `key` to `max` if the value exceeds `max`.
func limitMaxInt64(cfg model.Config, key string, max int64) {
	val := cfg.GetInt64(key)
	if val > max {
		log.Warnf("configuration key `%s` was set to `%d`, using maximum value `%d` instead", key, val, max)
		cfg.Set(key, max, model.SourceAgentRuntime)
	}
}
